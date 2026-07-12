//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"

	"ticket-allocation/internal/store/postgres"
	"ticket-allocation/internal/ticketing"
)

const concurrentBuyers = 64

const benchAllocation = 5_000_000

func openBenchDB(b *testing.B) *sqlx.DB {
	b.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		b.Skip("TEST_DATABASE_URL not set")
	}

	db, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		b.Fatalf("connect: %v", err)
	}
	// Match buyer concurrency so the pool is not the bottleneck we are measuring.
	db.SetMaxOpenConns(concurrentBuyers)
	db.SetMaxIdleConns(concurrentBuyers)
	db.SetConnMaxLifetime(5 * time.Minute)
	b.Cleanup(func() { _ = db.Close() })
	return db
}

// BenchmarkCreatePurchaseContention compares bucket_count=1 vs 32 under a fixed
// parallel purchase load.
func BenchmarkCreatePurchaseContention(b *testing.B) {
	for _, bucketCount := range []int{1, 32} {
		b.Run(fmt.Sprintf("bucket_count=%d/buyers=%d", bucketCount, concurrentBuyers), func(b *testing.B) {
			db := openBenchDB(b)
			if _, err := db.Exec(`TRUNCATE purchase_allocations, purchases, ticket_option_buckets, ticket_options CASCADE`); err != nil {
				b.Fatalf("truncate: %v", err)
			}

			store := postgres.NewStore(db)
			ctx := context.Background()

			option, err := store.CreateTicketOption(ctx, ticketing.CreateTicketOptionParams{
				Name:        fmt.Sprintf("bench-%d", bucketCount),
				Allocation:  benchAllocation,
				BucketCount: bucketCount,
			})
			if err != nil {
				b.Fatalf("create option: %v", err)
			}

			// RunParallel uses parallelism * GOMAXPROCS workers; scale to ~concurrentBuyers.
			procs := runtime.GOMAXPROCS(0)
			parallelism := (concurrentBuyers + procs - 1) / procs
			if parallelism < 1 {
				parallelism = 1
			}
			b.SetParallelism(parallelism)

			b.ReportMetric(float64(concurrentBuyers), "buyers")
			b.ReportMetric(float64(bucketCount), "buckets")
			b.ResetTimer()

			b.RunParallel(func(pb *testing.PB) {
				userID := uuid.New()
				for pb.Next() {
					_, err := store.CreatePurchase(ctx, ticketing.CreatePurchaseParams{
						Quantity:       1,
						UserID:         userID,
						TicketOptionID: option.ID,
					})
					if err == nil {
						continue
					}
					// Sell-out should not happen with benchAllocation; treat as hard failure.
					if errors.Is(err, ticketing.ErrInsufficientAllocation) {
						b.Error("sold out during benchmark; increase benchAllocation")
						return
					}
					b.Errorf("purchase: %v", err)
					return
				}
			})
		})
	}
}
