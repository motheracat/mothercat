package systemmonitor

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
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
	mux.HandleFunc("POST /api/v1/system-monitor/endpoints/pause", handler.pauseEndpoint)
	mux.HandleFunc("POST /api/v1/system-monitor/endpoints/resume", handler.resumeEndpoint)
	mux.HandleFunc("GET /api/v1/system-monitor/endpoints/history", handler.endpointHistory)
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
	ctx, cancel, ok := h.authorize(w, r)
	if !ok {
		return
	}
	defer cancel()
	if err := h.engine.RefreshControls(ctx); err != nil {
		h.logger.ErrorContext(ctx, "load endpoint control state", "error", err)
		writeError(w, http.StatusServiceUnavailable, "endpoint_controls_unavailable")
		return
	}
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

type pauseEndpointPayload struct {
	Method          string    `json:"method"`
	Route           string    `json:"route"`
	Reason          string    `json:"reason"`
	ResponseStatus  int       `json:"response_status"`
	ResponseMessage string    `json:"response_message"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type resumeEndpointPayload struct {
	Method string `json:"method"`
	Route  string `json:"route"`
	Reason string `json:"reason"`
}

func (h *Handler) pauseEndpoint(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := h.authorize(w, r)
	if !ok {
		return
	}
	defer cancel()
	if !validActionRequest(w, r) {
		return
	}
	var payload pauseEndpointPayload
	if !decodeActionPayload(w, r, &payload) {
		return
	}
	err := h.engine.PauseEndpoint(ctx, h.db, PauseRequest{
		Method: payload.Method, Route: payload.Route, ActorID: accountIDFromContext(ctx),
		Reason: payload.Reason, ResponseStatus: payload.ResponseStatus,
		ResponseMessage: payload.ResponseMessage, ExpiresAt: payload.ExpiresAt,
	})
	if err != nil {
		h.writeControlError(ctx, w, err, "pause endpoint")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "paused"})
}

func (h *Handler) resumeEndpoint(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := h.authorize(w, r)
	if !ok {
		return
	}
	defer cancel()
	if !validActionRequest(w, r) {
		return
	}
	var payload resumeEndpointPayload
	if !decodeActionPayload(w, r, &payload) {
		return
	}
	err := h.engine.ResumeEndpoint(ctx, h.db, ResumeRequest{
		Method: payload.Method, Route: payload.Route, ActorID: accountIDFromContext(ctx),
		Reason: payload.Reason,
	})
	if err != nil {
		h.writeControlError(ctx, w, err, "resume endpoint")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "resumed"})
}

func (h *Handler) endpointHistory(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := h.authorize(w, r)
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
		if err != nil || parsed < 1 || parsed > maxHistoryLimit {
			writeError(w, http.StatusBadRequest, "invalid_query")
			return
		}
		limit = parsed
	}
	history, err := EndpointControlHistory(ctx, h.db, limit)
	if err != nil {
		h.logger.ErrorContext(ctx, "read endpoint control history", "error", err)
		writeError(w, http.StatusServiceUnavailable, "endpoint_history_unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"history": history})
}

func (h *Handler) writeControlError(ctx context.Context, w http.ResponseWriter, err error, operation string) {
	switch {
	case errors.Is(err, ErrInvalidEndpointControl):
		writeError(w, http.StatusBadRequest, "invalid_endpoint_control")
	case errors.Is(err, ErrEndpointNotControllable):
		writeError(w, http.StatusUnprocessableEntity, "endpoint_not_controllable")
	case errors.Is(err, ErrEndpointNotPaused):
		writeError(w, http.StatusConflict, "endpoint_not_paused")
	default:
		h.logger.ErrorContext(ctx, operation, "error", err)
		writeError(w, http.StatusServiceUnavailable, "endpoint_control_unavailable")
	}
}

func validActionRequest(w http.ResponseWriter, r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" ||
		r.Header.Get("X-System-Monitor-Action") != "confirm" {
		writeError(w, http.StatusUnsupportedMediaType, "json_and_action_header_required")
		return false
	}
	return true
}

func decodeActionPayload(w http.ResponseWriter, r *http.Request, payload any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_body")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request_body")
		return false
	}
	return true
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
	ctx = context.WithValue(ctx, accountIDContextKey{}, accountID)
	return ctx, cancel, true
}

func accountIDFromContext(ctx context.Context) string {
	accountID, _ := ctx.Value(accountIDContextKey{}).(string)
	return accountID
}

type accountIDContextKey struct{}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code}})
}
