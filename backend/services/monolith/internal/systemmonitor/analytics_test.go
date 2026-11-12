package systemmonitor

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestFlushAnalyticsPersistsEndpointBucketAndStatusCounts(t *testing.T) {
	db, mock, err := newMockDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	bucket := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	engine := NewEngine()
	engine.record(Endpoint{Method: http.MethodGet, Route: "/api/v1/tasks/{taskID}"},
		http.StatusOK, 12*time.Millisecond, bucket.Add(time.Second))
	engine.record(Endpoint{Method: http.MethodGet, Route: "/api/v1/tasks/{taskID}"},
		http.StatusNotFound, 24*time.Millisecond, bucket.Add(2*time.Second))

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)INSERT INTO system_monitor\.endpoint_analytics AS stored`).
		WithArgs(http.MethodGet, "/api/v1/tasks/{taskID}", bucket, uint64(2), uint64(1), uint64(0),
			uint64(36), uint64(24), uint64(0), uint64(2), uint64(0), uint64(0), uint64(0), uint64(0),
			sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := engine.FlushAnalytics(context.Background(), db); err != nil {
		t.Fatalf("FlushAnalytics() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFlushAnalyticsRestoresPendingDeltasOnDatabaseFailure(t *testing.T) {
	db, mock, err := newMockDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	bucket := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	engine := NewEngine()
	engine.record(Endpoint{Method: http.MethodPost, Route: "/api/v1/tasks"},
		http.StatusCreated, 5*time.Millisecond, bucket)
	mock.ExpectBegin().WillReturnError(errors.New("database unavailable"))
	if err := engine.FlushAnalytics(context.Background(), db); err == nil {
		t.Fatal("FlushAnalytics() error = nil, want database failure")
	}

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)INSERT INTO system_monitor\.endpoint_analytics AS stored`).
		WithArgs(http.MethodPost, "/api/v1/tasks", bucket, uint64(1), uint64(0), uint64(0),
			uint64(5), uint64(5), uint64(1), uint64(0), uint64(0), uint64(0), uint64(0), uint64(0),
			sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := engine.FlushAnalytics(context.Background(), db); err != nil {
		t.Fatalf("FlushAnalytics() retry error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEndpointAnalyticsReturnsStatusLatencyAndEmptyChartBuckets(t *testing.T) {
	db, mock, err := newMockDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	from := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	to := from.Add(2 * time.Hour)
	mock.ExpectQuery(`(?s)WITH filtered AS .*ORDER BY metrics\.period`).
		WithArgs(http.MethodGet, "/api/v1/tasks/{taskID}", from, to, "hour").
		WillReturnRows(sqlmock.NewRows([]string{
			"period", "requests", "client_errors", "server_errors", "latency_total_ms", "latency_max_ms",
			"latency_lt_10ms", "latency_lt_50ms", "latency_lt_100ms", "latency_lt_500ms",
			"latency_lt_1000ms", "latency_gte_1000ms", "status_counts",
		}).AddRow(from, 2, 1, 0, 36, 24, 0, 2, 0, 0, 0, 0, `{"200":1,"404":1}`))

	report, err := EndpointAnalytics(context.Background(), db, newEndpointCatalog(), AnalyticsQuery{
		Method: http.MethodGet, Route: "/api/v1/tasks/{taskID}",
		From: from, To: to, Interval: "hour",
	})
	if err != nil {
		t.Fatalf("EndpointAnalytics() error = %v", err)
	}
	if len(report.Points) != 2 {
		t.Fatalf("point count = %d, want 2", len(report.Points))
	}
	first := report.Points[0]
	if first.Requests != 2 || first.Successes != 1 || first.ClientErrors != 1 ||
		first.StatusDistribution["404"] != 1 || first.AverageLatencyMS != 18 ||
		first.P95LatencyMS != 50 || first.LatencyDistribution["<50ms"] != 2 {
		t.Fatalf("unexpected analytics point: %#v", first)
	}
	if second := report.Points[1]; second.Requests != 0 || second.Bucket != from.Add(time.Hour) {
		t.Fatalf("unexpected zero-filled chart point: %#v", second)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEndpointAnalyticsRejectsUnknownEndpointAndOversizedRange(t *testing.T) {
	db, mock, err := newMockDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	from := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	_, err = EndpointAnalytics(context.Background(), db, newEndpointCatalog(), AnalyticsQuery{
		Method: http.MethodDelete, Route: "/api/v1/tasks/{taskID}",
		From: from, To: from.Add(time.Hour), Interval: "hour",
	})
	if !errors.Is(err, ErrEndpointNotFound) {
		t.Fatalf("unknown endpoint error = %v, want ErrEndpointNotFound", err)
	}
	_, err = EndpointAnalytics(context.Background(), db, newEndpointCatalog(), AnalyticsQuery{
		Method: http.MethodGet, Route: "/api/v1/tasks/{taskID}",
		From: from, To: from.Add(91 * 24 * time.Hour), Interval: "day",
	})
	if !errors.Is(err, ErrInvalidAnalyticsQuery) {
		t.Fatalf("oversized range error = %v, want ErrInvalidAnalyticsQuery", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAnalyticsPendingBufferIsBoundedAndReportsDroppedRequests(t *testing.T) {
	engine := NewEngine()
	endpoint := Endpoint{Method: http.MethodGet, Route: "/api/v1/tasks"}
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for index := 0; index <= maxPendingBuckets; index++ {
		engine.record(endpoint, http.StatusOK, time.Millisecond,
			start.Add(time.Duration(index)*analyticsBucketWidth))
	}
	overview := engine.Overview(context.Background(), nil)
	if overview.AnalyticsPendingBuckets != maxPendingBuckets ||
		overview.AnalyticsDroppedRequests != 1 {
		t.Fatalf("unexpected analytics buffer status: %#v", overview)
	}
	events := engine.Events(eventCapacity)
	foundBufferAlert := false
	for _, event := range events {
		if event.Type == "analytics_buffer_full" {
			foundBufferAlert = true
			break
		}
	}
	if !foundBufferAlert {
		t.Fatal("buffer overflow did not emit an analytics_buffer_full event")
	}
}

func TestAnalyticsHandlerRequiresMonitorGrantAndReturnsEndpointSeries(t *testing.T) {
	db, mock, err := newMockDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	from := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	mock.ExpectQuery(`(?s)SELECT EXISTS \(.*system_monitor\.access_grants`).
		WithArgs(testMonitorActorID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`(?s)WITH filtered AS .*ORDER BY metrics\.period`).
		WithArgs(http.MethodGet, "/api/v1/tasks/{taskID}", from, to, "hour").
		WillReturnRows(sqlmock.NewRows([]string{
			"period", "requests", "client_errors", "server_errors", "latency_total_ms", "latency_max_ms",
			"latency_lt_10ms", "latency_lt_50ms", "latency_lt_100ms", "latency_lt_500ms",
			"latency_lt_1000ms", "latency_gte_1000ms", "status_counts",
		}).AddRow(from, 1, 0, 1, 1500, 1500, 0, 0, 0, 0, 0, 1, `{"503":1}`))

	handler := NewHandler(db, NewEngine(), testAuthenticator{}, nil)
	target := "/api/v1/system-monitor/analytics?method=GET&route=%2Fapi%2Fv1%2Ftasks%2F%7BtaskID%7D" +
		"&from=" + from.Format(time.RFC3339) + "&to=" + to.Format(time.RFC3339) + "&interval=hour"
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "opaque-session"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want OK; response: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"status_distribution":{"503":1}`) ||
		!strings.Contains(response.Body.String(), `"server_errors":1`) {
		t.Fatalf("analytics response missing endpoint error metrics: %s", response.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
