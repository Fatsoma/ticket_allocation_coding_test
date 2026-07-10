.PHONY: generate local run test test-integration

DATABASE_URL ?= postgres://ticket:ticket@localhost:5432/ticket_allocation?sslmode=disable
PORT ?= 3000

generate:
	go generate ./internal/api/v1/...

test-integration: docker-up
	TEST_DATABASE_URL="$(DATABASE_URL)" go test ./internal/store/postgres/... -tags=integration -race -count=1 -timeout=120s
