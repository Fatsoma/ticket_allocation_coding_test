package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"ticket-allocation/internal/ticketing"
)

// Store is a Postgres-backed ticketing.Store.
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
	Purchased   int       `db:"purchased"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}

func (r ticketOptionRow) toDomain() ticketing.TicketOption {
	return ticketing.TicketOption{
		ID:          r.ID,
		Name:        r.Name,
		Description: r.Description,
		Allocation:  r.Allocation,
		Purchased:   r.Purchased,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
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

	const q = `
		INSERT INTO ticket_options (name, description, allocation)
		VALUES ($1, $2, $3)
		RETURNING id, name, description, allocation, purchased, created_at, updated_at
	`

	var row ticketOptionRow
	if err := s.db.GetContext(ctx, &row, q, params.Name, params.Description, params.Allocation); err != nil {
		return ticketing.TicketOption{}, fmt.Errorf("create ticket option: %w", err)
	}
	return row.toDomain(), nil
}

func (s *Store) GetTicketOption(ctx context.Context, id uuid.UUID) (ticketing.TicketOption, error) {
	const q = `
		SELECT id, name, description, allocation, purchased, created_at, updated_at
		FROM ticket_options
		WHERE id = $1
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

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return ticketing.Purchase{}, fmt.Errorf("begin purchase tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Atomic conditional update: succeeds only when remaining capacity covers quantity.
	const reserveQ = `
		UPDATE ticket_options
		SET purchased = purchased + $2,
		    updated_at = now()
		WHERE id = $1
		  AND purchased + $2 <= allocation
	`
	result, err := tx.ExecContext(ctx, reserveQ, params.TicketOptionID, params.Quantity)
	if err != nil {
		return ticketing.Purchase{}, fmt.Errorf("reserve allocation: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return ticketing.Purchase{}, fmt.Errorf("reserve allocation rows affected: %w", err)
	}
	if affected == 0 {
		var exists bool
		if err := tx.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM ticket_options WHERE id = $1)`, params.TicketOptionID); err != nil {
			return ticketing.Purchase{}, fmt.Errorf("check ticket option exists: %w", err)
		}
		if !exists {
			return ticketing.Purchase{}, ticketing.ErrTicketOptionNotFound
		}
		return ticketing.Purchase{}, ticketing.ErrInsufficientAllocation
	}

	const insertQ = `
		INSERT INTO purchases (quantity, user_id, ticket_option_id)
		VALUES ($1, $2, $3)
		RETURNING id, quantity, user_id, ticket_option_id, created_at, updated_at
	`
	var row purchaseRow
	if err := tx.GetContext(ctx, &row, insertQ, params.Quantity, params.UserID, params.TicketOptionID); err != nil {
		return ticketing.Purchase{}, fmt.Errorf("insert purchase: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return ticketing.Purchase{}, fmt.Errorf("commit purchase: %w", err)
	}
	return row.toDomain(), nil
}
