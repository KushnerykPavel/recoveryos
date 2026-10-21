package api_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	recoveryhandler "github.com/KushnerykPavel/recoveryos/api/internal/handlers/recovery"
	"github.com/KushnerykPavel/recoveryos/api/internal/repo/postgres"
	"github.com/KushnerykPavel/recoveryos/api/internal/service/paypal"
	"github.com/KushnerykPavel/recoveryos/api/internal/usecase/investigation"
	recoveryuc "github.com/KushnerykPavel/recoveryos/api/internal/usecase/recovery"
)

type unavailableModel struct{}

func (unavailableModel) Respond(context.Context, investigation.Request) (investigation.Response, error) {
	return investigation.Response{}, errors.New("model unavailable")
}

func TestPostgresConcurrentApprovalsCommitOneCorrection(t *testing.T) {
	db := postgresTestPool(t)
	store := postgres.New(db, 1)
	provider := &fakeProvider{statuses: []string{"SUCCESS"}}
	service := recoveryuc.NewService(store, provider, 0, 0).WithBackup(recoveryuc.BackupEvidence{
		CreationKey: "precreated-key", SenderItemID: "item-a", BatchID: "batch-1", ItemID: "provider-item-1",
	})
	investigator := investigation.New(service, provider, unavailableModel{}, store)
	handler := recoveryhandler.New(service, false, slog.New(slog.NewTextHandler(io.Discard, nil))).WithInvestigator(investigator)
	mux := http.NewServeMux()
	handler.Register(mux)
	_, _, firstCookie := request(t, mux, http.MethodPost, "/api/sessions", nil)
	_, started, cookie := request(t, mux, http.MethodPost, "/api/sessions/backup", firstCookie)
	if status, result := investigateRequest(t, mux, cookie); status != http.StatusOK || result.ProposedAction != "correct_payout_record" {
		t.Fatalf("investigation: %d %+v", status, result)
	}
	const attempts = 8
	start := make(chan struct{})
	results := make(chan int, attempts)
	var wg sync.WaitGroup
	for range attempts {
		wg.Go(func() {
			<-start
			status, _ := approvalRequest(t, mux, cookie, started.Incident.ID)
			results <- status
		})
	}
	close(start)
	wg.Wait()
	close(results)
	successes := 0
	for status := range results {
		if status == http.StatusNoContent {
			successes++
		} else if status != http.StatusConflict {
			t.Fatalf("approval status %d", status)
		}
	}
	_, after, _ := request(t, mux, http.MethodGet, "/api/sessions/current", cookie)
	var corrections, approvals, verifications int
	if err := db.QueryRow(t.Context(), `SELECT
		count(*) FILTER (WHERE kind = 'record_corrected'),
		count(*) FILTER (WHERE kind = 'operator_approval'),
		count(*) FILTER (WHERE kind = 'provider_verification')
		FROM payout_events WHERE payout_id = $1`, started.Payout.ID).Scan(&corrections, &approvals, &verifications); err != nil {
		t.Fatal(err)
	}
	if successes != 1 || corrections != 1 || approvals != 1 || verifications != 1 ||
		after.Payout.Status != "corrected" || after.Incident.Status != "resolved" || provider.createCalls != 0 {
		t.Fatalf("successes=%d audit=%d/%d/%d snapshot=%+v creates=%d", successes, corrections, approvals, verifications, after, provider.createCalls)
	}
}

func TestPostgresWrongSessionApprovalIsRecordedWithoutPayout(t *testing.T) {
	db := postgresTestPool(t)
	store := postgres.New(db, 1)
	provider := &fakeProvider{statuses: []string{"SUCCESS"}}
	service := recoveryuc.NewService(store, provider, 0, 0)
	investigator := investigation.New(service, provider, nil, store)
	handler := recoveryhandler.New(service, false, slog.New(slog.NewTextHandler(io.Discard, nil))).WithInvestigator(investigator)
	mux := http.NewServeMux()
	handler.Register(mux)
	_, _, cookie := request(t, mux, http.MethodPost, "/api/sessions", nil)
	status, reason := approvalRequest(t, mux, cookie, "00000000-0000-0000-0000-000000000000")
	if status != http.StatusConflict || !strings.Contains(reason, "session") {
		t.Fatalf("wrong-session approval: %d %q", status, reason)
	}
	_, snapshot, _ := request(t, mux, http.MethodGet, "/api/sessions/current", cookie)
	if snapshot.Payout != nil || len(snapshot.Events) != 1 || snapshot.Events[0].Kind != "record_correction_rejected" {
		t.Fatalf("session audit event missing: %+v", snapshot)
	}
}

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

func TestPostgresBackupBypassesDailyCreationLimitWithIsolatedRecords(t *testing.T) {
	db := postgresTestPool(t)
	provider := &fakeProvider{statuses: []string{"SUCCESS"}}
	service := recoveryuc.NewService(postgres.New(db, 1), provider, 0, 0).WithBackup(recoveryuc.BackupEvidence{
		CreationKey: "precreated-key", SenderItemID: "item-a", BatchID: "precreated-batch", ItemID: "provider-item-1",
	})
	handler := recoveryhandler.New(service, false, slog.New(slog.NewTextHandler(io.Discard, nil)))
	mux := http.NewServeMux()
	handler.Register(mux)
	_, _, firstCookie := request(t, mux, http.MethodPost, "/api/sessions", nil)
	_, _, secondCookie := request(t, mux, http.MethodPost, "/api/sessions", nil)
	_, _, _ = request(t, mux, http.MethodPost, "/api/payouts", firstCookie)
	status, _, _ := request(t, mux, http.MethodPost, "/api/payouts", secondCookie)
	if status != http.StatusTooManyRequests {
		t.Fatalf("daily limit returned %d", status)
	}
	_, first, firstBackupCookie := request(t, mux, http.MethodPost, "/api/sessions/backup", secondCookie)
	_, second, secondBackupCookie := request(t, mux, http.MethodPost, "/api/sessions/backup", secondCookie)
	if firstBackupCookie == nil || secondBackupCookie == nil || first.Payout == nil || second.Payout == nil ||
		first.Incident == nil || second.Incident == nil || first.Payout.ID == second.Payout.ID ||
		first.Incident.ID == second.Incident.ID || first.Payout.ItemID != second.Payout.ItemID || provider.createCalls != 1 {
		t.Fatalf("backups were not isolated or another payout was created: first=%+v second=%+v calls=%d", first, second, provider.createCalls)
	}
	var count int
	if err := db.QueryRow(t.Context(), `SELECT count FROM daily_payout_counts LIMIT 1`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("backup consumed daily creation quota: %d", count)
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
