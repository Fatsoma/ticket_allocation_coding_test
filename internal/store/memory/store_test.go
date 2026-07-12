package memory_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"ticket-allocation/internal/store/memory"
	"ticket-allocation/internal/ticketing"
)

func TestCreatePurchaseRespectsAllocation(t *testing.T) {
	t.Parallel()

	store := memory.NewStore()
	ctx := context.Background()

	option, err := store.CreateTicketOption(ctx, ticketing.CreateTicketOptionParams{
		Name:        "GA",
		Allocation:  5,
		BucketCount: 1,
	})
	if err != nil {
		t.Fatalf("create option: %v", err)
	}

	userID := uuid.New()
	_, err = store.CreatePurchase(ctx, ticketing.CreatePurchaseParams{
		Quantity:       3,
		UserID:         userID,
		TicketOptionID: option.ID,
	})
	if err != nil {
		t.Fatalf("first purchase: %v", err)
	}

	_, err = store.CreatePurchase(ctx, ticketing.CreatePurchaseParams{
		Quantity:       3,
		UserID:         userID,
		TicketOptionID: option.ID,
	})
	if !errors.Is(err, ticketing.ErrInsufficientAllocation) {
		t.Fatalf("expected ErrInsufficientAllocation, got %v", err)
	}

	if got := store.PurchasedSum(option.ID); got != 3 {
		t.Fatalf("purchased = %d, want 3", got)
	}
	got, err := store.GetTicketOption(ctx, option.ID)
	if err != nil {
		t.Fatalf("get option: %v", err)
	}
	if got.Allocation != 5 {
		t.Fatalf("allocation = %d, want 5", got.Allocation)
	}
}

func TestMultiBucketFragmentation(t *testing.T) {
	t.Parallel()

	store := memory.NewStore()
	ctx := context.Background()

	// 3 buckets of capacity 2 each (allocation 6).
	option, err := store.CreateTicketOption(ctx, ticketing.CreateTicketOptionParams{
		Name:        "GA",
		Allocation:  6,
		BucketCount: 3,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	userID := uuid.New()
	// Buy 1 from each bucket to fragment remainders to [1,1,1].
	for i := 0; i < 3; i++ {
		if _, err := store.CreatePurchase(ctx, ticketing.CreatePurchaseParams{
			Quantity:       1,
			UserID:         userID,
			TicketOptionID: option.ID,
		}); err != nil {
			t.Fatalf("seed buy %d: %v", i, err)
		}
	}

	// Buy 3 must span buckets.
	if _, err := store.CreatePurchase(ctx, ticketing.CreatePurchaseParams{
		Quantity:       3,
		UserID:         userID,
		TicketOptionID: option.ID,
	}); err != nil {
		t.Fatalf("multi-bucket buy: %v", err)
	}

	if got := store.PurchasedSum(option.ID); got != 6 {
		t.Fatalf("purchased = %d, want 6", got)
	}

	_, err = store.CreatePurchase(ctx, ticketing.CreatePurchaseParams{
		Quantity:       1,
		UserID:         userID,
		TicketOptionID: option.ID,
	})
	if !errors.Is(err, ticketing.ErrInsufficientAllocation) {
		t.Fatalf("expected insufficient, got %v", err)
	}
}

func TestConcurrentPurchasesDoNotOversell(t *testing.T) {
	t.Parallel()

	store := memory.NewStore()
	ctx := context.Background()

	const allocation = 100
	const workers = 100
	const qty = 3

	option, err := store.CreateTicketOption(ctx, ticketing.CreateTicketOptionParams{
		Name:        "GA",
		Allocation:  allocation,
		BucketCount: 32,
	})
	if err != nil {
		t.Fatalf("create option: %v", err)
	}

	var (
		wg        sync.WaitGroup
		successes atomic.Int64
	)
	userID := uuid.New()

	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			_, err := store.CreatePurchase(ctx, ticketing.CreatePurchaseParams{
				Quantity:       qty,
				UserID:         userID,
				TicketOptionID: option.ID,
			})
			if err == nil {
				successes.Add(1)
				return
			}
			if !errors.Is(err, ticketing.ErrInsufficientAllocation) {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	maxSuccess := int64(allocation / qty)
	if successes.Load() != maxSuccess {
		t.Fatalf("successes = %d, want %d", successes.Load(), maxSuccess)
	}
	if got := store.PurchasedSum(option.ID); got != int(maxSuccess*qty) {
		t.Fatalf("purchased sum = %d, want %d", got, maxSuccess*qty)
	}
}

func TestGetTicketOptionNotFound(t *testing.T) {
	t.Parallel()

	store := memory.NewStore()
	_, err := store.GetTicketOption(context.Background(), uuid.New())
	if !errors.Is(err, ticketing.ErrTicketOptionNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}
