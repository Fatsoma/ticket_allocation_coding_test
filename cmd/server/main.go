package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"

	api "ticket-allocation/internal/api/v1"
	"ticket-allocation/internal/store/postgres"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	db, err := sqlx.Connect("pgx", cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	store := postgres.NewStore(db)
	handler := api.NewServer(store)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /_health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	strict := api.NewStrictHandlerWithOptions(handler, nil, api.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeJSONAPIError(w, http.StatusBadRequest, api.ErrorDocument{
				Errors: []api.ErrorObject{{
					Status: "400",
					Code:   strPtr("invalid_request_body"),
					Title:  "Invalid request body",
					Detail: strPtr(err.Error()),
				}},
			})
		},
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("internal error: %v", err)
			writeJSONAPIError(w, http.StatusInternalServerError, api.ErrorDocument{
				Errors: []api.ErrorObject{{
					Status: "500",
					Code:   strPtr("internal_error"),
					Title:  "Internal server error",
					Detail: strPtr("an unexpected error occurred"),
				}},
			})
		},
	})
	mux.Handle("/", api.Handler(strict))

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
		return nil
	case err := <-errCh:
		return err
	}
}

type config struct {
	DatabaseURL string
	Port        string
}

func loadConfig() (config, error) {
	cfg := config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		Port:        os.Getenv("PORT"),
	}
	if cfg.DatabaseURL == "" {
		cfg.DatabaseURL = "postgres://ticket:ticket@localhost:5432/ticket_allocation?sslmode=disable"
	}
	if cfg.Port == "" {
		cfg.Port = "3000"
	}
	return cfg, nil
}

// would put this in a convert util package
func strPtr(s string) *string { return &s }

func writeJSONAPIError(w http.ResponseWriter, status int, doc api.ErrorDocument) {
	w.Header().Set("Content-Type", "application/vnd.api+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(doc)
}
