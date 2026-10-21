package api_test

import (
	"database/sql"
	"encoding/json"
	"errors"
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

func postgresTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
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
	t.Cleanup(func() { sqlDB.Close() })
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
	t.Cleanup(db.Close)
	if _, err := db.Exec(t.Context(), `TRUNCATE incidents, payout_events, payout_records, daily_payout_counts, sessions RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestPostgresRecoveryWorkflow(t *testing.T) {
	db := postgresTestPool(t)

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
	status, first, cookie := request(t, mux, http.MethodPost, "/api/sessions", nil)
	if status != http.StatusCreated || cookie == nil || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("anonymous session response: status=%d cookie=%+v", status, cookie)
	}
	status, second, otherCookie := request(t, mux, http.MethodPost, "/api/sessions", nil)
	if status != http.StatusCreated || otherCookie == nil || first.Session.ID == second.Session.ID || cookie.Value == otherCookie.Value {
		t.Fatal("anonymous visitors received the same session identity")
	}
	status, _, _ = request(t, mux, http.MethodGet, "/api/sessions/current", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("reading without a cookie returned %d", status)
	}
	_, snapshot, _ := requestWithScenario(t, mux, http.MethodPost, "/api/payouts", cookie, "missed-update")
	if snapshot.Payout == nil || snapshot.Payout.Status != "pending" || snapshot.Payout.ProviderStatus != "SUCCESS" || snapshot.Incident == nil {
		t.Fatalf("missing incident after provider SUCCESS: %+v", snapshot)
	}
	if len(snapshot.Events) != 3 || snapshot.Events[1].Source != "paypal" ||
		snapshot.Events[1].Kind != "item_observed" || snapshot.Events[1].Details != "provider-item-1:SUCCESS" ||
		snapshot.Events[2].Source != "injected" || snapshot.Events[2].Kind != "internal_update_skipped" {
		t.Fatalf("provider evidence and injected fault were not distinct: %+v", snapshot.Events)
	}
	_, _, _ = request(t, mux, http.MethodPost, "/api/payouts", cookie)
	status, other, _ := request(t, mux, http.MethodGet, "/api/sessions/current?session_id="+first.Session.ID, otherCookie)
	if status != http.StatusOK || other.Session.ID != second.Session.ID || other.Payout != nil || other.Incident != nil || len(other.Events) != 0 {
		t.Fatalf("another visitor read first session state: status=%d snapshot=%+v", status, other)
	}
	status, _, _ = request(t, mux, http.MethodPost, "/api/payouts", otherCookie)
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

func TestPostgresPayoutItemOutcomes(t *testing.T) {
	cases := []struct {
		name         string
		statuses     []string
		createError  bool
		mismatchItem bool
		wantHTTP     int
		wantRecord   string
		wantProvider string
		wantIncident bool
	}{
		{name: "pending then success", statuses: []string{"PENDING", "SUCCESS"}, wantHTTP: http.StatusOK, wantRecord: "pending", wantProvider: "SUCCESS", wantIncident: true},
		{name: "failed item", statuses: []string{"FAILED"}, wantHTTP: http.StatusOK, wantRecord: "pending", wantProvider: "FAILED"},
		{name: "unclaimed item", statuses: []string{"UNCLAIMED"}, wantHTTP: http.StatusOK, wantRecord: "pending", wantProvider: "UNCLAIMED"},
		{name: "unknown item status", statuses: []string{"UNKNOWN"}, wantHTTP: http.StatusOK, wantRecord: "pending", wantProvider: "UNKNOWN"},
		{name: "uncertain creation", createError: true, wantHTTP: http.StatusOK, wantRecord: "provider_unknown"},
		{name: "mismatched item", statuses: []string{"SUCCESS"}, mismatchItem: true, wantHTTP: http.StatusBadGateway, wantRecord: "pending"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := postgresTestPool(t)
			var createCalls, readCalls int
			var creationKey, senderItemID string
			transport := scriptedTransport(func(r *http.Request) (*http.Response, error) {
				body := ""
				status := http.StatusOK
				switch r.URL.Path {
				case "/v1/oauth2/token":
					body = `{"access_token":"test-token","expires_in":3600}`
				case "/v1/payments/payouts":
					createCalls++
					var payoutRequest struct {
						SenderBatchHeader struct {
							SenderBatchID string `json:"sender_batch_id"`
						} `json:"sender_batch_header"`
						Items []struct {
							SenderItemID string `json:"sender_item_id"`
						} `json:"items"`
					}
					if err := json.NewDecoder(r.Body).Decode(&payoutRequest); err != nil {
						t.Fatal(err)
					}
					if len(payoutRequest.Items) != 1 {
						t.Fatalf("expected one payout item, got %d", len(payoutRequest.Items))
					}
					creationKey, senderItemID = payoutRequest.SenderBatchHeader.SenderBatchID, payoutRequest.Items[0].SenderItemID
					if tc.createError {
						return nil, errors.New("PayPal connection lost")
					}
					status = http.StatusCreated
					body = `{"batch_header":{"payout_batch_id":"batch-1"}}`
				case "/v1/payments/payouts/batch-1":
					itemStatus := tc.statuses[min(readCalls, len(tc.statuses)-1)]
					readCalls++
					itemSenderID := senderItemID
					if tc.mismatchItem {
						itemSenderID = "wrong-item"
					}
					body = fmt.Sprintf(`{"batch_header":{"payout_batch_id":"batch-1"},"items":[{"payout_item_id":"provider-item-1","transaction_status":%q,"payout_item":{"sender_item_id":%q}}]}`, itemStatus, itemSenderID)
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
			status, first, _ := requestWithScenario(t, mux, http.MethodPost, "/api/payouts", cookie, "missed-update")
			if status != tc.wantHTTP {
				t.Fatalf("create returned HTTP %d, want %d", status, tc.wantHTTP)
			}
			if len(tc.statuses) == 2 {
				if first.Payout == nil || first.Payout.ProviderStatus != "PENDING" || first.Payout.ItemID != "provider-item-1" || first.Incident != nil {
					t.Fatalf("first item observation was not pending: %+v", first)
				}
				status, first, _ = request(t, mux, http.MethodPost, "/api/payouts/reconcile", cookie)
				if status != http.StatusOK {
					t.Fatalf("reconcile returned HTTP %d", status)
				}
			}
			if tc.mismatchItem {
				_, first, _ = request(t, mux, http.MethodGet, "/api/sessions/current", cookie)
			}
			if first.Payout == nil || first.Payout.CreationKey != creationKey || first.Payout.Status != tc.wantRecord ||
				first.Payout.ProviderStatus != tc.wantProvider || (first.Incident != nil) != tc.wantIncident {
				t.Fatalf("unexpected payout item outcome: %+v", first)
			}
			if tc.createError {
				if first.Payout.BatchID != "" || first.Payout.ItemID != "" {
					t.Fatalf("uncertain creation claimed provider evidence: %+v", first.Payout)
				}
			} else if !tc.mismatchItem && (first.Payout.BatchID != "batch-1" || first.Payout.ItemID != "provider-item-1") {
				t.Fatalf("provider identifiers missing from snapshot: %+v", first.Payout)
			}
			_, _, _ = request(t, mux, http.MethodPost, "/api/payouts", cookie)
			_, again, _ := request(t, mux, http.MethodGet, "/api/sessions/current", cookie)
			if again.Payout == nil || again.Payout.CreationKey != creationKey || createCalls != 1 {
				t.Fatalf("repeat request changed the stable creation key or created another payout: %+v calls=%d", again, createCalls)
			}
		})
	}
}
