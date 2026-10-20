package api_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	recoveryhandler "github.com/KushnerykPavel/recoveryos/api/internal/handlers/recovery"
	"github.com/KushnerykPavel/recoveryos/api/internal/repo/postgres"
	"github.com/KushnerykPavel/recoveryos/api/internal/service/paypal"
	recoveryuc "github.com/KushnerykPavel/recoveryos/api/internal/usecase/recovery"
)

type scriptedTransport func(*http.Request) (*http.Response, error)

func (f scriptedTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPostgresRecoveryWorkflow(t *testing.T) {
	databaseURL := os.Getenv("TEST_DB_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DB_URL to a disposable PostgreSQL database ending in _test")
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(config.ConnConfig.Database, "_test") {
		t.Fatal("TEST_DB_URL database name must end in _test")
	}

	sqlDB, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpContext(t.Context(), sqlDB, "../../sql"); err != nil {
		t.Fatal(err)
	}
	db, err := pgxpool.New(t.Context(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(t.Context(), `TRUNCATE incidents, payout_events, payout_records, daily_payout_counts, sessions RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}

	var createCalls atomic.Int32
	var senderItemID string
	transport := scriptedTransport(func(r *http.Request) (*http.Response, error) {
		body := ""
		status := http.StatusOK
		switch r.URL.Path {
		case "/v1/oauth2/token":
			body = `{"access_token":"test-token","expires_in":3600}`
		case "/v1/payments/payouts":
			createCalls.Add(1)
			var request struct {
				Items []struct {
					SenderItemID string `json:"sender_item_id"`
				} `json:"items"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			if len(request.Items) != 1 {
				t.Fatalf("expected one payout item, got %d", len(request.Items))
			}
			senderItemID = request.Items[0].SenderItemID
			status = http.StatusCreated
			body = `{"batch_header":{"payout_batch_id":"batch-1"}}`
		case "/v1/payments/payouts/batch-1":
			body = fmt.Sprintf(`{"batch_header":{"payout_batch_id":"batch-1"},"items":[{"payout_item_id":"provider-item-1","transaction_status":"SUCCESS","payout_item":{"sender_item_id":%q}}]}`, senderItemID)
		default:
			status = http.StatusNotFound
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: r}, nil
	})
	provider := paypal.New(paypal.Config{BaseURL: "https://paypal.test", ClientID: "client", ClientSecret: "secret",
		Receiver: "receiver@example.com", Amount: "0.01", Currency: "USD"}, &http.Client{Transport: transport})
	service := recoveryuc.NewService(postgres.New(db, 1), provider, 0, 0)
	handler := recoveryhandler.New(service, false, slog.New(slog.NewTextHandler(io.Discard, nil)))
	mux := http.NewServeMux()
	handler.Register(mux)
	_, _, cookie := request(t, mux, http.MethodPost, "/api/sessions", nil)
	_, snapshot, _ := requestWithScenario(t, mux, http.MethodPost, "/api/payouts", cookie, "missed-update")
	if snapshot.Payout == nil || snapshot.Payout.Status != "stale" || snapshot.Incident == nil {
		t.Fatalf("missing incident after provider SUCCESS: %+v", snapshot)
	}
	_, _, _ = request(t, mux, http.MethodPost, "/api/payouts", cookie)
	_, _, otherCookie := request(t, mux, http.MethodPost, "/api/sessions", nil)
	status, _, _ := request(t, mux, http.MethodPost, "/api/payouts", otherCookie)
	if status != http.StatusTooManyRequests {
		t.Fatalf("daily payout limit returned %d", status)
	}

	var payouts, incidents, injected int
	if err := db.QueryRow(t.Context(), `SELECT count(*) FROM payout_records`).Scan(&payouts); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(t.Context(), `SELECT count(*) FROM incidents`).Scan(&incidents); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(t.Context(), `SELECT count(*) FROM payout_events WHERE source = 'injected'`).Scan(&injected); err != nil {
		t.Fatal(err)
	}
	if payouts != 1 || incidents != 1 || injected != 1 || createCalls.Load() != 1 {
		t.Fatalf("payouts=%d incidents=%d injected=%d provider creates=%d", payouts, incidents, injected, createCalls.Load())
	}
}
