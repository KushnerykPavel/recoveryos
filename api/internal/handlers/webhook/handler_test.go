package webhook

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/KushnerykPavel/recoveryos/api/internal/service/paypal"
)

type stubVerifier struct {
	valid bool
	err   error
	calls int
}

func (v *stubVerifier) VerifyWebhook(_ context.Context, webhookID string, delivery paypal.WebhookDelivery) (bool, error) {
	v.calls++
	if webhookID != "registered-webhook" || delivery.TransmissionID != "transmission-1" ||
		delivery.TransmissionTime == "" || delivery.CertURL == "" || delivery.AuthAlgo == "" ||
		delivery.TransmissionSig == "" || len(delivery.Event) == 0 {
		return false, errors.New("verification request omitted PayPal data")
	}
	return v.valid, v.err
}

type stubStore struct {
	known  bool
	events map[string]string
}

func (s *stubStore) SavePayPalWebhook(_ context.Context, eventID, itemID string) (bool, error) {
	if !s.known {
		return false, nil
	}
	s.events[eventID] = itemID
	return true, nil
}

func webhookRequest(body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/webhooks/paypal", strings.NewReader(body))
	r.Header.Set("PayPal-Transmission-Id", "transmission-1")
	r.Header.Set("PayPal-Transmission-Time", "2026-10-23T10:00:00Z")
	r.Header.Set("PayPal-Cert-Url", "https://api-m.sandbox.paypal.com/cert")
	r.Header.Set("PayPal-Auth-Algo", "SHA256withRSA")
	r.Header.Set("PayPal-Transmission-Sig", "signature")
	return r
}

const successEvent = `{"id":"WH-1","event_type":"PAYMENT.PAYOUTS-ITEM.SUCCEEDED","resource":{"payout_item_id":"ITEM-1"}}`

func TestWebhookAcceptsVerifiedEventWithoutSessionAndDeduplicates(t *testing.T) {
	verifier := &stubVerifier{valid: true}
	store := &stubStore{known: true, events: map[string]string{}}
	mux := http.NewServeMux()
	New("registered-webhook", verifier, store, slog.New(slog.NewTextHandler(io.Discard, nil))).Register(mux)
	for range 2 {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, webhookRequest(successEvent))
		if response.Code != http.StatusNoContent {
			t.Fatalf("verified webhook returned %d: %s", response.Code, response.Body.String())
		}
	}
	if verifier.calls != 2 || len(store.events) != 1 || store.events["WH-1"] != "ITEM-1" {
		t.Fatalf("verification calls=%d events=%v", verifier.calls, store.events)
	}
}

func TestWebhookRejectsUntrustedOrUnmatchedEvents(t *testing.T) {
	tests := []struct {
		name   string
		valid  bool
		err    error
		known  bool
		status int
	}{
		{"invalid signature", false, nil, true, http.StatusUnauthorized},
		{"verification unavailable", false, errors.New("PayPal unavailable"), true, http.StatusServiceUnavailable},
		{"unknown item", true, nil, false, http.StatusNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			verifier := &stubVerifier{valid: test.valid, err: test.err}
			store := &stubStore{known: test.known, events: map[string]string{}}
			mux := http.NewServeMux()
			New("registered-webhook", verifier, store, slog.New(slog.NewTextHandler(io.Discard, nil))).Register(mux)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, webhookRequest(successEvent))
			if response.Code != test.status || len(store.events) != 0 {
				t.Fatalf("status=%d events=%v", response.Code, store.events)
			}
		})
	}
}

func TestWebhookRejectsMissingHeadersAndOversizedBodyBeforeVerification(t *testing.T) {
	verifier := &stubVerifier{valid: true}
	store := &stubStore{known: true, events: map[string]string{}}
	mux := http.NewServeMux()
	New("registered-webhook", verifier, store, slog.New(slog.NewTextHandler(io.Discard, nil))).Register(mux)
	missing := webhookRequest(successEvent)
	missing.Header.Del("PayPal-Transmission-Sig")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, missing)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("missing signature header returned %d", response.Code)
	}
	repeated := webhookRequest(successEvent)
	repeated.Header.Add("PayPal-Transmission-Id", "transmission-2")
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, repeated)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("repeated transmission ID returned %d", response.Code)
	}
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, webhookRequest(strings.Repeat("x", maxBodyBytes+1)))
	if response.Code != http.StatusRequestEntityTooLarge || verifier.calls != 0 || len(store.events) != 0 {
		t.Fatalf("oversized body: status=%d verification calls=%d events=%v", response.Code, verifier.calls, store.events)
	}
}
