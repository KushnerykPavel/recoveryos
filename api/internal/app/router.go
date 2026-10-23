package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	recoveryhandler "github.com/KushnerykPavel/recoveryos/api/internal/handlers/recovery"
	webhookhandler "github.com/KushnerykPavel/recoveryos/api/internal/handlers/webhook"
)

type pinger interface{ Ping(context.Context) error }

func buildRouter(recovery *recoveryhandler.Handler, webhooks *webhookhandler.Handler, db pinger, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			logger.Error("database readiness failed", "error", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "database unavailable"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	recovery.Register(mux)
	if webhooks != nil {
		webhooks.Register(mux)
	}
	return mux
}
