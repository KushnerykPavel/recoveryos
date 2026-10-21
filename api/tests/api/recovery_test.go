package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/KushnerykPavel/recoveryos/api/internal/domain/recovery"
	recoveryhandler "github.com/KushnerykPavel/recoveryos/api/internal/handlers/recovery"
	recoveryuc "github.com/KushnerykPavel/recoveryos/api/internal/usecase/recovery"
)

type memoryStore struct {
	mu             sync.Mutex
	setBatchError  error
	sessions       map[string]recovery.Session
	payouts        map[string]recovery.Payout
	items          map[string]recovery.Incident
	events         map[string][]recovery.Event
	investigations map[string]recovery.Investigation
}

func newMemoryStore() *memoryStore {
	return &memoryStore{sessions: map[string]recovery.Session{}, payouts: map[string]recovery.Payout{},
		items: map[string]recovery.Incident{}, events: map[string][]recovery.Event{}, investigations: map[string]recovery.Investigation{}}
}

func (s *memoryStore) CreateSession(_ context.Context, hash []byte, expiresAt time.Time) (recovery.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session := recovery.Session{ID: string(rune('a' + len(s.sessions))), CreatedAt: time.Now(), ExpiresAt: expiresAt}
	s.sessions[string(hash)] = session
	return session, nil
}

func (s *memoryStore) Snapshot(_ context.Context, hash []byte) (recovery.Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[string(hash)]
	if !ok {
		return recovery.Snapshot{}, recoveryuc.ErrSessionNotFound
	}
	snapshot := recovery.Snapshot{Session: session, Events: append([]recovery.Event{}, s.events[session.ID]...)}
	if payout, ok := s.payouts[session.ID]; ok {
		snapshot.Payout = &payout
		if incident, ok := s.items[payout.ID]; ok {
			snapshot.Incident = &incident
			if result, ok := s.investigations[session.ID]; ok {
				snapshot.Investigation = &result
			}
		}
	}
	return snapshot, nil
}

func (s *memoryStore) SaveInvestigation(_ context.Context, sessionID, incidentID string, result recovery.Investigation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	incident, ok := s.items[s.payouts[sessionID].ID]
	if !ok || incident.ID != incidentID || incident.Status != "open" {
		return errors.New("incident is no longer open")
	}
	s.investigations[sessionID] = result
	s.events[sessionID] = append(s.events[sessionID], recovery.Event{
		ID: int64(len(s.events[sessionID]) + 1), Source: "system", Kind: "investigation_completed",
		Details: "Diagnosis mode: " + result.Origin, OccurredAt: time.Now(),
	})
	return nil
}

func (s *memoryStore) RecordRejectedApproval(_ context.Context, sessionID, payoutID, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	payout, ok := s.payouts[sessionID]
	if payoutID != "" && (!ok || payout.ID != payoutID) {
		return errors.New("payout record does not belong to session")
	}
	s.events[sessionID] = append(s.events[sessionID], recovery.Event{
		ID: int64(len(s.events[sessionID]) + 1), Source: "system", Kind: "record_correction_rejected",
		Details: reason, OccurredAt: time.Now(),
	})
	return nil
}

func (s *memoryStore) ReservePayout(_ context.Context, sessionID, scenario string) (recovery.Payout, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if payout, ok := s.payouts[sessionID]; ok {
		return payout, false, nil
	}
	payout := recovery.Payout{ID: "payout-" + sessionID, SessionID: sessionID,
		CreationKey: "recoveryos-" + sessionID, SenderItemID: "item-" + sessionID, Status: "creating", Scenario: scenario}
	s.payouts[sessionID] = payout
	return payout, true, nil
}

func (s *memoryStore) MarkCreateUncertain(_ context.Context, payoutID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, payout := range s.payouts {
		if payout.ID == payoutID {
			payout.Status = "provider_unknown"
			s.payouts[key] = payout
			return nil
		}
	}
	return errors.New("payout not found")
}

func (s *memoryStore) SetBatch(_ context.Context, payoutID, batchID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.setBatchError != nil {
		return s.setBatchError
	}
	for key, payout := range s.payouts {
		if payout.ID == payoutID {
			payout.BatchID, payout.Status = batchID, "pending"
			s.payouts[key] = payout
			return nil
		}
	}
	return errors.New("payout not found")
}

func (s *memoryStore) ObserveItem(_ context.Context, payoutID, itemID, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, payout := range s.payouts {
		if payout.ID == payoutID {
			payout.ItemID, payout.ProviderStatus = itemID, status
			s.payouts[key] = payout
			return nil
		}
	}
	return errors.New("payout not found")
}

func (s *memoryStore) ObserveSuccess(_ context.Context, payoutID, itemID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, payout := range s.payouts {
		if payout.ID != payoutID {
			continue
		}
		if payout.Status == "stale" || payout.Status == "completed" || payout.ProviderStatus == "SUCCESS" {
			return nil
		}
		payout.ItemID, payout.ProviderStatus, payout.Status = itemID, "SUCCESS", "completed"
		if payout.Scenario == recovery.ScenarioMissedUpdate {
			payout.Status = "pending"
		}
		s.payouts[key] = payout
		s.events[key] = append(s.events[key], recovery.Event{Source: "paypal", Kind: "item_observed"})
		if payout.Scenario == recovery.ScenarioMissedUpdate {
			s.items[payoutID] = recovery.Incident{ID: "incident-" + key, PayoutID: payoutID, Status: "open"}
			s.events[key] = append(s.events[key], recovery.Event{Source: "injected", Kind: "internal_update_skipped"})
		} else {
			s.events[key] = append(s.events[key], recovery.Event{Source: "system", Kind: "internal_record_updated"})
		}
		return nil
	}
	return errors.New("payout not found")
}

type fakeProvider struct {
	mu           sync.Mutex
	createCalls  int
	createError  error
	readError    error
	statuses     []string
	mismatchItem bool
}

func (p *fakeProvider) CreatePayout(_ context.Context, _, _ string) (recoveryuc.Batch, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.createCalls++
	if p.createError != nil {
		return recoveryuc.Batch{}, p.createError
	}
	return recoveryuc.Batch{ID: "batch-1"}, nil
}

func (p *fakeProvider) GetBatchItem(_ context.Context, batchID string) (recoveryuc.Item, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.readError != nil {
		return recoveryuc.Item{}, p.readError
	}
	status := p.statuses[0]
	if len(p.statuses) > 1 {
		p.statuses = p.statuses[1:]
	}
	senderItemID := "item-a"
	if p.mismatchItem {
		senderItemID = "wrong-item"
	}
	return recoveryuc.Item{ID: "provider-item-1", BatchID: batchID, SenderItemID: senderItemID, Status: status}, nil
}

func newServer(provider *fakeProvider) http.Handler {
	return newServerWithStore(provider, newMemoryStore())
}

func newServerWithStore(provider *fakeProvider, store *memoryStore) http.Handler {
	service := recoveryuc.NewService(store, provider, 0, 0)
	handler := recoveryhandler.New(service, false, slog.New(slog.NewTextHandler(io.Discard, nil)))
	mux := http.NewServeMux()
	handler.Register(mux)
	return mux
}

func TestFailedBatchPersistenceLeavesOneUncertainPayout(t *testing.T) {
	provider := &fakeProvider{statuses: []string{"SUCCESS"}}
	store := newMemoryStore()
	store.setBatchError = errors.New("database write failed")
	server := newServerWithStore(provider, store)
	_, _, cookie := request(t, server, http.MethodPost, "/api/sessions", nil)
	status, first, _ := request(t, server, http.MethodPost, "/api/payouts", cookie)
	if status != http.StatusOK || first.Payout == nil || first.Payout.Status != "provider_unknown" ||
		first.Payout.CreationKey == "" || first.Payout.BatchID != "" {
		t.Fatalf("failed batch persistence did not keep an uncertain record: status=%d snapshot=%+v", status, first)
	}
	_, again, _ := request(t, server, http.MethodPost, "/api/payouts", cookie)
	if again.Payout == nil || again.Payout.CreationKey != first.Payout.CreationKey || provider.createCalls != 1 {
		t.Fatalf("failed batch persistence caused another payout attempt: %+v", again)
	}
}

func request(t *testing.T, server http.Handler, method, path string, cookie *http.Cookie) (int, recovery.Snapshot, *http.Cookie) {
	return requestWithScenario(t, server, method, path, cookie, "")
}

func requestWithScenario(t *testing.T, server http.Handler, method, path string, cookie *http.Cookie, scenario string) (int, recovery.Snapshot, *http.Cookie) {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, nil)
	if scenario != "" {
		req.Header.Set("X-RecoveryOS-Scenario", scenario)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, req)
	resp := recorder.Result()
	defer resp.Body.Close()
	var snapshot recovery.Snapshot
	if resp.StatusCode < 300 {
		if err := json.NewDecoder(resp.Body).Decode(&snapshot); err != nil {
			t.Fatal(err)
		}
	}
	var issued *http.Cookie
	if cookies := resp.Cookies(); len(cookies) > 0 {
		issued = cookies[0]
	}
	return resp.StatusCode, snapshot, issued
}

func TestRecoveryWorkflowKeepsSessionsIsolatedAndCreatesOnePayout(t *testing.T) {
	provider := &fakeProvider{statuses: []string{"SUCCESS"}}
	server := newServer(provider)
	_, first, firstCookie := request(t, server, http.MethodPost, "/api/sessions", nil)
	_, second, secondCookie := request(t, server, http.MethodPost, "/api/sessions", nil)
	if first.Session.ID == second.Session.ID || firstCookie == nil || secondCookie == nil {
		t.Fatal("sessions were not isolated")
	}
	status, started, _ := requestWithScenario(t, server, http.MethodPost, "/api/payouts", firstCookie, recovery.ScenarioMissedUpdate)
	if status != http.StatusOK || started.Payout == nil || started.Payout.Status != "pending" ||
		started.Payout.ProviderStatus != "SUCCESS" || started.Incident == nil || len(started.Events) != 2 ||
		started.Events[0].Source != "paypal" || started.Events[0].Kind != "item_observed" ||
		started.Events[1].Source != "injected" || started.Events[1].Kind != "internal_update_skipped" {
		t.Fatalf("unexpected completed scenario: status=%d snapshot=%+v", status, started)
	}
	_, again, _ := request(t, server, http.MethodPost, "/api/payouts", firstCookie)
	if again.Incident == nil || len(again.Events) != 2 || provider.createCalls != 1 {
		t.Fatal("repeated start created another payout or incident")
	}
	_, other, _ := request(t, server, http.MethodGet, "/api/sessions/current", secondCookie)
	if other.Payout != nil || other.Incident != nil || len(other.Events) != 0 {
		t.Fatal("second session can see first session's state")
	}
}

func TestSuccessfulPayoutUpdatesRecordWithoutInjectedIncident(t *testing.T) {
	provider := &fakeProvider{statuses: []string{"SUCCESS"}}
	server := newServer(provider)
	_, _, cookie := request(t, server, http.MethodPost, "/api/sessions", nil)
	status, snapshot, _ := request(t, server, http.MethodPost, "/api/payouts", cookie)
	if status != http.StatusOK || snapshot.Payout == nil || snapshot.Payout.Status != "completed" ||
		snapshot.Payout.ProviderStatus != "SUCCESS" || snapshot.Incident != nil || len(snapshot.Events) != 2 ||
		snapshot.Events[0].Source != "paypal" || snapshot.Events[1].Kind != "internal_record_updated" {
		t.Fatalf("successful payout did not complete normally: status=%d snapshot=%+v", status, snapshot)
	}
	_, again, _ := request(t, server, http.MethodPost, "/api/payouts", cookie)
	if again.Payout == nil || again.Payout.Status != "completed" || len(again.Events) != 2 || provider.createCalls != 1 {
		t.Fatalf("repeated payout request changed a completed payout: %+v", again)
	}
}

func TestInjectedIncidentKeepsVerifiedSuccessOnRepeatedStart(t *testing.T) {
	provider := &fakeProvider{statuses: []string{"SUCCESS", "FAILED"}}
	server := newServer(provider)
	_, _, cookie := request(t, server, http.MethodPost, "/api/sessions", nil)
	_, first, _ := requestWithScenario(t, server, http.MethodPost, "/api/payouts", cookie, recovery.ScenarioMissedUpdate)
	_, again, _ := request(t, server, http.MethodPost, "/api/payouts", cookie)
	if first.Payout == nil || again.Payout == nil || again.Payout.ProviderStatus != "SUCCESS" ||
		again.Payout.Status != "pending" || again.Incident == nil || len(again.Events) != len(first.Events) {
		t.Fatalf("repeated start changed verified incident evidence: %+v", again)
	}
}

func TestPayoutScenarioHeaderIsValidatedAndCannotChangeExistingPayout(t *testing.T) {
	provider := &fakeProvider{statuses: []string{"SUCCESS"}}
	server := newServer(provider)
	_, _, cookie := request(t, server, http.MethodPost, "/api/sessions", nil)

	status, _, _ := requestWithScenario(t, server, http.MethodPost, "/api/payouts", cookie, "unknown")
	if status != http.StatusBadRequest || provider.createCalls != 0 {
		t.Fatalf("invalid scenario returned %d or created a payout", status)
	}

	status, standard, _ := request(t, server, http.MethodPost, "/api/payouts", cookie)
	if status != http.StatusOK || standard.Payout == nil || standard.Payout.Scenario != recovery.ScenarioStandard {
		t.Fatalf("standard payout not created: status=%d snapshot=%+v", status, standard)
	}

	status, _, _ = requestWithScenario(t, server, http.MethodPost, "/api/payouts", cookie, recovery.ScenarioMissedUpdate)
	if status != http.StatusConflict || provider.createCalls != 1 {
		t.Fatalf("scenario change returned %d or created another payout", status)
	}
	_, current, _ := request(t, server, http.MethodGet, "/api/sessions/current", cookie)
	if current.Payout == nil || current.Payout.Scenario != recovery.ScenarioStandard || current.Incident != nil {
		t.Fatalf("scenario change altered the stored payout: %+v", current)
	}
}

func TestPendingItemNeedsLaterReconciliation(t *testing.T) {
	provider := &fakeProvider{statuses: []string{"PENDING", "SUCCESS"}}
	server := newServer(provider)
	_, _, cookie := request(t, server, http.MethodPost, "/api/sessions", nil)
	_, pending, _ := requestWithScenario(t, server, http.MethodPost, "/api/payouts", cookie, recovery.ScenarioMissedUpdate)
	if pending.Incident != nil || pending.Payout == nil || pending.Payout.ProviderStatus != "PENDING" ||
		pending.Payout.ItemID != "provider-item-1" || pending.Payout.BatchID != "batch-1" ||
		pending.Payout.CreationKey == "" {
		t.Fatalf("pending item opened incident: %+v", pending)
	}
	_, reconciled, _ := request(t, server, http.MethodPost, "/api/payouts/reconcile", cookie)
	if reconciled.Incident == nil || reconciled.Payout == nil || reconciled.Payout.ItemID != pending.Payout.ItemID ||
		reconciled.Payout.ProviderStatus != "SUCCESS" || provider.createCalls != 1 {
		t.Fatal("later SUCCESS did not open incident safely")
	}
}

func TestNonSuccessItemIsShownWithoutClaimingCompletion(t *testing.T) {
	for _, itemStatus := range []string{"FAILED", "UNCLAIMED", "UNKNOWN"} {
		t.Run(itemStatus, func(t *testing.T) {
			provider := &fakeProvider{statuses: []string{itemStatus}}
			server := newServer(provider)
			_, _, cookie := request(t, server, http.MethodPost, "/api/sessions", nil)
			status, snapshot, _ := requestWithScenario(t, server, http.MethodPost, "/api/payouts", cookie, recovery.ScenarioMissedUpdate)
			if status != http.StatusOK || snapshot.Payout == nil || snapshot.Payout.ItemID != "provider-item-1" ||
				snapshot.Payout.ProviderStatus != itemStatus || snapshot.Payout.Status != "pending" || snapshot.Incident != nil {
				t.Fatalf("non-success item was treated as complete: status=%d snapshot=%+v", status, snapshot)
			}
			_, again, _ := request(t, server, http.MethodPost, "/api/payouts", cookie)
			if again.Payout == nil || again.Payout.CreationKey != snapshot.Payout.CreationKey || provider.createCalls != 1 {
				t.Fatalf("repeat request changed the payout: %+v", again)
			}
		})
	}
}

func TestUnknownCreateOutcomeDoesNotRetryWithAnotherPayout(t *testing.T) {
	provider := &fakeProvider{createError: errors.New("network timeout")}
	server := newServer(provider)
	_, _, cookie := request(t, server, http.MethodPost, "/api/sessions", nil)
	_, first, _ := request(t, server, http.MethodPost, "/api/payouts", cookie)
	_, second, _ := request(t, server, http.MethodPost, "/api/payouts", cookie)
	if first.Payout == nil || first.Payout.Status != "provider_unknown" ||
		second.Payout == nil || second.Payout.CreationKey != first.Payout.CreationKey || provider.createCalls != 1 {
		t.Fatal("ambiguous PayPal outcome led to a second payout attempt")
	}
}

func TestMismatchedProviderItemCannotOpenIncident(t *testing.T) {
	provider := &fakeProvider{statuses: []string{"SUCCESS"}, mismatchItem: true}
	server := newServer(provider)
	_, _, cookie := request(t, server, http.MethodPost, "/api/sessions", nil)
	status, _, _ := requestWithScenario(t, server, http.MethodPost, "/api/payouts", cookie, recovery.ScenarioMissedUpdate)
	if status != http.StatusBadGateway {
		t.Fatalf("mismatched provider item returned %d", status)
	}
	_, snapshot, _ := request(t, server, http.MethodGet, "/api/sessions/current", cookie)
	if snapshot.Incident != nil || snapshot.Payout == nil || snapshot.Payout.Status != "pending" {
		t.Fatal("mismatched item changed payout or incident")
	}
}
