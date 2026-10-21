package recovery

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/KushnerykPavel/recoveryos/api/internal/domain/recovery"
	"github.com/KushnerykPavel/recoveryos/api/internal/usecase/investigation"
	recoveryuc "github.com/KushnerykPavel/recoveryos/api/internal/usecase/recovery"
)

const sessionCookie = "recoveryos_session"

type service interface {
	CreateSession(ctx context.Context) (recovery.Snapshot, string, error)
	Snapshot(ctx context.Context, token string) (recovery.Snapshot, error)
	CreatePayout(ctx context.Context, token, scenario string) (recovery.Snapshot, error)
	Reconcile(ctx context.Context, token string) (recovery.Snapshot, error)
}

type Handler struct {
	service      service
	investigator interface {
		Investigate(context.Context, string) (investigation.Result, error)
	}
	secureCookie bool
	logger       *slog.Logger
}

func (h *Handler) WithInvestigator(investigator interface {
	Investigate(context.Context, string) (investigation.Result, error)
}) *Handler {
	h.investigator = investigator
	return h
}

func New(service service, secureCookie bool, logger *slog.Logger) *Handler {
	return &Handler{service: service, secureCookie: secureCookie, logger: logger}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/sessions", h.createSession)
	mux.HandleFunc("GET /api/sessions/current", h.snapshot)
	mux.HandleFunc("POST /api/payouts", h.createPayout)
	mux.HandleFunc("POST /api/payouts/reconcile", h.reconcile)
	if h.investigator != nil {
		mux.HandleFunc("POST /api/incidents/investigate", h.investigate)
	}
}

func (h *Handler) investigate(w http.ResponseWriter, r *http.Request) {
	result, err := h.investigator.Investigate(r.Context(), cookieToken(r))
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) createSession(w http.ResponseWriter, r *http.Request) {
	snapshot, token, err := h.service.CreateSession(r.Context())
	if err != nil {
		h.fail(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", Expires: snapshot.Session.ExpiresAt,
		HttpOnly: true, Secure: h.secureCookie, SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusCreated, snapshot)
}

func (h *Handler) snapshot(w http.ResponseWriter, r *http.Request) {
	snapshot, err := h.service.Snapshot(r.Context(), cookieToken(r))
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (h *Handler) createPayout(w http.ResponseWriter, r *http.Request) {
	values := r.Header.Values("X-RecoveryOS-Scenario")
	if len(values) > 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "specify one payout scenario"})
		return
	}
	scenario := ""
	if len(values) == 1 {
		scenario = values[0]
	}
	snapshot, err := h.service.CreatePayout(r.Context(), cookieToken(r), scenario)
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (h *Handler) reconcile(w http.ResponseWriter, r *http.Request) {
	snapshot, err := h.service.Reconcile(r.Context(), cookieToken(r))
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	if errors.Is(err, investigation.ErrNoIncident) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "no open incident in this session"})
		return
	}
	if errors.Is(err, recoveryuc.ErrSessionNotFound) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "session not found"})
		return
	}
	if errors.Is(err, recoveryuc.ErrDailyLimit) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "daily payout creation limit reached"})
		return
	}
	if errors.Is(err, recoveryuc.ErrInvalidScenario) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payout scenario"})
		return
	}
	if errors.Is(err, recoveryuc.ErrScenarioConflict) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "payout scenario cannot be changed"})
		return
	}
	h.logger.Error("recovery request failed", "error", err)
	writeJSON(w, http.StatusBadGateway, map[string]string{"error": "recovery operation unavailable"})
}

func cookieToken(r *http.Request) string {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
