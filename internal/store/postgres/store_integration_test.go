//go:build integration

package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"

	"ticket-allocation/internal/store/postgres"
	"ticket-allocation/internal/ticketing"
)

func openTestDB(t *testing.T) *sqlx.DB {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	db, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	// Cap the pool so concurrent hammers queue instead of exhausting Postgres max_connections.
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(20)
	db.SetConnMaxLifetime(5 * time.Minute)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func truncate(t *testing.T, db *sqlx.DB) {
	t.Helper()
	if _, err := db.Exec(`TRUNCATE purchases, ticket_options CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

func TestPostgresCreateGetAndPurchase(t *testing.T) {
	db := openTestDB(t)
	truncate(t, db)
	store := postgres.NewStore(db)
	ctx := context.Background()

	option, err := store.CreateTicketOption(ctx, ticketing.CreateTicketOptionParams{
		Name:        "GA",
		Description: "general admission",
		Allocation:  10,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := store.GetTicketOption(ctx, option.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Allocation != 10 || got.Name != "GA" {
		t.Fatalf("unexpected option: %+v", got)
	}

	purchase, err := store.CreatePurchase(ctx, ticketing.CreatePurchaseParams{
		Quantity:       4,
		UserID:         uuid.New(),
		TicketOptionID: option.ID,
	})
	if err != nil {
		t.Fatalf("purchase: %v", err)
	}
	if purchase.Quantity != 4 {
		t.Fatalf("quantity = %d", purchase.Quantity)
	}

	got, err = store.GetTicketOption(ctx, option.ID)
	if err != nil {
		t.Fatalf("get after purchase: %v", err)
	}
	if got.Purchased != 4 {
		t.Fatalf("purchased = %d, want 4", got.Purchased)
	}
	if got.Allocation != 10 {
		t.Fatalf("allocation mutated: %d", got.Allocation)
	}
}

func TestPostgresInsufficientAllocation(t *testing.T) {
	db := openTestDB(t)
	truncate(t, db)
	store := postgres.NewStore(db)
	ctx := context.Background()

	option, err := store.CreateTicketOption(ctx, ticketing.CreateTicketOptionParams{
		Name:       "GA",
		Allocation: 2,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, err = store.CreatePurchase(ctx, ticketing.CreatePurchaseParams{
		Quantity:       3,
		UserID:         uuid.New(),
		TicketOptionID: option.ID,
	})
	if !errors.Is(err, ticketing.ErrInsufficientAllocation) {
		t.Fatalf("expected insufficient allocation, got %v", err)
	}

	var purchased int
	if err := db.Get(&purchased, `SELECT purchased FROM ticket_options WHERE id = $1`, option.ID); err != nil {
		t.Fatalf("select purchased: %v", err)
	}
	if purchased != 0 {
		t.Fatalf("purchased = %d, want 0", purchased)
	}

	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM purchases WHERE ticket_option_id = $1`, option.ID); err != nil {
		t.Fatalf("count purchases: %v", err)
	}
	if count != 0 {
		t.Fatalf("purchase rows = %d, want 0", count)
	}
}

func TestPostgresPurchaseMissingOption(t *testing.T) {
	db := openTestDB(t)
	truncate(t, db)
	store := postgres.NewStore(db)

	_, err := store.CreatePurchase(context.Background(), ticketing.CreatePurchaseParams{
		Quantity:       1,
		UserID:         uuid.New(),
		TicketOptionID: uuid.New(),
	})
	if !errors.Is(err, ticketing.ErrTicketOptionNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestPostgresConcurrentPurchasesDoNotOversell(t *testing.T) {
	db := openTestDB(t)
	truncate(t, db)
	store := postgres.NewStore(db)
	ctx := context.Background()

	const allocation = 100
	const workers = 100
	const qty = 3

	option, err := store.CreateTicketOption(ctx, ticketing.CreateTicketOptionParams{
		Name:       "GA",
		Allocation: allocation,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
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
			switch {
			case err == nil:
				successes.Add(1)
			case errors.Is(err, ticketing.ErrInsufficientAllocation):
				// expected under contention
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	wantSuccess := int64(allocation / qty)
	if successes.Load() != wantSuccess {
		t.Fatalf("successes = %d, want %d", successes.Load(), wantSuccess)
	}

	var purchased int
	if err := db.Get(&purchased, `SELECT purchased FROM ticket_options WHERE id = $1`, option.ID); err != nil {
		t.Fatalf("select purchased: %v", err)
	}
	if purchased != int(wantSuccess*qty) {
		t.Fatalf("purchased counter = %d, want %d", purchased, wantSuccess*qty)
	}

	var sum sql.NullInt64
	if err := db.Get(&sum, `SELECT COALESCE(SUM(quantity), 0) FROM purchases WHERE ticket_option_id = $1`, option.ID); err != nil {
		t.Fatalf("sum quantities: %v", err)
	}
	if sum.Int64 != wantSuccess*qty {
		t.Fatalf("sum(quantity) = %d, want %d", sum.Int64, wantSuccess*qty)
	}
	if sum.Int64 > allocation {
		t.Fatalf("oversold: sum=%d allocation=%d", sum.Int64, allocation)
	}
}

func TestPostgresExactSellOut(t *testing.T) {
	db := openTestDB(t)
	truncate(t, db)
	store := postgres.NewStore(db)
	ctx := context.Background()

	option, err := store.CreateTicketOption(ctx, ticketing.CreateTicketOptionParams{
		Name:       "GA",
		Allocation: 10,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	userID := uuid.New()
	for _, qty := range []int{7, 3} {
		if _, err := store.CreatePurchase(ctx, ticketing.CreatePurchaseParams{
			Quantity:       qty,
			UserID:         userID,
			TicketOptionID: option.ID,
		}); err != nil {
			t.Fatalf("purchase qty=%d: %v", qty, err)
		}
	}

	_, err = store.CreatePurchase(ctx, ticketing.CreatePurchaseParams{
		Quantity:       1,
		UserID:         userID,
		TicketOptionID: option.ID,
	})
	if !errors.Is(err, ticketing.ErrInsufficientAllocation) {
		t.Fatalf("expected insufficient allocation after sell-out, got %v", err)
	}

	assertPurchasedMatchesSum(t, db, option.ID, 10)
}

func TestPostgresConcurrentExactBoundary(t *testing.T) {
	db := openTestDB(t)
	truncate(t, db)
	store := postgres.NewStore(db)
	ctx := context.Background()

	const allocation = 50
	const workers = 50

	option, err := store.CreateTicketOption(ctx, ticketing.CreateTicketOptionParams{
		Name:       "GA",
		Allocation: allocation,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
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
				Quantity:       1,
				UserID:         userID,
				TicketOptionID: option.ID,
			})
			switch {
			case err == nil:
				successes.Add(1)
			case errors.Is(err, ticketing.ErrInsufficientAllocation):
				// should not happen when workers == allocation and qty == 1
				t.Errorf("unexpected insufficient allocation with exact boundary")
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	if successes.Load() != allocation {
		t.Fatalf("successes = %d, want %d", successes.Load(), allocation)
	}
	assertPurchasedMatchesSum(t, db, option.ID, allocation)
}

func TestPostgresIndependentOptionsUnderContention(t *testing.T) {
	db := openTestDB(t)
	truncate(t, db)
	store := postgres.NewStore(db)
	ctx := context.Background()

	const (
		allocationA = 40
		allocationB = 60
		workersA    = 60
		workersB    = 80
		qty         = 1
	)

	optionA, err := store.CreateTicketOption(ctx, ticketing.CreateTicketOptionParams{
		Name:       "A",
		Allocation: allocationA,
	})
	if err != nil {
		t.Fatalf("create A: %v", err)
	}
	optionB, err := store.CreateTicketOption(ctx, ticketing.CreateTicketOptionParams{
		Name:       "B",
		Allocation: allocationB,
	})
	if err != nil {
		t.Fatalf("create B: %v", err)
	}

	var (
		wg         sync.WaitGroup
		successesA atomic.Int64
		successesB atomic.Int64
	)
	userID := uuid.New()

	wg.Add(workersA + workersB)
	for i := 0; i < workersA; i++ {
		go func() {
			defer wg.Done()
			_, err := store.CreatePurchase(ctx, ticketing.CreatePurchaseParams{
				Quantity:       qty,
				UserID:         userID,
				TicketOptionID: optionA.ID,
			})
			switch {
			case err == nil:
				successesA.Add(1)
			case errors.Is(err, ticketing.ErrInsufficientAllocation):
			default:
				t.Errorf("option A unexpected error: %v", err)
			}
		}()
	}
	for i := 0; i < workersB; i++ {
		go func() {
			defer wg.Done()
			_, err := store.CreatePurchase(ctx, ticketing.CreatePurchaseParams{
				Quantity:       qty,
				UserID:         userID,
				TicketOptionID: optionB.ID,
			})
			switch {
			case err == nil:
				successesB.Add(1)
			case errors.Is(err, ticketing.ErrInsufficientAllocation):
			default:
				t.Errorf("option B unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	if successesA.Load() != allocationA {
		t.Fatalf("option A successes = %d, want %d", successesA.Load(), allocationA)
	}
	if successesB.Load() != allocationB {
		t.Fatalf("option B successes = %d, want %d", successesB.Load(), allocationB)
	}

	assertPurchasedMatchesSum(t, db, optionA.ID, allocationA)
	assertPurchasedMatchesSum(t, db, optionB.ID, allocationB)

	// No cross-contamination: every purchase row must reference exactly one of the two options,
	// and counts must match the per-option counters.
	var foreignOrphan int
	if err := db.Get(&foreignOrphan, `
		SELECT COUNT(*) FROM purchases
		WHERE ticket_option_id NOT IN ($1, $2)
	`, optionA.ID, optionB.ID); err != nil {
		t.Fatalf("count orphan FKs: %v", err)
	}
	if foreignOrphan != 0 {
		t.Fatalf("found %d purchases with unexpected ticket_option_id", foreignOrphan)
	}
}

func TestPostgresZeroAllocation(t *testing.T) {
	db := openTestDB(t)
	truncate(t, db)
	store := postgres.NewStore(db)
	ctx := context.Background()

	option, err := store.CreateTicketOption(ctx, ticketing.CreateTicketOptionParams{
		Name:       "SoldOut",
		Allocation: 0,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, err = store.CreatePurchase(ctx, ticketing.CreatePurchaseParams{
		Quantity:       1,
		UserID:         uuid.New(),
		TicketOptionID: option.ID,
	})
	if !errors.Is(err, ticketing.ErrInsufficientAllocation) {
		t.Fatalf("expected insufficient allocation, got %v", err)
	}

	assertPurchasedMatchesSum(t, db, option.ID, 0)

	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM purchases WHERE ticket_option_id = $1`, option.ID); err != nil {
		t.Fatalf("count purchases: %v", err)
	}
	if count != 0 {
		t.Fatalf("purchase rows = %d, want 0", count)
	}
}

// TestPostgresCancelUnderLoad cancels an in-flight purchase storm and asserts the
// purchased counter never diverges from SUM(purchases.quantity) — i.e. cancelled
// transactions roll back both the counter bump and the purchase insert together.
func TestPostgresCancelUnderLoad(t *testing.T) {
	db := openTestDB(t)
	truncate(t, db)
	store := postgres.NewStore(db)

	const allocation = 100
	const workers = 100

	option, err := store.CreateTicketOption(context.Background(), ticketing.CreateTicketOptionParams{
		Name:       "GA",
		Allocation: allocation,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		wg        sync.WaitGroup
		successes atomic.Int64
		cancels   atomic.Int64
		otherErrs atomic.Int64
		gate      = make(chan struct{})
	)
	userID := uuid.New()

	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			<-gate
			_, err := store.CreatePurchase(ctx, ticketing.CreatePurchaseParams{
				Quantity:       1,
				UserID:         userID,
				TicketOptionID: option.ID,
			})
			switch {
			case err == nil:
				successes.Add(1)
			case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
				cancels.Add(1)
			case errors.Is(err, ticketing.ErrInsufficientAllocation):
				// possible if cancel is late and capacity is exhausted
			case ctx.Err() != nil:
				// Driver may surface sql.ErrTxDone / "already rolled back" after cancel.
				cancels.Add(1)
			default:
				otherErrs.Add(1)
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}

	close(gate)
	// Let some transactions begin acquiring connections / row locks, then cancel.
	time.Sleep(5 * time.Millisecond)
	cancel()
	wg.Wait()

	if otherErrs.Load() != 0 {
		t.Fatalf("saw %d unexpected errors", otherErrs.Load())
	}
	if successes.Load() == 0 && cancels.Load() == 0 {
		t.Fatal("expected some successes and/or cancellations")
	}

	var purchased int
	if err := db.Get(&purchased, `SELECT purchased FROM ticket_options WHERE id = $1`, option.ID); err != nil {
		t.Fatalf("select purchased: %v", err)
	}
	var sum sql.NullInt64
	if err := db.Get(&sum, `SELECT COALESCE(SUM(quantity), 0) FROM purchases WHERE ticket_option_id = $1`, option.ID); err != nil {
		t.Fatalf("sum quantities: %v", err)
	}
	if int64(purchased) != sum.Int64 {
		t.Fatalf("counter/purchase divergence after cancel: purchased=%d sum=%d (successes=%d cancels=%d)",
			purchased, sum.Int64, successes.Load(), cancels.Load())
	}
	if purchased > allocation {
		t.Fatalf("oversold after cancel: purchased=%d allocation=%d", purchased, allocation)
	}
	if int64(purchased) != successes.Load() {
		t.Fatalf("purchased=%d does not match observed successes=%d", purchased, successes.Load())
	}
}

func assertPurchasedMatchesSum(t *testing.T, db *sqlx.DB, ticketOptionID uuid.UUID, want int) {
	t.Helper()

	var purchased int
	if err := db.Get(&purchased, `SELECT purchased FROM ticket_options WHERE id = $1`, ticketOptionID); err != nil {
		t.Fatalf("select purchased: %v", err)
	}
	if purchased != want {
		t.Fatalf("purchased = %d, want %d", purchased, want)
	}

	var sum sql.NullInt64
	if err := db.Get(&sum, `SELECT COALESCE(SUM(quantity), 0) FROM purchases WHERE ticket_option_id = $1`, ticketOptionID); err != nil {
		t.Fatalf("sum quantities: %v", err)
	}
	if sum.Int64 != int64(want) {
		t.Fatalf("sum(quantity) = %d, want %d", sum.Int64, want)
	}
}
