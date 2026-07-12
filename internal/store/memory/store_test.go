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
		Name:       "GA",
		Allocation: 5,
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

	got, err := store.GetTicketOption(ctx, option.ID)
	if err != nil {
		t.Fatalf("get option: %v", err)
	}
	if got.Purchased != 3 {
		t.Fatalf("purchased = %d, want 3", got.Purchased)
	}
	if got.Allocation != 5 {
		t.Fatalf("allocation = %d, want 5", got.Allocation)
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
		Name:       "GA",
		Allocation: allocation,
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
