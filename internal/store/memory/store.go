package memory

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"ticket-allocation/internal/ticketing"
)

// Store is an in-memory ticketing.Store suitable for unit tests.
type Store struct {
	mu            sync.Mutex
	ticketOptions map[uuid.UUID]ticketing.TicketOption
	purchases     map[uuid.UUID]ticketing.Purchase
}

func NewStore() *Store {
	return &Store{
		ticketOptions: make(map[uuid.UUID]ticketing.TicketOption),
		purchases:     make(map[uuid.UUID]ticketing.Purchase),
	}
}

func (s *Store) CreateTicketOption(_ context.Context, params ticketing.CreateTicketOptionParams) (ticketing.TicketOption, error) {
	if err := params.Validate(); err != nil {
		return ticketing.TicketOption{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	option := ticketing.TicketOption{
		ID:          uuid.New(),
		Name:        params.Name,
		Description: params.Description,
		Allocation:  params.Allocation,
		Purchased:   0,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.ticketOptions[option.ID] = option
	return option, nil
}

func (s *Store) GetTicketOption(_ context.Context, id uuid.UUID) (ticketing.TicketOption, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	option, ok := s.ticketOptions[id]
	if !ok {
		return ticketing.TicketOption{}, ticketing.ErrTicketOptionNotFound
	}
	return option, nil
}

func (s *Store) CreatePurchase(_ context.Context, params ticketing.CreatePurchaseParams) (ticketing.Purchase, error) {
	if err := params.Validate(); err != nil {
		return ticketing.Purchase{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	option, ok := s.ticketOptions[params.TicketOptionID]
	if !ok {
		return ticketing.Purchase{}, ticketing.ErrTicketOptionNotFound
	}
	if option.Purchased+params.Quantity > option.Allocation {
		return ticketing.Purchase{}, ticketing.ErrInsufficientAllocation
	}

	now := time.Now().UTC()
	option.Purchased += params.Quantity
	option.UpdatedAt = now
	s.ticketOptions[option.ID] = option

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

// PurchasedSum returns the sum of purchase quantities for a ticket option (test helper).
func (s *Store) PurchasedSum(ticketOptionID uuid.UUID) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	option, ok := s.ticketOptions[ticketOptionID]
	if !ok {
		return 0
	}
	return option.Purchased
}
