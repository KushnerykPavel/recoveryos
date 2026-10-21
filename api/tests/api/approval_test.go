package api_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func approvalRequest(t *testing.T, server http.Handler, cookie *http.Cookie, incidentID string) (int, string) {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/incidents/approve", strings.NewReader(`{"incident_id":"`+incidentID+`"}`))
	request.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	var body struct {
		Error string `json:"error"`
	}
	if response.Body.Len() > 0 {
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
	}
	return response.Code, body.Error
}

func TestApprovalRejectsUnsafeFreshItemResults(t *testing.T) {
	for _, test := range []struct {
		name, status, reason string
		mismatch             bool
		unavailable          bool
	}{
		{"pending", "PENDING", "PENDING", false, false},
		{"unclaimed", "UNCLAIMED", "UNCLAIMED", false, false},
		{"unknown", "BLOCKED", "BLOCKED", false, false},
		{"empty status", "", "unknown", false, false},
		{"mismatched", "SUCCESS", "identity", true, false},
		{"unavailable", "SUCCESS", "unavailable", false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server, provider, _, _ := investigationServer(t, "correct_payout_record", "SUCCESS", "")
			_, _, cookie := request(t, server, http.MethodPost, "/api/sessions", nil)
			_, started, _ := requestWithScenario(t, server, http.MethodPost, "/api/payouts", cookie, "missed-update")
			if status, _ := investigateRequest(t, server, cookie); status != http.StatusOK {
				t.Fatalf("investigation status = %d", status)
			}
			provider.mu.Lock()
			provider.statuses = []string{test.status}
			provider.mismatchItem = test.mismatch
			if test.unavailable {
				provider.readError = errors.New("provider unavailable")
			}
			provider.mu.Unlock()
			status, reason := approvalRequest(t, server, cookie, started.Incident.ID)
			if status != http.StatusConflict || !strings.Contains(reason, test.reason) {
				t.Fatalf("approval status = %d, reason = %q", status, reason)
			}
			_, after, _ := request(t, server, http.MethodGet, "/api/sessions/current", cookie)
			if after.Payout.Status != "pending" || after.Incident.Status != "open" || provider.createCalls != 1 || after.Events[len(after.Events)-1].Kind != "record_correction_rejected" {
				t.Fatalf("approval changed payout or omitted event: %+v", after)
			}
		})
	}
}

func TestApprovalRejectsUnsupportedRecommendationWrongSessionAndChangedRecord(t *testing.T) {
	server, provider, _, _ := investigationServer(t, "no_change", "SUCCESS", "")
	_, _, owner := request(t, server, http.MethodPost, "/api/sessions", nil)
	_, _, other := request(t, server, http.MethodPost, "/api/sessions", nil)
	_, started, _ := requestWithScenario(t, server, http.MethodPost, "/api/payouts", owner, "missed-update")
	if status, _ := investigateRequest(t, server, owner); status != http.StatusOK {
		t.Fatalf("investigation status = %d", status)
	}
	if status, reason := approvalRequest(t, server, owner, started.Incident.ID); status != http.StatusConflict || !strings.Contains(reason, "unsupported") {
		t.Fatalf("unsupported proposal: %d %q", status, reason)
	}
	if status, reason := approvalRequest(t, server, other, started.Incident.ID); status != http.StatusConflict || !strings.Contains(reason, "session") {
		t.Fatalf("other session: %d %q", status, reason)
	}
	_, otherAfter, _ := request(t, server, http.MethodGet, "/api/sessions/current", other)
	if otherAfter.Payout != nil || len(otherAfter.Events) != 1 || otherAfter.Events[0].Kind != "record_correction_rejected" {
		t.Fatalf("wrong-session attempt was not recorded in its own history: %+v", otherAfter)
	}
	_, ownerAfter, _ := request(t, server, http.MethodGet, "/api/sessions/current", owner)
	if ownerAfter.Payout.Status != "pending" || ownerAfter.Incident.Status != "open" || len(ownerAfter.Events) != len(started.Events)+2 {
		t.Fatalf("wrong-session attempt altered the owner: %+v", ownerAfter)
	}
	if provider.createCalls != 1 {
		t.Fatalf("approval created another payout: %d", provider.createCalls)
	}
}

func TestApprovalRejectsMalformedRecommendationAndNoLongerStaleRecord(t *testing.T) {
	for _, test := range []struct {
		name, reason string
		change       func(*memoryStore, string)
	}{
		{"malformed recommendation", "malformed", func(store *memoryStore, sessionID string) {
			result := store.investigations[sessionID]
			result.ItemID = "wrong-item"
			store.investigations[sessionID] = result
		}},
		{"record no longer stale", "no longer stale", func(store *memoryStore, sessionID string) {
			payout := store.payouts[sessionID]
			payout.Status = "completed"
			store.payouts[sessionID] = payout
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			server, provider, _, store := investigationServer(t, "correct_payout_record", "SUCCESS", "")
			_, _, cookie := request(t, server, http.MethodPost, "/api/sessions", nil)
			_, started, _ := requestWithScenario(t, server, http.MethodPost, "/api/payouts", cookie, "missed-update")
			if status, _ := investigateRequest(t, server, cookie); status != http.StatusOK {
				t.Fatalf("investigation status = %d", status)
			}
			store.mu.Lock()
			test.change(store, started.Session.ID)
			store.mu.Unlock()
			status, reason := approvalRequest(t, server, cookie, started.Incident.ID)
			if status != http.StatusConflict || !strings.Contains(reason, test.reason) {
				t.Fatalf("approval status = %d, reason = %q", status, reason)
			}
			_, after, _ := request(t, server, http.MethodGet, "/api/sessions/current", cookie)
			if after.Incident.Status != "open" || after.Events[len(after.Events)-1].Kind != "record_correction_rejected" || provider.createCalls != 1 {
				t.Fatalf("approval changed incident or omitted event: %+v", after)
			}
		})
	}
}

func TestApprovalRejectsFreshFailedItemAndRecordsAttempt(t *testing.T) {
	server, provider, _, _ := investigationServer(t, "correct_payout_record", "SUCCESS", "")
	_, _, cookie := request(t, server, http.MethodPost, "/api/sessions", nil)
	_, started, _ := requestWithScenario(t, server, http.MethodPost, "/api/payouts", cookie, "missed-update")
	if status, _ := investigateRequest(t, server, cookie); status != http.StatusOK {
		t.Fatalf("investigation status = %d", status)
	}
	provider.mu.Lock()
	provider.statuses = []string{"FAILED"}
	provider.mu.Unlock()
	status, reason := approvalRequest(t, server, cookie, started.Incident.ID)
	if status != http.StatusConflict || !strings.Contains(reason, "FAILED") {
		t.Fatalf("approval status = %d, reason = %q", status, reason)
	}
	_, after, _ := request(t, server, http.MethodGet, "/api/sessions/current", cookie)
	if after.Payout.Status != "pending" || after.Incident.Status != "open" || provider.createCalls != 1 {
		t.Fatalf("rejected approval changed payout or incident: %+v", after)
	}
	last := after.Events[len(after.Events)-1]
	if last.Kind != "record_correction_rejected" || !strings.Contains(last.Details, "FAILED") {
		t.Fatalf("rejection missing from event history: %+v", last)
	}
}

func TestVerifiedProposalCorrectsRecordOnce(t *testing.T) {
	server, provider, _, _ := investigationServer(t, "correct_payout_record", "SUCCESS", "")
	_, _, cookie := request(t, server, http.MethodPost, "/api/sessions", nil)
	_, started, _ := requestWithScenario(t, server, http.MethodPost, "/api/payouts", cookie, "missed-update")
	if status, _ := investigateRequest(t, server, cookie); status != http.StatusOK {
		t.Fatalf("investigation status = %d", status)
	}
	status, reason := approvalRequest(t, server, cookie, started.Incident.ID)
	if status != http.StatusNoContent || reason != "" {
		t.Fatalf("verified proposal: %d %q", status, reason)
	}
	_, after, _ := request(t, server, http.MethodGet, "/api/sessions/current", cookie)
	if after.Payout.Status != "corrected" || after.Incident.Status != "resolved" || provider.createCalls != 1 {
		t.Fatalf("verified proposal did not resolve discrepancy: %+v", after)
	}
	for _, kind := range []string{"provider_verification", "operator_approval", "record_corrected"} {
		found := false
		for _, event := range after.Events {
			if event.Kind == kind {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing %s audit event: %+v", kind, after.Events)
		}
	}
	if status, _ := approvalRequest(t, server, cookie, started.Incident.ID); status != http.StatusConflict {
		t.Fatalf("repeat approval returned %d", status)
	}
	_, repeated, _ := request(t, server, http.MethodGet, "/api/sessions/current", cookie)
	corrections := 0
	for _, event := range repeated.Events {
		if event.Kind == "record_corrected" {
			corrections++
		}
	}
	if corrections != 1 || len(repeated.Events) != len(after.Events) || repeated.Payout.Status != "corrected" || provider.createCalls != 1 {
		t.Fatalf("repeat approval had another effect: %+v", repeated)
	}
}

func TestConcurrentApprovalsCorrectRecordOnce(t *testing.T) {
	server, provider, _, _ := investigationServer(t, "correct_payout_record", "SUCCESS", "")
	_, _, cookie := request(t, server, http.MethodPost, "/api/sessions", nil)
	_, started, _ := requestWithScenario(t, server, http.MethodPost, "/api/payouts", cookie, "missed-update")
	if status, _ := investigateRequest(t, server, cookie); status != http.StatusOK {
		t.Fatalf("investigation status = %d", status)
	}
	const attempts = 12
	results := make(chan int, attempts)
	var ready sync.WaitGroup
	ready.Add(attempts)
	start := make(chan struct{})
	for range attempts {
		go func() {
			ready.Done()
			<-start
			status, _ := approvalRequest(t, server, cookie, started.Incident.ID)
			results <- status
		}()
	}
	ready.Wait()
	close(start)
	successes := 0
	for range attempts {
		switch status := <-results; status {
		case http.StatusNoContent:
			successes++
		case http.StatusConflict:
		default:
			t.Fatalf("approval status = %d", status)
		}
	}
	_, after, _ := request(t, server, http.MethodGet, "/api/sessions/current", cookie)
	corrections := 0
	for _, event := range after.Events {
		if event.Kind == "record_corrected" {
			corrections++
		}
	}
	if successes != 1 || corrections != 1 || after.Payout.Status != "corrected" || after.Incident.Status != "resolved" || provider.createCalls != 1 {
		t.Fatalf("successes=%d corrections=%d snapshot=%+v createCalls=%d", successes, corrections, after, provider.createCalls)
	}
}
