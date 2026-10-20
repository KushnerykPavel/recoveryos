package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	demohandler "github.com/KushnerykPavel/recoveryos/api/internal/handlers/demo"
	"github.com/KushnerykPavel/recoveryos/api/internal/repo/postgres"
	"github.com/KushnerykPavel/recoveryos/api/internal/service/paypal"
	demouc "github.com/KushnerykPavel/recoveryos/api/internal/usecase/demo"
)

func Run(ctx context.Context, config Config, logger *slog.Logger) error {
	db, err := pgxpool.New(ctx, config.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database pool: %w", err)
	}
	defer db.Close()
	if err := db.Ping(ctx); err != nil {
		return fmt.Errorf("connect database: %w", err)
	}

	store := postgres.New(db, config.DailyPayoutLimit)
	provider := paypal.New(config.PayPal, nil)
	service := demouc.NewService(store, provider, config.PollInterval, config.WaitTimeout)
	handler := demohandler.New(service, config.CookieSecure, logger)
	server := &http.Server{
		Addr:              config.Address,
		Handler:           buildRouter(handler, store, logger),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      40 * time.Second,
	}
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.ListenAndServe() }()
	logger.Info("API listening", "address", config.Address)
	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("HTTP server: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("HTTP shutdown: %w", err)
		}
		return nil
	}
}
