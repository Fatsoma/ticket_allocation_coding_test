package api

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"ticket-allocation/internal/ticketing"
)

// Server implements StrictServerInterface.
type Server struct {
	store ticketing.Store
}

func NewServer(store ticketing.Store) *Server {
	return &Server{store: store}
}

func (h *Server) CreateTicketOption(ctx context.Context, request CreateTicketOptionRequestObject) (CreateTicketOptionResponseObject, error) {
	if request.Body == nil {
		return CreateTicketOption400ApplicationVndAPIPlusJSONResponse(
			badRequestBodyError("request body is required"),
		), nil
	}

	attrs := request.Body.Data.Attributes
	description := ""
	if attrs.Description != nil {
		description = *attrs.Description
	}

	params := ticketing.CreateTicketOptionParams{
		Name:        attrs.Name,
		Description: description,
		Allocation:  attrs.Allocation,
		BucketCount: ticketing.ResolveBucketCount(attrs.Allocation, attrs.BucketCount),
	}
	if err := params.Validate(); err != nil {
		var invalid *ticketing.InvalidInputError
		if errors.As(err, &invalid) {
			return CreateTicketOption400ApplicationVndAPIPlusJSONResponse(invalidInputError(invalid)), nil
		}
		return CreateTicketOption400ApplicationVndAPIPlusJSONResponse(
			badRequestBodyError(err.Error()),
		), nil
	}

	option, err := h.store.CreateTicketOption(ctx, params)
	if err != nil {
		if doc, _, ok := MapDomainError(err); ok {
			return CreateTicketOption400ApplicationVndAPIPlusJSONResponse(doc), nil
		}
		return nil, fmt.Errorf("create ticket option: %w", err)
	}

	return CreateTicketOption201ApplicationVndAPIPlusJSONResponse(toTicketOptionDocument(option)), nil
}

func (h *Server) GetTicketOption(ctx context.Context, request GetTicketOptionRequestObject) (GetTicketOptionResponseObject, error) {
	option, err := h.store.GetTicketOption(ctx, uuid.UUID(request.TicketOptionID))
	if err != nil {
		if errors.Is(err, ticketing.ErrTicketOptionNotFound) {
			return GetTicketOption404ApplicationVndAPIPlusJSONResponse(
				notFoundError("ticket option not found"),
			), nil
		}
		return nil, fmt.Errorf("get ticket option: %w", err)
	}

	return GetTicketOption200ApplicationVndAPIPlusJSONResponse(toTicketOptionDocument(option)), nil
}

func (h *Server) CreatePurchase(ctx context.Context, request CreatePurchaseRequestObject) (CreatePurchaseResponseObject, error) {
	if request.Body == nil {
		return CreatePurchase400ApplicationVndAPIPlusJSONResponse(
			badRequestBodyError("request body is required"),
		), nil
	}

	data := request.Body.Data
	params := ticketing.CreatePurchaseParams{
		Quantity:       data.Attributes.Quantity,
		UserID:         uuid.UUID(data.Relationships.User.Data.Id),
		TicketOptionID: uuid.UUID(data.Relationships.TicketOption.Data.Id),
	}
	if err := params.Validate(); err != nil {
		var invalid *ticketing.InvalidInputError
		if errors.As(err, &invalid) {
			return CreatePurchase400ApplicationVndAPIPlusJSONResponse(invalidInputError(invalid)), nil
		}
		return CreatePurchase400ApplicationVndAPIPlusJSONResponse(
			badRequestBodyError(err.Error()),
		), nil
	}

	purchase, err := h.store.CreatePurchase(ctx, params)
	if err != nil {
		switch {
		case errors.Is(err, ticketing.ErrTicketOptionNotFound):
			return CreatePurchase404ApplicationVndAPIPlusJSONResponse(
				notFoundError("ticket option not found"),
			), nil
		case errors.Is(err, ticketing.ErrInsufficientAllocation):
			return CreatePurchase400ApplicationVndAPIPlusJSONResponse(
				insufficientAllocationError(),
			), nil
		default:
			var invalid *ticketing.InvalidInputError
			if errors.As(err, &invalid) {
				return CreatePurchase400ApplicationVndAPIPlusJSONResponse(invalidInputError(invalid)), nil
			}
			return nil, fmt.Errorf("create purchase: %w", err)
		}
	}

	return CreatePurchase201ApplicationVndAPIPlusJSONResponse(toPurchaseDocument(purchase)), nil
}

func toTicketOptionDocument(option ticketing.TicketOption) TicketOptionDocument {
	doc := TicketOptionDocument{}
	doc.Data.Type = "ticket_options"
	doc.Data.Id = openapi_types.UUID(option.ID)
	doc.Data.Attributes.Name = option.Name
	doc.Data.Attributes.Allocation = option.Allocation
	if option.Description != "" {
		doc.Data.Attributes.Description = ptr(option.Description)
	}
	return doc
}

func toPurchaseDocument(purchase ticketing.Purchase) PurchaseDocument {
	doc := PurchaseDocument{}
	doc.Data.Type = "purchases"
	doc.Data.Id = openapi_types.UUID(purchase.ID)
	doc.Data.Attributes.Quantity = purchase.Quantity
	doc.Data.Relationships.TicketOption.Data.Type = "ticket_options"
	doc.Data.Relationships.TicketOption.Data.Id = openapi_types.UUID(purchase.TicketOptionID)
	doc.Data.Relationships.User.Data.Type = "users"
	doc.Data.Relationships.User.Data.Id = openapi_types.UUID(purchase.UserID)
	return doc
}
