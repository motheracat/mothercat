package http

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/mothercat-enterprise/mothercat/backend/services/audit/internal/domain"
)

const maxEventBody = 16 << 10

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var requestIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{32}$`)

type eventStore interface {
	Ping(context.Context) error
	Record(context.Context, domain.Event) error
}

type requestIDKey struct{}

func NewHandler(store eventStore, serviceToken string, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "service_not_ready"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := store.Ping(ctx); err != nil {
			logger.WarnContext(ctx, "Audit readiness check failed", "request_id", requestID(ctx), "error", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "service_not_ready"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("POST /internal/v1/audit-events", func(w http.ResponseWriter, r *http.Request) {
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if serviceToken == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(serviceToken)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		if store == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "service_unavailable"})
			return
		}
		mediaType, _, mediaTypeErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if mediaTypeErr != nil || mediaType != "application/json" {
			writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "content_type_must_be_application_json"})
			return
		}
		var event domain.Event
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxEventBody))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&event); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_event"})
			return
		}
		var extra any
		if !errors.Is(decoder.Decode(&extra), io.EOF) ||
			len(event.EventKey) == 0 || len(event.EventKey) > 200 ||
			!uuidPattern.MatchString(event.ActorID) || !uuidPattern.MatchString(event.ResourceID) ||
			!validAction(event.Action) || event.Details == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_event"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := store.Record(ctx, event); err != nil {
			logger.ErrorContext(ctx, "persist audit event", "request_id", requestID(ctx), "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"status": "recorded"})
	})
	return requestLoggingMiddleware(logger, mux)
}

func validAction(action string) bool {
	if len(action) == 0 || len(action) > 120 {
		return false
	}
	for _, char := range action {
		if !(char >= 'a' && char <= 'z') && !(char >= '0' && char <= '9') && char != '_' && char != '.' {
			return false
		}
	}
	return true
}

func requestLoggingMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if !requestIDPattern.MatchString(id) {
			var requestIDBytes [16]byte
			_, err := rand.Read(requestIDBytes[:])
			id = "unavailable"
			if err != nil {
				logger.ErrorContext(r.Context(), "generate Audit request ID", "error", err)
			} else {
				id = hex.EncodeToString(requestIDBytes[:])
			}
		}
		ctx := context.WithValue(r.Context(), requestIDKey{}, id)
		r = r.WithContext(ctx)
		w.Header().Set("X-Request-ID", id)
		writer := &statusWriter{ResponseWriter: w}
		started := time.Now()
		next.ServeHTTP(writer, r)
		logger.InfoContext(ctx, "Audit HTTP request",
			"request_id", id, "method", r.Method, "route", auditRoute(r.URL.Path),
			"status", writer.status, "duration_ms", time.Since(started).Milliseconds())
	})
}

func auditRoute(path string) string {
	if path == "/health/live" {
		return "health_live"
	}
	if path == "/health/ready" {
		return "health_ready"
	}
	if path == "/internal/v1/audit-events" {
		return "record_audit_event"
	}
	return "unmatched"
}

func requestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.status, w.wroteHeader = status, true
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
