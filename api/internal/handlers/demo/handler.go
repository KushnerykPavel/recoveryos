package demo

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/KushnerykPavel/recoveryos/api/internal/domain/demo"
	demouc "github.com/KushnerykPavel/recoveryos/api/internal/usecase/demo"
)

const sessionCookie = "recoveryos_demo"

type service interface {
	CreateSession(ctx context.Context) (demo.Snapshot, string, error)
	Snapshot(ctx context.Context, token string) (demo.Snapshot, error)
	Start(ctx context.Context, token string) (demo.Snapshot, error)
	Reconcile(ctx context.Context, token string) (demo.Snapshot, error)
}

type Handler struct {
	service      service
	secureCookie bool
	logger       *slog.Logger
}

func New(service service, secureCookie bool, logger *slog.Logger) *Handler {
	return &Handler{service: service, secureCookie: secureCookie, logger: logger}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/demo/sessions", h.createSession)
	mux.HandleFunc("GET /api/demo", h.snapshot)
	mux.HandleFunc("POST /api/demo/start", h.start)
	mux.HandleFunc("POST /api/demo/reconcile", h.reconcile)
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

func (h *Handler) start(w http.ResponseWriter, r *http.Request) {
	snapshot, err := h.service.Start(r.Context(), cookieToken(r))
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
	if errors.Is(err, demouc.ErrSessionNotFound) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "demo session not found"})
		return
	}
	if errors.Is(err, demouc.ErrDailyLimit) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "daily payout creation limit reached"})
		return
	}
	h.logger.Error("demo request failed", "error", err)
	writeJSON(w, http.StatusBadGateway, map[string]string{"error": "demo operation unavailable"})
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
