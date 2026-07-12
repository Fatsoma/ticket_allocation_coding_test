package memory

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/google/uuid"

	"ticket-allocation/internal/ticketing"
)

type bucket struct {
	ID        uuid.UUID
	Index     int
	Capacity  int
	Purchased int
}

type ticketOption struct {
	Option  ticketing.TicketOption
	Buckets []bucket
}

// Store is an in-memory ticketing.Store with bucketed capacity.
type Store struct {
	mu            sync.Mutex
	ticketOptions map[uuid.UUID]*ticketOption
	purchases     map[uuid.UUID]ticketing.Purchase
}

func NewStore() *Store {
	return &Store{
		ticketOptions: make(map[uuid.UUID]*ticketOption),
		purchases:     make(map[uuid.UUID]ticketing.Purchase),
	}
}

func (s *Store) CreateTicketOption(_ context.Context, params ticketing.CreateTicketOptionParams) (ticketing.TicketOption, error) {
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

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	option := ticketing.TicketOption{
		ID:          uuid.New(),
		Name:        params.Name,
		Description: params.Description,
		Allocation:  params.Allocation,
		BucketCount: params.BucketCount,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	buckets := make([]bucket, len(capacities))
	for i, capacity := range capacities {
		buckets[i] = bucket{
			ID:       uuid.New(),
			Index:    i,
			Capacity: capacity,
		}
	}

	s.ticketOptions[option.ID] = &ticketOption{
		Option:  option,
		Buckets: buckets,
	}
	return option, nil
}

func (s *Store) GetTicketOption(_ context.Context, id uuid.UUID) (ticketing.TicketOption, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	to, ok := s.ticketOptions[id]
	if !ok {
		return ticketing.TicketOption{}, ticketing.ErrTicketOptionNotFound
	}
	return to.Option, nil
}

func (s *Store) CreatePurchase(_ context.Context, params ticketing.CreatePurchaseParams) (ticketing.Purchase, error) {
	if err := params.Validate(); err != nil {
		return ticketing.Purchase{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	to, ok := s.ticketOptions[params.TicketOptionID]
	if !ok {
		return ticketing.Purchase{}, ticketing.ErrTicketOptionNotFound
	}
	if len(to.Buckets) == 0 {
		return ticketing.Purchase{}, ticketing.ErrInsufficientAllocation
	}

	if !tryReserveSingleBucketMem(to.Buckets, params.Quantity) {
		if !reserveMultiBucketMem(to.Buckets, params.Quantity) {
			return ticketing.Purchase{}, ticketing.ErrInsufficientAllocation
		}
	}

	now := time.Now().UTC()
	to.Option.UpdatedAt = now

	purchase := ticketing.Purchase{
		ID:             uuid.New(),
		Quantity:       params.Quantity,
		UserID:         params.UserID,
		TicketOptionID: params.TicketOptionID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	s.purchases[purchase.ID] = purchase
	return purchase, nil
}

// PurchasedSum returns the sum of purchased quantities for a ticket option (test helper).
func (s *Store) PurchasedSum(ticketOptionID uuid.UUID) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	to, ok := s.ticketOptions[ticketOptionID]
	if !ok {
		return 0
	}
	return purchasedSum(to.Buckets)
}

// BucketPurchased returns purchased counts by bucket index (test helper).
func (s *Store) BucketPurchased(ticketOptionID uuid.UUID) []int {
	s.mu.Lock()
	defer s.mu.Unlock()

	to, ok := s.ticketOptions[ticketOptionID]
	if !ok {
		return nil
	}
	out := make([]int, len(to.Buckets))
	for i, b := range to.Buckets {
		out[i] = b.Purchased
	}
	return out
}

func purchasedSum(buckets []bucket) int {
	sum := 0
	for _, b := range buckets {
		sum += b.Purchased
	}
	return sum
}

func tryReserveSingleBucketMem(buckets []bucket, qty int) bool {
	n := len(buckets)
	start := rand.Intn(n)
	for i := 0; i < n; i++ {
		idx := (start + i) % n
		if buckets[idx].Purchased+qty <= buckets[idx].Capacity {
			buckets[idx].Purchased += qty
			return true
		}
	}
	return false
}

func reserveMultiBucketMem(buckets []bucket, qty int) bool {
	remaining := qty
	takes := make([]int, len(buckets))
	for i := range buckets {
		if remaining == 0 {
			break
		}
		available := buckets[i].Capacity - buckets[i].Purchased
		if available <= 0 {
			continue
		}
		take := available
		if take > remaining {
			take = remaining
		}
		takes[i] = take
		remaining -= take
	}
	if remaining > 0 {
		return false
	}
	for i, take := range takes {
		if take > 0 {
			buckets[i].Purchased += take
		}
	}
	return true
}
