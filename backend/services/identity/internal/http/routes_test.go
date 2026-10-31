package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type pingerFunc func(context.Context) error

func (fn pingerFunc) Ping(ctx context.Context) error {
	return fn(ctx)
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func TestLiveHealthReturnsCorrelationID(t *testing.T) {
	handler := NewHandler(nil, testLogger(), time.Second)
	request := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	requestID := response.Header().Get("X-Request-ID")
	if len(requestID) != 32 {
		t.Fatalf("X-Request-ID length = %d, want 32", len(requestID))
	}
	if !strings.HasPrefix(response.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", response.Header().Get("Content-Type"))
	}
}

func TestReadinessReturnsUnavailableWithoutDatabase(t *testing.T) {
	handler := NewHandler(nil, testLogger(), time.Second)
	request := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(response.Body.String(), `"service_not_ready"`) {
		t.Fatalf("response body %q does not contain the expected error code", response.Body.String())
	}
}

func TestReadinessReturnsUnavailableWithoutLeakingDatabaseError(t *testing.T) {
	const privateDatabaseDetail = "internal-hostname-and-driver-detail"
	var logOutput bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logOutput, nil))
	handler := NewHandler(pingerFunc(func(context.Context) error {
		return errors.New(privateDatabaseDetail)
	}), logger, time.Second)
	request := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	if strings.Contains(response.Body.String(), privateDatabaseDetail) {
		t.Fatalf("response leaked internal database error: %q", response.Body.String())
	}
	if strings.Contains(logOutput.String(), privateDatabaseDetail) {
		t.Fatalf("logs leaked internal database error: %q", logOutput.String())
	}
	var body struct {
		Error struct {
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode readiness response: %v", err)
	}
	if body.Error.RequestID != response.Header().Get("X-Request-ID") {
		t.Fatalf("error request_id = %q, header = %q", body.Error.RequestID, response.Header().Get("X-Request-ID"))
	}
}

func TestReadinessUsesConfiguredTimeout(t *testing.T) {
	handler := NewHandler(pingerFunc(func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}), testLogger(), 10*time.Millisecond)
	request := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	response := httptest.NewRecorder()

	started := time.Now()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("readiness check took %s, expected configured timeout to bound it", elapsed)
	}
}

func TestRecoveryReturnsGenericErrorBeforeResponseIsCommitted(t *testing.T) {
	handler := requestLoggingMiddleware(testLogger(), recoveryMiddleware(testLogger(), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("sensitive panic detail")
	})))
	request := httptest.NewRequest(http.MethodGet, "/test", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if strings.Contains(response.Body.String(), "sensitive panic detail") {
		t.Fatalf("response leaked panic value: %q", response.Body.String())
	}
}

func TestRecoveryDoesNotAppendBodyAfterResponseIsCommitted(t *testing.T) {
	handler := requestLoggingMiddleware(testLogger(), recoveryMiddleware(testLogger(), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("partial"))
		panic("sensitive panic detail")
	})))
	request := httptest.NewRequest(http.MethodGet, "/test", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusAccepted)
	}
	if got := response.Body.String(); got != "partial" {
		t.Fatalf("body = %q, want only the already-written response", got)
	}
}
