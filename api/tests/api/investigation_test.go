package api_test

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	recoveryhandler "github.com/KushnerykPavel/recoveryos/api/internal/handlers/recovery"
	"github.com/KushnerykPavel/recoveryos/api/internal/service/openai"
	"github.com/KushnerykPavel/recoveryos/api/internal/usecase/investigation"
	recoveryuc "github.com/KushnerykPavel/recoveryos/api/internal/usecase/recovery"
)

func investigationServer(t *testing.T, proposedAction, itemStatus string) (http.Handler, *fakeProvider, *int) {
	t.Helper()
	fixture, err := os.ReadFile("../../../client/test/fixtures/investigation.json")
	if err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{statuses: []string{"SUCCESS", itemStatus}}
	store := newMemoryStore()
	service := recoveryuc.NewService(store, provider, 0, 0)
	callCount := 0
	apiHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.URL.Path != "/v1/responses" || r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer scripted-key" {
			t.Errorf("unexpected OpenAI request: %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Model             string `json:"model"`
			ParallelToolCalls bool   `json:"parallel_tool_calls"`
			Tools             []struct {
				Name string `json:"name"`
			} `json:"tools"`
			Input []json.RawMessage `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "scripted-model" || len(body.Tools) != 5 || !body.ParallelToolCalls {
			t.Errorf("model or tools missing: %+v", body)
		}
		for _, tool := range body.Tools {
			if !strings.HasPrefix(tool.Name, "read_") {
				t.Errorf("non-read tool exposed: %s", tool.Name)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			_, _ = io.WriteString(w, `{"status":"completed","output":[`+
				`{"type":"function_call","name":"read_incident","call_id":"a","arguments":"{}"},`+
				`{"type":"function_call","name":"read_payout_record","call_id":"b","arguments":"{}"},`+
				`{"type":"function_call","name":"read_paypal_payout","call_id":"c","arguments":"{}"},`+
				`{"type":"function_call","name":"read_payout_events","call_id":"d","arguments":"{}"},`+
				`{"type":"function_call","name":"read_recovery_policy","call_id":"e","arguments":"{}"}]}`)
			return
		}
		if len(body.Input) != 11 {
			t.Errorf("tool evidence missing from follow-up: %d items", len(body.Input))
		}
		var incidentID, payoutID, itemID string
		for _, raw := range body.Input {
			var entry struct {
				Output string `json:"output"`
			}
			if json.Unmarshal(raw, &entry) != nil || entry.Output == "" {
				continue
			}
			var evidence struct {
				Reference string          `json:"reference"`
				Data      json.RawMessage `json:"data"`
			}
			if json.Unmarshal([]byte(entry.Output), &evidence) != nil {
				continue
			}
			var data map[string]any
			_ = json.Unmarshal(evidence.Data, &data)
			if data["payout_id"] != nil {
				incidentID = evidence.Reference
			}
			if data["creation_key"] != nil {
				payoutID = evidence.Reference
			}
			if data["SenderItemID"] != nil {
				itemID = evidence.Reference
			}
		}
		var result map[string]any
		if err := json.Unmarshal(fixture, &result); err != nil {
			t.Fatal(err)
		}
		if result["incident_id"] != incidentID || result["item_id"] != itemID || result["evidence"].([]any)[1].(map[string]any)["reference"] != payoutID {
			t.Error("scripted diagnosis does not match read-only evidence")
		}
		result["item_status"] = itemStatus
		result["proposed_action"] = proposedAction
		delete(result, "origin")
		encoded, _ := json.Marshal(result)
		message, _ := json.Marshal(map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": string(encoded)}}}}})
		_, _ = w.Write(message)
	})
	client := &http.Client{Transport: scriptedTransport(func(request *http.Request) (*http.Response, error) {
		recorder := httptest.NewRecorder()
		apiHandler.ServeHTTP(recorder, request)
		return recorder.Result(), nil
	})}
	model := openai.New("scripted-key", "scripted-model", client)
	inv := investigation.New(service, provider, model, store)
	handler := recoveryhandler.New(service, false, slog.New(slog.NewTextHandler(io.Discard, nil))).WithInvestigator(inv)
	mux := http.NewServeMux()
	handler.Register(mux)
	return mux, provider, &callCount
}

func investigateRequest(t *testing.T, server http.Handler, cookie *http.Cookie) (int, investigation.Result) {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/incidents/investigate", nil)
	if cookie != nil {
		request.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	server.ServeHTTP(w, request)
	var result investigation.Result
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
	}
	return w.Code, result
}

func TestIncidentInvestigationShowsCitedDiagnosisWithoutChangingPayout(t *testing.T) {
	server, provider, calls := investigationServer(t, "correct_payout_record", "SUCCESS")
	_, _, cookie := request(t, server, http.MethodPost, "/api/sessions", nil)
	_, started, _ := requestWithScenario(t, server, http.MethodPost, "/api/payouts", cookie, "missed-update")
	status, result := investigateRequest(t, server, cookie)
	if status != http.StatusOK || result.IncidentID != started.Incident.ID || result.ItemID != started.Payout.ItemID || result.ItemStatus != "SUCCESS" || result.Origin != "ai" || result.ProposedAction != "correct_payout_record" || len(result.Evidence) != 4 {
		t.Fatalf("visible diagnosis missing: status=%d result=%+v", status, result)
	}
	_, after, _ := request(t, server, http.MethodGet, "/api/sessions/current", cookie)
	if after.Payout.Status != "pending" || after.Incident.Status != "open" || after.Investigation == nil || after.Investigation.Diagnosis != result.Diagnosis || provider.createCalls != 1 || *calls != 2 {
		t.Fatalf("investigation mutated the payout or did not use scripted responses: %+v, calls=%d", after, *calls)
	}
}

func TestInvestigationRejectsUnsupportedCorrectionAndOtherSession(t *testing.T) {
	server, _, calls := investigationServer(t, "correct_payout_record", "FAILED")
	_, _, owner := request(t, server, http.MethodPost, "/api/sessions", nil)
	_, _, other := request(t, server, http.MethodPost, "/api/sessions", nil)
	_, _, _ = requestWithScenario(t, server, http.MethodPost, "/api/payouts", owner, "missed-update")
	if status, _ := investigateRequest(t, server, other); status != http.StatusConflict {
		t.Fatal("another session investigated the incident")
	}
	if status, _ := investigateRequest(t, server, owner); status != http.StatusBadGateway {
		t.Fatalf("unsafe correction returned %d", status)
	}
	if *calls != 2 {
		t.Fatal(fmt.Sprintf("unexpected model calls: %d", *calls))
	}
}
