package api

import (
	"errors"
	"net/http"

	"ticket-allocation/internal/ticketing"
)

func ptr[T any](v T) *T { return &v }

func newErrorDocument(status, code, title, detail, pointer string) ErrorDocument {
	obj := ErrorObject{
		Status: status,
		Title:  title,
	}
	if code != "" {
		obj.Code = ptr(code)
	}
	if detail != "" {
		obj.Detail = ptr(detail)
	}
	if pointer != "" {
		obj.Source = &struct {
			Pointer *string `json:"pointer,omitempty"`
		}{Pointer: ptr(pointer)}
	}
	return ErrorDocument{Errors: []ErrorObject{obj}}
}

func invalidInputError(err *ticketing.InvalidInputError) ErrorDocument {
	return newErrorDocument(
		"400",
		"invalid_input",
		"Invalid request",
		err.Detail,
		err.Pointer,
	)
}

func insufficientAllocationError() ErrorDocument {
	return newErrorDocument(
		"400",
		"invalid_purchase_quantity",
		"Unable to purchase provided quantity",
		"Unable to reserve given quantity of ticket options",
		"/data/attributes/quantity",
	)
}

func notFoundError(detail string) ErrorDocument {
	return newErrorDocument(
		"404",
		"not_found",
		"Resource not found",
		detail,
		"",
	)
}

func badRequestBodyError(detail string) ErrorDocument {
	return newErrorDocument(
		"400",
		"invalid_request_body",
		"Invalid request body",
		detail,
		"",
	)
}

// MapDomainError maps domain errors to a JSON:API ErrorDocument and HTTP status.
// Returns ok=false when the error is not a known domain error.
func MapDomainError(err error) (doc ErrorDocument, status int, ok bool) {
	var invalid *ticketing.InvalidInputError
	switch {
	case errors.As(err, &invalid):
		return invalidInputError(invalid), http.StatusBadRequest, true
	case errors.Is(err, ticketing.ErrInsufficientAllocation):
		return insufficientAllocationError(), http.StatusBadRequest, true
	case errors.Is(err, ticketing.ErrTicketOptionNotFound):
		return notFoundError("ticket option not found"), http.StatusNotFound, true
	default:
		return ErrorDocument{}, 0, false
	}
}
