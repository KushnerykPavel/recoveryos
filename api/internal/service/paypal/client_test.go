package paypal

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCreateAndReadSpecificPayoutItem(t *testing.T) {
	var tokenCalls, createCalls int
	transport := transportFunc(func(r *http.Request) (*http.Response, error) {
		body := ""
		status := http.StatusOK
		switch r.URL.Path {
		case "/v1/oauth2/token":
			id, secret, ok := r.BasicAuth()
			if !ok || id != "client" || secret != "secret" {
				t.Error("OAuth request omitted credentials")
			}
			tokenCalls++
			body = `{"access_token":"test-token","expires_in":3600}`
		case "/v1/payments/payouts":
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer test-token" {
				t.Error("payout creation was not authenticated")
			}
			var request struct {
				SenderBatchHeader struct {
					SenderBatchID string `json:"sender_batch_id"`
				} `json:"sender_batch_header"`
				Items []struct {
					SenderItemID string `json:"sender_item_id"`
					Receiver     string `json:"receiver"`
				} `json:"items"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			if request.SenderBatchHeader.SenderBatchID != "stable-key" || len(request.Items) != 1 ||
				request.Items[0].SenderItemID != "stable-item" || request.Items[0].Receiver != "receiver@example.com" {
				t.Errorf("wrong payout payload: %+v", request)
			}
			createCalls++
			body = `{"batch_header":{"payout_batch_id":"batch-1"}}`
		case "/v1/payments/payouts/batch-1":
			body = `{"batch_header":{"payout_batch_id":"batch-1"},"items":[{"payout_item_id":"item-1","transaction_status":"SUCCESS","payout_item":{"sender_item_id":"stable-item"}}]}`
		default:
			status = http.StatusNotFound
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: r}, nil
	})

	client := New(Config{BaseURL: "https://paypal.test", ClientID: "client", ClientSecret: "secret",
		Receiver: "receiver@example.com", Amount: "0.01", Currency: "USD"}, &http.Client{Transport: transport})
	batch, err := client.CreatePayout(t.Context(), "stable-key", "stable-item")
	if err != nil || batch.ID != "batch-1" {
		t.Fatalf("create payout: batch=%+v err=%v", batch, err)
	}
	item, err := client.GetBatchItem(context.Background(), batch.ID)
	if err != nil || item.ID != "item-1" || item.Status != "SUCCESS" || item.SenderItemID != "stable-item" {
		t.Fatalf("read payout item: item=%+v err=%v", item, err)
	}
	if tokenCalls != 1 || createCalls != 1 {
		t.Fatalf("token calls=%d create calls=%d", tokenCalls, createCalls)
	}
}

func TestVerifyWebhookSendsPayPalDeliveryAndRegisteredID(t *testing.T) {
	event := json.RawMessage(`{"id":"WH-1","event_type":"PAYMENT.PAYOUTS-ITEM.SUCCEEDED"}`)
	verificationStatus := "SUCCESS"
	transport := transportFunc(func(r *http.Request) (*http.Response, error) {
		body := `{"access_token":"test-token","expires_in":3600}`
		if r.URL.Path == "/v1/notifications/verify-webhook-signature" {
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer test-token" {
				t.Error("verification request was not authenticated")
			}
			var request struct {
				TransmissionID   string          `json:"transmission_id"`
				TransmissionTime string          `json:"transmission_time"`
				CertURL          string          `json:"cert_url"`
				AuthAlgo         string          `json:"auth_algo"`
				TransmissionSig  string          `json:"transmission_sig"`
				WebhookID        string          `json:"webhook_id"`
				WebhookEvent     json.RawMessage `json:"webhook_event"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if request.TransmissionID != "tx-1" || request.TransmissionTime != "2026-10-23T10:00:00Z" ||
				request.CertURL != "https://api-m.sandbox.paypal.com/cert" || request.AuthAlgo != "SHA256withRSA" ||
				request.TransmissionSig != "signed" || request.WebhookID != "webhook-1" || string(request.WebhookEvent) != string(event) {
				t.Errorf("wrong verification request: %+v", request)
			}
			body = `{"verification_status":"` + verificationStatus + `"}`
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: r}, nil
	})
	client := New(Config{BaseURL: "https://paypal.test", ClientID: "client", ClientSecret: "secret"}, &http.Client{Transport: transport})
	verified, err := client.VerifyWebhook(t.Context(), "webhook-1", WebhookDelivery{
		TransmissionID: "tx-1", TransmissionTime: "2026-10-23T10:00:00Z",
		CertURL: "https://api-m.sandbox.paypal.com/cert", AuthAlgo: "SHA256withRSA",
		TransmissionSig: "signed", Event: event,
	})
	if err != nil || !verified {
		t.Fatalf("verified=%t err=%v", verified, err)
	}
	verificationStatus = "FAILURE"
	verified, err = client.VerifyWebhook(t.Context(), "webhook-1", WebhookDelivery{
		TransmissionID: "tx-1", TransmissionTime: "2026-10-23T10:00:00Z",
		CertURL: "https://api-m.sandbox.paypal.com/cert", AuthAlgo: "SHA256withRSA",
		TransmissionSig: "signed", Event: event,
	})
	if err != nil || verified {
		t.Fatalf("failed signature verified=%t err=%v", verified, err)
	}
}
