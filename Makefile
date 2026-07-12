.PHONY: generate local run test test-integration bench-purchase docker-up docker-down

DATABASE_URL ?= postgres://ticket:ticket@localhost:5432/ticket_allocation?sslmode=disable
PORT ?= 3000

generate:
	go generate ./internal/api/v1/...

# Build and run the full stack (Postgres + API) on the docker network.
local:
	docker compose up --build -d --wait
	@echo "API listening on http://localhost:3000"
	@echo "Health: curl -s http://localhost:3000/_health"

docker-up:
	docker compose up -d --wait postgres

docker-down:
	docker compose down -v

# Run the API on the host against compose Postgres.
run: docker-up
	DATABASE_URL="$(DATABASE_URL)" PORT="$(PORT)" go run ./cmd/server

test:
	go test ./... -race -count=1

test-integration: docker-up
	TEST_DATABASE_URL="$(DATABASE_URL)" go test ./internal/store/postgres/... -tags=integration -race -count=1 -timeout=120s

bench-purchase: docker-up
	TEST_DATABASE_URL="$(DATABASE_URL)" go test ./internal/store/postgres/... -tags=integration -run=^$$ -bench=BenchmarkCreatePurchaseContention -benchmem -count=3 -timeout=180s
