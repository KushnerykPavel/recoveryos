package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/KushnerykPavel/recoveryos/api/internal/service/paypal"
)

const maxBodyBytes = 64 << 10

type verifier interface {
	VerifyWebhook(context.Context, string, paypal.WebhookDelivery) (bool, error)
}

type store interface {
	SavePayPalWebhook(context.Context, string, string) (bool, error)
}

type Handler struct {
	webhookID string
	verifier  verifier
	store     store
	logger    *slog.Logger
}

func New(webhookID string, verifier verifier, store store, logger *slog.Logger) *Handler {
	return &Handler{webhookID: webhookID, verifier: verifier, store: store, logger: logger}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/webhooks/paypal", h.receive)
}

func (h *Handler) receive(w http.ResponseWriter, r *http.Request) {
	transmissionID, idOK := singleHeader(r.Header, "PayPal-Transmission-Id")
	transmissionTime, timeOK := singleHeader(r.Header, "PayPal-Transmission-Time")
	certURL, certOK := singleHeader(r.Header, "PayPal-Cert-Url")
	authAlgo, algoOK := singleHeader(r.Header, "PayPal-Auth-Algo")
	transmissionSig, sigOK := singleHeader(r.Header, "PayPal-Transmission-Sig")
	if !idOK || !timeOK || !certOK || !algoOK || !sigOK {
		http.Error(w, "missing or repeated PayPal transmission headers", http.StatusBadRequest)
		return
	}
	delivery := paypal.WebhookDelivery{
		TransmissionID:   transmissionID,
		TransmissionTime: transmissionTime,
		CertURL:          certURL,
		AuthAlgo:         authAlgo,
		TransmissionSig:  transmissionSig,
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		if _, tooLarge := errors.AsType[*http.MaxBytesError](err); tooLarge {
			http.Error(w, "webhook body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "cannot read webhook body", http.StatusBadRequest)
		return
	}
	if !json.Valid(body) {
		http.Error(w, "invalid webhook JSON", http.StatusBadRequest)
		return
	}
	delivery.Event = body
	verified, err := h.verifier.VerifyWebhook(r.Context(), h.webhookID, delivery)
	if err != nil {
		h.logger.Error("PayPal webhook verification unavailable", "error", err)
		http.Error(w, "webhook verification unavailable", http.StatusServiceUnavailable)
		return
	}
	if !verified {
		http.Error(w, "invalid PayPal webhook signature", http.StatusUnauthorized)
		return
	}
	var event struct {
		ID       string `json:"id"`
		Type     string `json:"event_type"`
		Resource struct {
			ItemID string `json:"payout_item_id"`
		} `json:"resource"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "invalid PayPal webhook event", http.StatusBadRequest)
		return
	}
	if event.Type != "PAYMENT.PAYOUTS-ITEM.SUCCEEDED" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !validID(event.ID) || !validID(event.Resource.ItemID) {
		http.Error(w, "missing PayPal event or payout item ID", http.StatusBadRequest)
		return
	}
	matched, err := h.store.SavePayPalWebhook(r.Context(), event.ID, event.Resource.ItemID)
	if err != nil {
		h.logger.Error("PayPal webhook event could not be saved", "event_id", event.ID, "error", err)
		http.Error(w, "webhook storage unavailable", http.StatusServiceUnavailable)
		return
	}
	if !matched {
		http.Error(w, "payout item not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func singleHeader(headers http.Header, name string) (string, bool) {
	values := headers.Values(name)
	if len(values) != 1 || values[0] == "" {
		return "", false
	}
	return values[0], true
}

func validID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, ch := range value {
		if ch < '!' || ch > '~' {
			return false
		}
	}
	return true
}
