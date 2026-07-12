.PHONY: generate local run test test-integration docker-up docker-down

DATABASE_URL ?= postgres://ticket:ticket@localhost:5432/ticket_allocation?sslmode=disable
PORT ?= 3000

generate:
	go generate ./internal/api/v1/...

# Build and run the full stack (Postgres + API) on the docker network.
local:
	docker compose up --build -d --wait
	@echo "API listening on http://localhost:3000"
	@echo "Health: curl -s http://localhost:3000/_health"

test:
	go test ./... -race -count=1

test-integration: docker-up
	TEST_DATABASE_URL="$(DATABASE_URL)" go test ./internal/store/postgres/... -tags=integration -race -count=1 -timeout=120s
