package http

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/observability"
)

type pinger interface {
	Ping(context.Context) error
}

// NewHandler registers operational endpoints. Business routes are intentionally
// absent until their API contracts and persistence behavior are implemented.
func NewHandler(database pinger, logger *slog.Logger, healthTimeout time.Duration) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	if healthTimeout <= 0 {
		healthTimeout = 2 * time.Second
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		if database == nil {
			writeNotReady(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), healthTimeout)
		defer cancel()
		if err := database.Ping(ctx); err != nil {
			logger.WarnContext(ctx, "readiness check failed",
				"request_id", requestIDFromContext(r.Context()),
				"dependency", "postgres",
			)
			writeNotReady(w, r)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	return requestIDMiddleware(logger, requestLoggingMiddleware(logger, recoveryMiddleware(logger, mux)))
}

func requestIDMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID, err := newRequestID()
		if err != nil {
			logger.Error("generate request correlation ID", "error", err)
			requestID = "unavailable"
		}
		ctx := observability.ContextWithRequestID(r.Context(), requestID)
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requestLoggingMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		writer := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(writer, r)
		logger.InfoContext(r.Context(), "HTTP request",
			"request_id", requestIDFromContext(r.Context()),
			"method", r.Method,
			"path", r.URL.Path,
			"status", writer.status,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	})
}

func recoveryMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.ErrorContext(r.Context(), "unhandled HTTP panic",
					"request_id", requestIDFromContext(r.Context()),
					"panic_type", fmt.Sprintf("%T", recovered),
				)
				if committer, ok := w.(interface{ committed() bool }); !ok || !committer.committed() {
					writeError(w, http.StatusInternalServerError, "internal_error", "an internal error occurred", requestIDFromContext(r.Context()))
				}
			}
		}()
		next.ServeHTTP(w, r)
	})
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
	w.status = status
	w.wroteHeader = true
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

func (w *statusWriter) committed() bool {
	return w.wroteHeader
}

func writeNotReady(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusServiceUnavailable, "service_not_ready", "service dependencies unavailable", requestIDFromContext(r.Context()))
}

func writeError(w http.ResponseWriter, status int, code, message, requestID string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{
			"code":       code,
			"message":    message,
			"request_id": requestID,
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func requestIDFromContext(ctx context.Context) string {
	return observability.RequestIDFromContext(ctx)
}

func newRequestID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(id[:]), nil
}
