package ticketing

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrTicketOptionNotFound   = errors.New("ticket option not found")
	ErrInsufficientAllocation = errors.New("insufficient allocation")
	ErrInvalidInput           = errors.New("invalid input")
)

// InvalidInputError carries field-level detail for JSON:API error mapping.
type InvalidInputError struct {
	Pointer string
	Detail  string
}

func (e *InvalidInputError) Error() string {
	if e.Detail != "" {
		return e.Detail
	}
	return ErrInvalidInput.Error()
}

func (e *InvalidInputError) Unwrap() error {
	return ErrInvalidInput
}

type TicketOption struct {
	ID          uuid.UUID
	Name        string
	Description string
	Allocation  int
	Purchased   int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Purchase struct {
	ID             uuid.UUID
	Quantity       int
	UserID         uuid.UUID
	TicketOptionID uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type CreateTicketOptionParams struct {
	Name        string
	Description string
	Allocation  int
}

func (p CreateTicketOptionParams) Validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return &InvalidInputError{
			Pointer: "/data/attributes/name",
			Detail:  "name is required",
		}
	}
	if p.Allocation < 0 {
		return &InvalidInputError{
			Pointer: "/data/attributes/allocation",
			Detail:  "allocation must be a non-negative integer",
		}
	}
	return nil
}

type CreatePurchaseParams struct {
	Quantity       int
	UserID         uuid.UUID
	TicketOptionID uuid.UUID
}

func (p CreatePurchaseParams) Validate() error {
	if p.Quantity <= 0 {
		return &InvalidInputError{
			Pointer: "/data/attributes/quantity",
			Detail:  "quantity must be a positive integer",
		}
	}
	if p.TicketOptionID == uuid.Nil {
		return &InvalidInputError{
			Pointer: "/data/relationships/ticket_option/data/id",
			Detail:  "ticket_option id is required",
		}
	}
	if p.UserID == uuid.Nil {
		return &InvalidInputError{
			Pointer: "/data/relationships/user/data/id",
			Detail:  "user id is required",
		}
	}
	return nil
}

// Store persists ticket options and purchases.
type Store interface {
	CreateTicketOption(ctx context.Context, params CreateTicketOptionParams) (TicketOption, error)
	GetTicketOption(ctx context.Context, id uuid.UUID) (TicketOption, error)
	CreatePurchase(ctx context.Context, params CreatePurchaseParams) (Purchase, error)
}
