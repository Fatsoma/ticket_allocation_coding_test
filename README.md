# Ticket Allocation API

Spec-first OpenAPI, Postgres, bucketed inventory for flash-sale contention.

## Requirements

- Go 1.25+ — only needed for host-side `make run` / tests (`go.mod` / Docker builder)
- Docker / Docker Compose

## How to run

Full stack (Postgres + API) on the docker network:

```bash
make local
```

This builds the API image, starts Postgres and the API, and waits until both are healthy. The API is at `http://localhost:3000`.

### Helpful commands

Spin up the API locally:
```bash
make local
```

- Health check: `GET http://localhost:3000/_health`

Generate handler code using the Open API specification:

```bash
make generate
```

### Example requests

```bash
# Create ticket option (bucket_count optional; defaults to 1)
curl -s -X POST http://localhost:3000/v1/ticket_options \
  -H 'Content-Type: application/vnd.api+json' \
  -d '{"data":{"type":"ticket_options","attributes":{"name":"example","description":"sample","allocation":100,"bucket_count":8}}}'

# Get ticket option
curl -s http://localhost:3000/v1/ticket_options/<id>

# Purchase
curl -s -X POST http://localhost:3000/v1/purchases \
  -H 'Content-Type: application/vnd.api+json' \
  -d '{"data":{"type":"purchases","attributes":{"quantity":2},"relationships":{"ticket_option":{"data":{"type":"ticket_options","id":"<id>"}},"user":{"data":{"type":"users","id":"d6abe829-c28c-44ec-bee6-3183f2c53fef"}}}}}'
```

## How to run the tests

```bash
# Unit tests (fake in-memory store + HTTP handler tests, with the race detector)
make test

# Integration tests against real Postgres
make test-integration

# Purchase throughput: bucket_count=1 vs 32 (~64 concurrent buyers)
make bench-purchase
```

### Benchmark results (`make bench-purchase`)

To justify the bucketed allocation solution, we used the following benchmarks.

Local run on Apple M5 / Darwin arm64, Postgres via Docker, `-count=3`, ~64 concurrent buyers:

| Config | Run | ns/op | B/op | allocs/op | ≈ purchases/sec |
|---|---:|---:|---:|---:|---:|
| `bucket_count=1` | 1 | 1,279,574 | 15,425 | 265 | ~780 |
| `bucket_count=1` | 2 | 1,234,176 | 15,558 | 266 | ~810 |
| `bucket_count=1` | 3 | 1,516,179 | 17,694 | 274 | ~660 |
| `bucket_count=32` | 1 | 450,953 | 21,842 | 501 | ~2,220 |
| `bucket_count=32` | 2 | 273,280 | 21,277 | 503 | ~3,660 |
| `bucket_count=32` | 3 | 277,221 | 21,172 | 505 | ~3,610 |

Analysis:

- Under this load, **32 buckets are roughly 3–4× faster** than a single bucket (lower `ns/op` → higher throughput).
- With one bucket, ~64 buyers serialise on the same row lock; with 32 buckets, many purchases hit different rows and proceed in parallel.
- **32 buckets allocate more** (~500 allocs/op vs ~270) because more bucket rows are loaded/probed — lock wait still dominates latency, so throughput still wins.
- Numbers are noisy on local Docker (see run-to-run spread); treat them as a directional comparison, not a lab-grade SLA.

## Design notes

The philosophy here is: unit test everything above the persistence layer in isolation, then integration test the persistence layer.

Instead of writing handlers manually, `internal/api/v1/Ticket_Allocation.swagger.yaml` is the single source of truth for endpoints.

### No-oversell invariant (bucketed capacity)

Total capacity (`allocation`) is fixed on `ticket_options`. At create time it is split across `bucket_count` rows in `ticket_option_buckets` (default **1**, max **32**, and `bucket_count <= allocation` so no empty buckets).

Each bucket has its own `purchased` counter with:

```sql
CHECK (purchased >= 0 AND purchased <= capacity)
```

Purchases run in a single transaction:

1. **Fast path:** try to fit the entire quantity in one bucket via conditional `UPDATE` (random start index, probe all buckets)
2. **Slow path:** if no single bucket fits, lock buckets in `bucket_index` order and fill across multiple buckets - This is likely to occur as the event sells out.
3. Insert one `purchases` row plus `purchase_allocations` lines for each bucket touched
4. On shortfall, roll back — nothing persisted

This spreads write contention across buckets under concurrent load while remaining correct across horizontally scaled API instances. With `bucket_count=1`, behaviour matches a single-row counter.

`GET /v1/ticket_options/:id` returns the original `allocation` (total capacity), not remaining availability or `bucket_count`, as required by the brief.

### Layering

- `internal/ticketing` — domain types, validation (`bucket_count` rules, `SplitAllocation`), sentinel errors, `Store` interface
- `internal/store/postgres` — sqlx + pgx/stdlib implementation
- `internal/store/memory` — mutex-backed fake for unit tests
- `internal/api/v1` — oapi-codegen (`types`, `std-http-server`, `strict-server`) plus hand-written handler / JSON:API error mapping
- `cmd/server` — wiring, config, graceful shutdown

### Assumptions

- User resources are not managed; any UUID is accepted as `user.id` (per brief)
- `description` defaults to empty string when omitted
- `bucket_count` defaults to `1` when omitted and `allocation > 0`; must be `0` when `allocation` is `0`
- Allocation of `0` is allowed; purchases against it fail with insufficient allocation
- JSON:API response schemas live in the OpenAPI spec so codegen owns serialisation shapes

### Trade-offs

- **Bucketed denormalised counters** vs `SUM(purchases.quantity)`: O(1) reservation per bucket and higher write throughput on hot options; the trade-off is keeping bucket counters consistent with `purchase_allocations` (done in one transaction) and more complex multi-bucket fills
- **Configurable `bucket_count`** (default 1): preserves simple single-row semantics by default; operators can opt into sharding up to 32 buckets
- **stdlib `net/http` routing** (Go 1.22+) over a third-party router: less dependency surface; oapi-codegen's `std-http-server` generator matches this choice
- **sqlx over an ORM**: explicit SQL for the critical path, still ergonomic scanning
