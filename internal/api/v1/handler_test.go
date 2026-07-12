package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	api "ticket-allocation/internal/api/v1"
	"ticket-allocation/internal/store/memory"
)

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	store := memory.NewStore()
	handler := api.NewServer(store)
	return api.Handler(api.NewStrictHandlerWithOptions(handler, nil, api.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			w.Header().Set("Content-Type", "application/vnd.api+json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(api.ErrorDocument{
				Errors: []api.ErrorObject{{
					Status: "400",
					Title:  "Invalid request body",
					Detail: strPtr(err.Error()),
				}},
			})
		},
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			t.Errorf("unexpected handler error: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		},
	}))
}

func strPtr(s string) *string { return &s }

func doJSON(t *testing.T, h http.Handler, method, path string, body any) (int, map[string]any) {
	t.Helper()

	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		rdr = bytes.NewReader(b)
	}

	req := httptest.NewRequest(method, path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/vnd.api+json")
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	bodyBytes := rr.Body.Bytes()
	if len(bodyBytes) == 0 {
		return rr.Code, nil
	}

	var payload map[string]any
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		t.Fatalf("%s %s: response is not valid JSON (status=%d Content-Type=%q): %v\nbody=%q",
			method, path, rr.Code, rr.Header().Get("Content-Type"), err, string(bodyBytes))
	}
	return rr.Code, payload
}

func createTicketOption(t *testing.T, h http.Handler, name string, allocation int) string {
	t.Helper()
	status, body := doJSON(t, h, http.MethodPost, "/v1/ticket_options", map[string]any{
		"data": map[string]any{
			"type": "ticket_options",
			"attributes": map[string]any{
				"name":        name,
				"description": "desc",
				"allocation":  allocation,
			},
		},
	})
	if status != http.StatusCreated {
		t.Fatalf("create ticket option status = %d, body=%v", status, body)
	}
	data := body["data"].(map[string]any)
	return data["id"].(string)
}

func TestCreateAndGetTicketOption(t *testing.T) {
	t.Parallel()
	h := newTestServer(t)

	id := createTicketOption(t, h, "example", 100)

	status, body := doJSON(t, h, http.MethodGet, "/v1/ticket_options/"+id, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	attrs := body["data"].(map[string]any)["attributes"].(map[string]any)
	if attrs["name"] != "example" {
		t.Fatalf("name = %v", attrs["name"])
	}
	if int(attrs["allocation"].(float64)) != 100 {
		t.Fatalf("allocation = %v", attrs["allocation"])
	}
}

func TestGetTicketOptionNotFound(t *testing.T) {
	t.Parallel()
	h := newTestServer(t)

	status, body := doJSON(t, h, http.MethodGet, "/v1/ticket_options/"+uuid.NewString(), nil)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%v", status, body)
	}
	errors := body["errors"].([]any)
	if len(errors) == 0 {
		t.Fatal("expected errors array")
	}
}

func TestCreateTicketOptionValidation(t *testing.T) {
	t.Parallel()
	h := newTestServer(t)

	status, body := doJSON(t, h, http.MethodPost, "/v1/ticket_options", map[string]any{
		"data": map[string]any{
			"type": "ticket_options",
			"attributes": map[string]any{
				"name":       "",
				"allocation": 10,
			},
		},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%v", status, body)
	}
}

func TestCreateTicketOptionBucketCountValidation(t *testing.T) {
	t.Parallel()
	h := newTestServer(t)

	status, body := doJSON(t, h, http.MethodPost, "/v1/ticket_options", map[string]any{
		"data": map[string]any{
			"type": "ticket_options",
			"attributes": map[string]any{
				"name":         "GA",
				"allocation":   5,
				"bucket_count": 6,
			},
		},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%v", status, body)
	}

	status, body = doJSON(t, h, http.MethodPost, "/v1/ticket_options", map[string]any{
		"data": map[string]any{
			"type": "ticket_options",
			"attributes": map[string]any{
				"name":         "GA",
				"allocation":   100,
				"bucket_count": 33,
			},
		},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for max, body=%v", status, body)
	}

	status, body = doJSON(t, h, http.MethodPost, "/v1/ticket_options", map[string]any{
		"data": map[string]any{
			"type": "ticket_options",
			"attributes": map[string]any{
				"name":         "GA",
				"allocation":   100,
				"bucket_count": 8,
			},
		},
	})
	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body=%v", status, body)
	}

	status, body = doJSON(t, h, http.MethodPost, "/v1/ticket_options", map[string]any{
		"data": map[string]any{
			"type": "ticket_options",
			"attributes": map[string]any{
				"name":         "Sold out",
				"allocation":   0,
				"bucket_count": 1,
			},
		},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for allocation=0, body=%v", status, body)
	}

	status, body = doJSON(t, h, http.MethodPost, "/v1/ticket_options", map[string]any{
		"data": map[string]any{
			"type": "ticket_options",
			"attributes": map[string]any{
				"name":         "GA",
				"allocation":   10,
				"bucket_count": 0,
			},
		},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for bucket_count=0, body=%v", status, body)
	}
}

func TestPurchaseSuccessAndOversell(t *testing.T) {
	t.Parallel()
	h := newTestServer(t)

	optionID := createTicketOption(t, h, "GA", 2)
	userID := uuid.NewString()

	status, body := doJSON(t, h, http.MethodPost, "/v1/purchases", purchaseBody(optionID, userID, 2))
	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body=%v", status, body)
	}

	status, body = doJSON(t, h, http.MethodPost, "/v1/purchases", purchaseBody(optionID, userID, 1))
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%v", status, body)
	}
	errObj := body["errors"].([]any)[0].(map[string]any)
	if errObj["code"] != "invalid_purchase_quantity" {
		t.Fatalf("code = %v", errObj["code"])
	}
	source := errObj["source"].(map[string]any)
	if source["pointer"] != "/data/attributes/quantity" {
		t.Fatalf("pointer = %v", source["pointer"])
	}

	// Original allocation unchanged on GET.
	status, body = doJSON(t, h, http.MethodGet, "/v1/ticket_options/"+optionID, nil)
	if status != http.StatusOK {
		t.Fatalf("get status = %d", status)
	}
	attrs := body["data"].(map[string]any)["attributes"].(map[string]any)
	if int(attrs["allocation"].(float64)) != 2 {
		t.Fatalf("allocation changed to %v", attrs["allocation"])
	}
}

func TestPurchaseMissingTicketOption(t *testing.T) {
	t.Parallel()
	h := newTestServer(t)

	status, body := doJSON(t, h, http.MethodPost, "/v1/purchases", purchaseBody(uuid.NewString(), uuid.NewString(), 1))
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%v", status, body)
	}
}

func TestPurchaseZeroQuantity(t *testing.T) {
	t.Parallel()
	h := newTestServer(t)

	optionID := createTicketOption(t, h, "GA", 10)
	status, body := doJSON(t, h, http.MethodPost, "/v1/purchases", purchaseBody(optionID, uuid.NewString(), 0))
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%v", status, body)
	}
}

func purchaseBody(optionID, userID string, qty int) map[string]any {
	return map[string]any{
		"data": map[string]any{
			"type": "purchases",
			"attributes": map[string]any{
				"quantity": qty,
			},
			"relationships": map[string]any{
				"ticket_option": map[string]any{
					"data": map[string]any{
						"type": "ticket_options",
						"id":   optionID,
					},
				},
				"user": map[string]any{
					"data": map[string]any{
						"type": "users",
						"id":   userID,
					},
				},
			},
		},
	}
}

func TestPurchaseResponseShape(t *testing.T) {
	t.Parallel()
	h := newTestServer(t)

	optionID := createTicketOption(t, h, "GA", 5)
	userID := uuid.NewString()
	status, body := doJSON(t, h, http.MethodPost, "/v1/purchases", purchaseBody(optionID, userID, 1))
	if status != http.StatusCreated {
		t.Fatalf("status = %d body=%v", status, body)
	}

	data := body["data"].(map[string]any)
	if data["type"] != "purchases" {
		t.Fatalf("type = %v", data["type"])
	}
	if _, err := uuid.Parse(fmt.Sprint(data["id"])); err != nil {
		t.Fatalf("id not uuid: %v", data["id"])
	}
	rels := data["relationships"].(map[string]any)
	to := rels["ticket_option"].(map[string]any)["data"].(map[string]any)
	if to["id"] != optionID {
		t.Fatalf("ticket_option id = %v", to["id"])
	}
}
