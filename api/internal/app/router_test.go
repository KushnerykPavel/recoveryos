package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	recoveryhandler "github.com/KushnerykPavel/recoveryos/api/internal/handlers/recovery"
)

type healthyDB struct{}

func (healthyDB) Ping(context.Context) error { return nil }

func TestWebhookRouteDisabledWithoutID(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := buildRouter(recoveryhandler.New(nil, false, logger), nil, healthyDB{}, logger)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/webhooks/paypal", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("disabled webhook route returned %d", response.Code)
	}
}
