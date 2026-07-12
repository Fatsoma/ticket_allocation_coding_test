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
			name:    "valid default bucket",
			params:  ticketing.CreateTicketOptionParams{Name: "GA", Allocation: 10, BucketCount: 1},
			wantErr: false,
		},
		{
			name:    "valid max buckets",
			params:  ticketing.CreateTicketOptionParams{Name: "GA", Allocation: 100, BucketCount: 32},
			wantErr: false,
		},
		{
			name:    "missing name",
			params:  ticketing.CreateTicketOptionParams{Name: "  ", Allocation: 10, BucketCount: 1},
			wantErr: true,
			pointer: "/data/attributes/name",
		},
		{
			name:    "negative allocation",
			params:  ticketing.CreateTicketOptionParams{Name: "GA", Allocation: -1, BucketCount: 1},
			wantErr: true,
			pointer: "/data/attributes/allocation",
		},
		{
			name:    "zero allocation rejected",
			params:  ticketing.CreateTicketOptionParams{Name: "GA", Allocation: 0, BucketCount: 1},
			wantErr: true,
			pointer: "/data/attributes/allocation",
		},
		{
			name:    "bucket_count zero rejected",
			params:  ticketing.CreateTicketOptionParams{Name: "GA", Allocation: 10, BucketCount: 0},
			wantErr: true,
			pointer: "/data/attributes/bucket_count",
		},
		{
			name:    "bucket_count exceeds max",
			params:  ticketing.CreateTicketOptionParams{Name: "GA", Allocation: 100, BucketCount: 33},
			wantErr: true,
			pointer: "/data/attributes/bucket_count",
		},
		{
			name:    "bucket_count exceeds allocation",
			params:  ticketing.CreateTicketOptionParams{Name: "GA", Allocation: 5, BucketCount: 6},
			wantErr: true,
			pointer: "/data/attributes/bucket_count",
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

func TestResolveBucketCount(t *testing.T) {
	t.Parallel()

	if got := ticketing.ResolveBucketCount(nil); got != 1 {
		t.Fatalf("omitted = %d, want 1", got)
	}
	n := 8
	if got := ticketing.ResolveBucketCount(&n); got != 8 {
		t.Fatalf("explicit = %d, want 8", got)
	}
	zero := 0
	if got := ticketing.ResolveBucketCount(&zero); got != 0 {
		t.Fatalf("explicit zero = %d, want 0 (for validation)", got)
	}
}

func TestSplitAllocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		total       int
		bucketCount int
		want        []int
		wantErr     bool
	}{
		{name: "zero total", total: 0, bucketCount: 1, wantErr: true},
		{name: "zero buckets", total: 100, bucketCount: 0, wantErr: true},
		{name: "single", total: 100, bucketCount: 1, want: []int{100}},
		{name: "even", total: 100, bucketCount: 4, want: []int{25, 25, 25, 25}},
		{name: "remainder", total: 10, bucketCount: 3, want: []int{4, 3, 3}},
		{name: "max", total: 32, bucketCount: 32, want: ones(32)},
		{name: "too many", total: 5, bucketCount: 6, wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ticketing.SplitAllocation(tt.total, tt.bucketCount)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d", len(got), len(tt.want))
			}
			sum := 0
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
				sum += got[i]
			}
			if sum != tt.total {
				t.Fatalf("sum = %d, want %d", sum, tt.total)
			}
		})
	}
}

func ones(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = 1
	}
	return out
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
