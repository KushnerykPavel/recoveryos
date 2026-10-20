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

	"github.com/KushnerykPavel/recoveryos/api/internal/domain/demo"
	demohandler "github.com/KushnerykPavel/recoveryos/api/internal/handlers/demo"
	demouc "github.com/KushnerykPavel/recoveryos/api/internal/usecase/demo"
)

type memoryStore struct {
	mu       sync.Mutex
	sessions map[string]demo.Session
	payouts  map[string]demo.Payout
	items    map[string]demo.Incident
	events   map[string][]demo.Event
}

func newMemoryStore() *memoryStore {
	return &memoryStore{sessions: map[string]demo.Session{}, payouts: map[string]demo.Payout{},
		items: map[string]demo.Incident{}, events: map[string][]demo.Event{}}
}

func (s *memoryStore) CreateSession(_ context.Context, hash []byte, expiresAt time.Time) (demo.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session := demo.Session{ID: string(rune('a' + len(s.sessions))), CreatedAt: time.Now(), ExpiresAt: expiresAt}
	s.sessions[string(hash)] = session
	return session, nil
}

func (s *memoryStore) Snapshot(_ context.Context, hash []byte) (demo.Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[string(hash)]
	if !ok {
		return demo.Snapshot{}, demouc.ErrSessionNotFound
	}
	snapshot := demo.Snapshot{Session: session, Events: append([]demo.Event{}, s.events[session.ID]...)}
	if payout, ok := s.payouts[session.ID]; ok {
		snapshot.Payout = &payout
		if incident, ok := s.items[payout.ID]; ok {
			snapshot.Incident = &incident
		}
	}
	return snapshot, nil
}

func (s *memoryStore) ReservePayout(_ context.Context, sessionID string) (demo.Payout, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if payout, ok := s.payouts[sessionID]; ok {
		return payout, false, nil
	}
	payout := demo.Payout{ID: "payout-" + sessionID, SessionID: sessionID,
		CreationKey: "recoveryos-" + sessionID, SenderItemID: "item-" + sessionID, Status: "creating"}
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
		if payout.Status == "stale" {
			return nil
		}
		payout.ItemID, payout.ProviderStatus, payout.Status = itemID, "SUCCESS", "stale"
		s.payouts[key] = payout
		s.items[payoutID] = demo.Incident{ID: "incident-" + key, PayoutID: payoutID, Status: "open"}
		s.events[key] = append(s.events[key], demo.Event{Source: "paypal", Kind: "item_observed"},
			demo.Event{Source: "injected", Kind: "internal_update_skipped"})
		return nil
	}
	return errors.New("payout not found")
}

type fakeProvider struct {
	mu           sync.Mutex
	createCalls  int
	createError  error
	statuses     []string
	mismatchItem bool
}

func (p *fakeProvider) CreatePayout(_ context.Context, _, _ string) (demouc.Batch, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.createCalls++
	if p.createError != nil {
		return demouc.Batch{}, p.createError
	}
	return demouc.Batch{ID: "batch-1"}, nil
}

func (p *fakeProvider) GetBatchItem(_ context.Context, batchID string) (demouc.Item, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	status := p.statuses[0]
	if len(p.statuses) > 1 {
		p.statuses = p.statuses[1:]
	}
	senderItemID := "item-a"
	if p.mismatchItem {
		senderItemID = "wrong-item"
	}
	return demouc.Item{ID: "provider-item-1", BatchID: batchID, SenderItemID: senderItemID, Status: status}, nil
}

func newServer(provider *fakeProvider) http.Handler {
	service := demouc.NewService(newMemoryStore(), provider, 0, 0)
	handler := demohandler.New(service, false, slog.New(slog.NewTextHandler(io.Discard, nil)))
	mux := http.NewServeMux()
	handler.Register(mux)
	return mux
}

func request(t *testing.T, server http.Handler, method, path string, cookie *http.Cookie) (int, demo.Snapshot, *http.Cookie) {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, req)
	resp := recorder.Result()
	defer resp.Body.Close()
	var snapshot demo.Snapshot
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

func TestDemoWorkflowKeepsSessionsIsolatedAndCreatesOnePayout(t *testing.T) {
	provider := &fakeProvider{statuses: []string{"SUCCESS"}}
	server := newServer(provider)
	_, first, firstCookie := request(t, server, http.MethodPost, "/api/demo/sessions", nil)
	_, second, secondCookie := request(t, server, http.MethodPost, "/api/demo/sessions", nil)
	if first.Session.ID == second.Session.ID || firstCookie == nil || secondCookie == nil {
		t.Fatal("sessions were not isolated")
	}
	status, started, _ := request(t, server, http.MethodPost, "/api/demo/start", firstCookie)
	if status != http.StatusOK || started.Payout == nil || started.Payout.Status != "stale" ||
		started.Payout.ProviderStatus != "SUCCESS" || started.Incident == nil || len(started.Events) != 2 ||
		started.Events[1].Source != "injected" {
		t.Fatalf("unexpected completed scenario: status=%d snapshot=%+v", status, started)
	}
	_, again, _ := request(t, server, http.MethodPost, "/api/demo/start", firstCookie)
	if again.Incident == nil || len(again.Events) != 2 || provider.createCalls != 1 {
		t.Fatal("repeated start created another payout or incident")
	}
	_, other, _ := request(t, server, http.MethodGet, "/api/demo", secondCookie)
	if other.Payout != nil || other.Incident != nil || len(other.Events) != 0 {
		t.Fatal("second session can see first session's state")
	}
}

func TestPendingItemNeedsLaterReconciliation(t *testing.T) {
	provider := &fakeProvider{statuses: []string{"PENDING", "SUCCESS"}}
	server := newServer(provider)
	_, _, cookie := request(t, server, http.MethodPost, "/api/demo/sessions", nil)
	_, pending, _ := request(t, server, http.MethodPost, "/api/demo/start", cookie)
	if pending.Incident != nil || pending.Payout == nil || pending.Payout.ProviderStatus != "PENDING" {
		t.Fatalf("pending item opened incident: %+v", pending)
	}
	_, reconciled, _ := request(t, server, http.MethodPost, "/api/demo/reconcile", cookie)
	if reconciled.Incident == nil || provider.createCalls != 1 {
		t.Fatal("later SUCCESS did not open incident safely")
	}
}

func TestUnknownCreateOutcomeDoesNotRetryWithAnotherPayout(t *testing.T) {
	provider := &fakeProvider{createError: errors.New("network timeout")}
	server := newServer(provider)
	_, _, cookie := request(t, server, http.MethodPost, "/api/demo/sessions", nil)
	_, first, _ := request(t, server, http.MethodPost, "/api/demo/start", cookie)
	_, second, _ := request(t, server, http.MethodPost, "/api/demo/start", cookie)
	if first.Payout == nil || first.Payout.Status != "provider_unknown" ||
		second.Payout == nil || second.Payout.CreationKey != first.Payout.CreationKey || provider.createCalls != 1 {
		t.Fatal("ambiguous PayPal outcome led to a second payout attempt")
	}
}

func TestMismatchedProviderItemCannotOpenIncident(t *testing.T) {
	provider := &fakeProvider{statuses: []string{"SUCCESS"}, mismatchItem: true}
	server := newServer(provider)
	_, _, cookie := request(t, server, http.MethodPost, "/api/demo/sessions", nil)
	status, _, _ := request(t, server, http.MethodPost, "/api/demo/start", cookie)
	if status != http.StatusBadGateway {
		t.Fatalf("mismatched provider item returned %d", status)
	}
	_, snapshot, _ := request(t, server, http.MethodGet, "/api/demo", cookie)
	if snapshot.Incident != nil || snapshot.Payout == nil || snapshot.Payout.Status != "pending" {
		t.Fatal("mismatched item changed payout or incident")
	}
}
