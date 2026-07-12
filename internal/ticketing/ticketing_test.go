package ticketing_test

import (
	"errors"
	"testing"

	"ticket-allocation/internal/ticketing"

	"github.com/google/uuid"
)

func TestCreateTicketOptionParamsValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		params  ticketing.CreateTicketOptionParams
		wantErr bool
		pointer string
	}{
		{
			name:    "valid",
			params:  ticketing.CreateTicketOptionParams{Name: "GA", Allocation: 10},
			wantErr: false,
		},
		{
			name:    "missing name",
			params:  ticketing.CreateTicketOptionParams{Name: "  ", Allocation: 10},
			wantErr: true,
			pointer: "/data/attributes/name",
		},
		{
			name:    "negative allocation",
			params:  ticketing.CreateTicketOptionParams{Name: "GA", Allocation: -1},
			wantErr: true,
			pointer: "/data/attributes/allocation",
		},
		{
			name:    "zero allocation allowed",
			params:  ticketing.CreateTicketOptionParams{Name: "GA", Allocation: 0},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.params.Validate()
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr {
				var invalid *ticketing.InvalidInputError
				if !errors.As(err, &invalid) {
					t.Fatalf("expected InvalidInputError, got %T", err)
				}
				if invalid.Pointer != tt.pointer {
					t.Fatalf("pointer = %q, want %q", invalid.Pointer, tt.pointer)
				}
			}
		})
	}
}

func TestCreatePurchaseParamsValidate(t *testing.T) {
	t.Parallel()

	valid := ticketing.CreatePurchaseParams{
		Quantity:       1,
		UserID:         uuid.New(),
		TicketOptionID: uuid.New(),
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	zeroQty := valid
	zeroQty.Quantity = 0
	if err := zeroQty.Validate(); err == nil {
		t.Fatal("expected error for zero quantity")
	}
}
