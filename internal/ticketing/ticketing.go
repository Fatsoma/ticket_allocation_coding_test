package ticketing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const MaxBucketCount = 32

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
	BucketCount int
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
	BucketCount int // resolved create-time value (default 1 when allocation > 0)
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
	if err := ValidateBucketCount(p.Allocation, p.BucketCount); err != nil {
		return err
	}
	return nil
}

// ValidateBucketCount enforces bucket_count rules at create time.
func ValidateBucketCount(allocation, bucketCount int) error {
	if allocation == 0 {
		if bucketCount != 0 {
			return &InvalidInputError{
				Pointer: "/data/attributes/bucket_count",
				Detail:  "bucket_count must be 0 when allocation is 0",
			}
		}
		return nil
	}
	if bucketCount < 1 {
		return &InvalidInputError{
			Pointer: "/data/attributes/bucket_count",
			Detail:  "bucket_count must be at least 1",
		}
	}
	if bucketCount > MaxBucketCount {
		return &InvalidInputError{
			Pointer: "/data/attributes/bucket_count",
			Detail:  fmt.Sprintf("bucket_count must be at most %d", MaxBucketCount),
		}
	}
	if bucketCount > allocation {
		return &InvalidInputError{
			Pointer: "/data/attributes/bucket_count",
			Detail:  "bucket_count must be less than or equal to allocation",
		}
	}
	return nil
}

// ResolveBucketCount applies the create-time default
func ResolveBucketCount(allocation int, requested *int) int {
	if allocation == 0 {
		return 0
	}
	if requested == nil {
		return 1
	}
	return *requested
}

// SplitAllocation evenly divides total capacity across bucketCount buckets.
func SplitAllocation(total, bucketCount int) ([]int, error) {
	if total < 0 {
		return nil, fmt.Errorf("total must be non-negative")
	}
	if total == 0 {
		if bucketCount != 0 {
			return nil, fmt.Errorf("bucketCount must be 0 when total is 0")
		}
		return nil, nil
	}
	if bucketCount < 1 {
		return nil, fmt.Errorf("bucketCount must be at least 1")
	}
	if bucketCount > total {
		return nil, fmt.Errorf("bucketCount must be <= total")
	}

	base := total / bucketCount
	rem := total % bucketCount
	out := make([]int, bucketCount)
	for i := 0; i < bucketCount; i++ {
		out[i] = base
		if i < rem {
			out[i]++
		}
	}
	return out, nil
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
