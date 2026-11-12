package systemmonitor

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const sessionCookieName = "mothercat_session"

var ErrUnauthenticated = errors.New("unauthenticated")

type Authenticator interface {
	Authenticate(context.Context, string) (string, error)
}

type Handler struct {
	db            *sql.DB
	engine        *Engine
	authenticator Authenticator
	logger        *slog.Logger
}

func NewHandler(db *sql.DB, engine *Engine, authenticator Authenticator, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	handler := &Handler{db: db, engine: engine, authenticator: authenticator, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/system-monitor/overview", handler.overview)
	mux.HandleFunc("GET /api/v1/system-monitor/endpoints", handler.endpoints)
	mux.HandleFunc("GET /api/v1/system-monitor/events", handler.events)
	return mux
}

func (h *Handler) overview(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := h.authorize(w, r)
	if !ok {
		return
	}
	defer cancel()
	writeJSON(w, http.StatusOK, map[string]any{"overview": h.engine.Overview(ctx, h.db)})
}

func (h *Handler) endpoints(w http.ResponseWriter, r *http.Request) {
	_, cancel, ok := h.authorize(w, r)
	if !ok {
		return
	}
	defer cancel()
	writeJSON(w, http.StatusOK, map[string]any{"endpoints": h.engine.Endpoints()})
}

func (h *Handler) events(w http.ResponseWriter, r *http.Request) {
	_, cancel, ok := h.authorize(w, r)
	if !ok {
		return
	}
	defer cancel()
	query := r.URL.Query()
	if len(query) > 1 || len(query["limit"]) > 1 {
		writeError(w, http.StatusBadRequest, "invalid_query")
		return
	}
	limit := 50
	if value := query.Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > eventCapacity {
			writeError(w, http.StatusBadRequest, "invalid_query")
			return
		}
		limit = parsed
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"events": h.engine.Events(limit),
		"retention": "process-local; recent events are lost on restart",
	})
}

func (h *Handler) authorize(w http.ResponseWriter, r *http.Request) (context.Context, context.CancelFunc, bool) {
	w.Header().Set("Cache-Control", "no-store")
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return nil, nil, false
	}
	if h.authenticator == nil {
		writeError(w, http.StatusServiceUnavailable, "identity_unavailable")
		return nil, nil, false
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	accountID, err := h.authenticator.Authenticate(ctx, cookie.Value)
	if err != nil {
		cancel()
		if errors.Is(err, ErrUnauthenticated) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
		} else {
			writeError(w, http.StatusServiceUnavailable, "identity_unavailable")
		}
		return nil, nil, false
	}
	allowed, err := HasAccess(ctx, h.db, accountID)
	if err != nil {
		h.logger.ErrorContext(ctx, "check System Monitor authorization", "error", err)
		cancel()
		writeError(w, http.StatusServiceUnavailable, "authorization_unavailable")
		return nil, nil, false
	}
	if !allowed {
		cancel()
		writeError(w, http.StatusForbidden, "permission_denied")
		return nil, nil, false
	}
	return ctx, cancel, true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code}})
}
