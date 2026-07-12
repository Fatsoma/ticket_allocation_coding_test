package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"

	"ticket-allocation/internal/ticketing"
)

// Store is a Postgres-backed ticketing.Store using bucketed capacity.
type Store struct {
	db *sqlx.DB
}

func NewStore(db *sqlx.DB) *Store {
	return &Store{db: db}
}

type ticketOptionRow struct {
	ID          uuid.UUID `db:"id"`
	Name        string    `db:"name"`
	Description string    `db:"description"`
	Allocation  int       `db:"allocation"`
	BucketCount int       `db:"bucket_count"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}

func (r ticketOptionRow) toDomain() ticketing.TicketOption {
	return ticketing.TicketOption{
		ID:          r.ID,
		Name:        r.Name,
		Description: r.Description,
		Allocation:  r.Allocation,
		BucketCount: r.BucketCount,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

type bucketRow struct {
	ID             uuid.UUID `db:"id"`
	TicketOptionID uuid.UUID `db:"ticket_option_id"`
	BucketIndex    int       `db:"bucket_index"`
	Capacity       int       `db:"capacity"`
	Purchased      int       `db:"purchased"`
}

type purchaseRow struct {
	ID             uuid.UUID `db:"id"`
	Quantity       int       `db:"quantity"`
	UserID         uuid.UUID `db:"user_id"`
	TicketOptionID uuid.UUID `db:"ticket_option_id"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
}

func (r purchaseRow) toDomain() ticketing.Purchase {
	return ticketing.Purchase{
		ID:             r.ID,
		Quantity:       r.Quantity,
		UserID:         r.UserID,
		TicketOptionID: r.TicketOptionID,
		CreatedAt:      r.CreatedAt,
		UpdatedAt:      r.UpdatedAt,
	}
}

func (s *Store) CreateTicketOption(ctx context.Context, params ticketing.CreateTicketOptionParams) (ticketing.TicketOption, error) {
	if err := params.Validate(); err != nil {
		return ticketing.TicketOption{}, err
	}

	capacities, err := ticketing.SplitAllocation(params.Allocation, params.BucketCount)
	if err != nil {
		return ticketing.TicketOption{}, &ticketing.InvalidInputError{
			Pointer: "/data/attributes/bucket_count",
			Detail:  err.Error(),
		}
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return ticketing.TicketOption{}, fmt.Errorf("begin create ticket option tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	const insertOption = `
		INSERT INTO ticket_options (name, description, allocation, bucket_count)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, description, allocation, bucket_count, created_at, updated_at
	`
	var option ticketOptionRow
	if err := tx.GetContext(ctx, &option, insertOption, params.Name, params.Description, params.Allocation, params.BucketCount); err != nil {
		return ticketing.TicketOption{}, fmt.Errorf("create ticket option: %w", err)
	}

	for i, capacity := range capacities {
		const insertBucket = `
			INSERT INTO ticket_option_buckets (ticket_option_id, bucket_index, capacity)
			VALUES ($1, $2, $3)
		`
		if _, err := tx.ExecContext(ctx, insertBucket, option.ID, i, capacity); err != nil {
			return ticketing.TicketOption{}, fmt.Errorf("create ticket option bucket: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return ticketing.TicketOption{}, fmt.Errorf("commit create ticket option: %w", err)
	}

	return option.toDomain(), nil
}

func (s *Store) GetTicketOption(ctx context.Context, id uuid.UUID) (ticketing.TicketOption, error) {
	const q = `
		SELECT
			t.id, t.name, t.description, t.allocation, t.bucket_count,
			t.created_at, t.updated_at
		FROM ticket_options t
		WHERE t.id = $1
	`

	var row ticketOptionRow
	err := s.db.GetContext(ctx, &row, q, id)
	if errors.Is(err, sql.ErrNoRows) {
		return ticketing.TicketOption{}, ticketing.ErrTicketOptionNotFound
	}
	if err != nil {
		return ticketing.TicketOption{}, fmt.Errorf("get ticket option: %w", err)
	}
	return row.toDomain(), nil
}

func (s *Store) CreatePurchase(ctx context.Context, params ticketing.CreatePurchaseParams) (ticketing.Purchase, error) {
	if err := params.Validate(); err != nil {
		return ticketing.Purchase{}, err
	}

	const maxAttempts = 5
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return ticketing.Purchase{}, err
		}
		purchase, err := s.createPurchaseOnce(ctx, params)
		if err == nil {
			return purchase, nil
		}
		if !isRetryableTxError(err) {
			return ticketing.Purchase{}, err
		}
		lastErr = err
		// Jittered backoff so concurrent retries do not immediately collide again.
		backoff := time.Duration(5*(1<<attempt)+rand.Intn(10)) * time.Millisecond
		select {
		case <-ctx.Done():
			return ticketing.Purchase{}, ctx.Err()
		case <-time.After(backoff):
		}
	}
	return ticketing.Purchase{}, fmt.Errorf("create purchase after retries: %w", lastErr)
}

func isRetryableTxError(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == pgerrcode.DeadlockDetected ||
			pgErr.Code == pgerrcode.SerializationFailure
	}
	return false
}

func (s *Store) createPurchaseOnce(ctx context.Context, params ticketing.CreatePurchaseParams) (ticketing.Purchase, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return ticketing.Purchase{}, fmt.Errorf("begin purchase tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var buckets []bucketRow
	if err := tx.SelectContext(ctx, &buckets, `
		SELECT id, ticket_option_id, bucket_index, capacity, purchased
		FROM ticket_option_buckets
		WHERE ticket_option_id = $1
		ORDER BY bucket_index
	`, params.TicketOptionID); err != nil {
		return ticketing.Purchase{}, fmt.Errorf("load buckets: %w", err)
	}

	if len(buckets) == 0 {
		var exists bool
		if err := tx.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM ticket_options WHERE id = $1)`, params.TicketOptionID); err != nil {
			return ticketing.Purchase{}, fmt.Errorf("check ticket option exists: %w", err)
		}
		if !exists {
			return ticketing.Purchase{}, ticketing.ErrTicketOptionNotFound
		}
		return ticketing.Purchase{}, ticketing.ErrInsufficientAllocation
	}

	allocations, err := tryReserveSingleBucket(ctx, tx, buckets, params.Quantity)
	if err != nil {
		return ticketing.Purchase{}, err
	}
	if allocations == nil {
		allocations, err = reserveMultiBucket(ctx, tx, params.TicketOptionID, params.Quantity)
		if err != nil {
			return ticketing.Purchase{}, err
		}
	}

	const insertQ = `
		INSERT INTO purchases (quantity, user_id, ticket_option_id)
		VALUES ($1, $2, $3)
		RETURNING id, quantity, user_id, ticket_option_id, created_at, updated_at
	`
	var purchase purchaseRow
	if err := tx.GetContext(ctx, &purchase, insertQ, params.Quantity, params.UserID, params.TicketOptionID); err != nil {
		return ticketing.Purchase{}, fmt.Errorf("insert purchase: %w", err)
	}

	for _, a := range allocations {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO purchase_allocations (purchase_id, bucket_id, quantity)
			VALUES ($1, $2, $3)
		`, purchase.ID, a.BucketID, a.Quantity); err != nil {
			return ticketing.Purchase{}, fmt.Errorf("insert purchase allocation: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return ticketing.Purchase{}, fmt.Errorf("commit purchase: %w", err)
	}
	return purchase.toDomain(), nil
}

type bucketAllocation struct {
	BucketID uuid.UUID
	Quantity int
}

func tryReserveSingleBucket(ctx context.Context, tx *sqlx.Tx, buckets []bucketRow, qty int) ([]bucketAllocation, error) {
	n := len(buckets)
	if n == 0 {
		return nil, nil
	}
	start := rand.Intn(n)
	for i := 0; i < n; i++ {
		b := buckets[(start+i)%n]
		sp := fmt.Sprintf("sp_bucket_%d", i)
		if _, err := tx.ExecContext(ctx, "SAVEPOINT "+sp); err != nil {
			return nil, fmt.Errorf("savepoint: %w", err)
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE ticket_option_buckets
			SET purchased = purchased + $2,
			    updated_at = now()
			WHERE id = $1
			  AND purchased + $2 <= capacity
		`, b.ID, qty)
		if err != nil {
			_, _ = tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+sp)
			return nil, fmt.Errorf("reserve single bucket: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			_, _ = tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+sp)
			return nil, fmt.Errorf("reserve single bucket rows affected: %w", err)
		}
		if affected == 1 {
			if _, err := tx.ExecContext(ctx, "RELEASE SAVEPOINT "+sp); err != nil {
				return nil, fmt.Errorf("release savepoint: %w", err)
			}
			return []bucketAllocation{{BucketID: b.ID, Quantity: qty}}, nil
		}
		// Release any row lock taken by a non-matching UPDATE attempt.
		if _, err := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+sp); err != nil {
			return nil, fmt.Errorf("rollback savepoint: %w", err)
		}
	}
	return nil, nil
}

func reserveMultiBucket(ctx context.Context, tx *sqlx.Tx, ticketOptionID uuid.UUID, qty int) ([]bucketAllocation, error) {
	var buckets []bucketRow
	if err := tx.SelectContext(ctx, &buckets, `
		SELECT id, ticket_option_id, bucket_index, capacity, purchased
		FROM ticket_option_buckets
		WHERE ticket_option_id = $1
		ORDER BY bucket_index
		FOR UPDATE
	`, ticketOptionID); err != nil {
		return nil, fmt.Errorf("lock buckets: %w", err)
	}

	remaining := qty
	var allocations []bucketAllocation
	for _, b := range buckets {
		if remaining == 0 {
			break
		}
		available := b.Capacity - b.Purchased
		if available <= 0 {
			continue
		}
		take := available
		if take > remaining {
			take = remaining
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE ticket_option_buckets
			SET purchased = purchased + $2,
			    updated_at = now()
			WHERE id = $1
			  AND purchased + $2 <= capacity
		`, b.ID, take)
		if err != nil {
			return nil, fmt.Errorf("reserve multi bucket: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("reserve multi bucket rows affected: %w", err)
		}
		if affected != 1 {
			return nil, ticketing.ErrInsufficientAllocation
		}
		allocations = append(allocations, bucketAllocation{BucketID: b.ID, Quantity: take})
		remaining -= take
	}

	if remaining > 0 {
		return nil, ticketing.ErrInsufficientAllocation
	}
	return allocations, nil
}
