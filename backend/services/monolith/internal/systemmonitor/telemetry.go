package systemmonitor

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	eventCapacity     = 500
	recentBucketCount = 30
	recentBucketWidth = 10 * time.Second
)

var latencyBoundaries = [...]time.Duration{
	10 * time.Millisecond,
	50 * time.Millisecond,
	100 * time.Millisecond,
	500 * time.Millisecond,
	time.Second,
}

type Endpoint struct {
	Method      string `json:"method"`
	Route       string `json:"route"`
	Description string `json:"description"`
	Controller  string `json:"controller"`
	Pausable    bool   `json:"pausable"`
}

type EndpointControl struct {
	ResponseStatus  int       `json:"response_status"`
	ResponseMessage string    `json:"response_message"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type Event struct {
	OccurredAt time.Time `json:"occurred_at"`
	Type       string    `json:"type"`
	Method     string    `json:"method"`
	Route      string    `json:"route"`
	Status     int       `json:"status"`
	DurationMS int64     `json:"duration_ms"`
}

type EndpointHealth struct {
	Endpoint
	Status              string            `json:"health"`
	Requests            uint64            `json:"requests"`
	FailedRequests      uint64            `json:"failed_requests"`
	ErrorRate           float64           `json:"error_rate"`
	LastStatus          int               `json:"last_status,omitempty"`
	LastRequestAt       *time.Time        `json:"last_request_at,omitempty"`
	StatusDistribution  map[int]uint64    `json:"status_distribution"`
	LatencyBuckets      map[string]uint64 `json:"latency_buckets"`
	Paused              bool              `json:"paused"`
	PausedUntil         *time.Time        `json:"paused_until,omitempty"`
	PauseStatus         int               `json:"pause_status,omitempty"`
	PauseMessage        string            `json:"pause_message,omitempty"`
}

type Overview struct {
	StartedAt      time.Time       `json:"started_at"`
	UptimeSeconds  int64           `json:"uptime_seconds"`
	DatabaseStatus string          `json:"database_status"`
	Database       databaseMetrics `json:"database"`
	EndpointCount  int             `json:"endpoint_count"`
	AnalyticsPendingBuckets int    `json:"analytics_pending_buckets"`
	AnalyticsDroppedRequests uint64 `json:"analytics_dropped_requests"`
}

type databaseMetrics struct {
	OpenConnections int   `json:"open_connections"`
	InUse           int   `json:"in_use"`
	Idle            int   `json:"idle"`
	WaitCount       int64 `json:"wait_count"`
	WaitDurationMS  int64 `json:"wait_duration_ms"`
}

type windowBucket struct {
	startedAt    int64
	requests     uint64
	errors       uint64
	serverErrors uint64
}

type endpointMetric struct {
	requests           uint64
	failedRequests     uint64
	lastStatus         int
	lastRequestAt      time.Time
	statusDistribution map[int]uint64
	latencyBuckets     [len(latencyBoundaries) + 1]uint64
	recent             [recentBucketCount]windowBucket
}

type Engine struct {
	startedAt time.Time
	catalog   *endpointCatalog
	mu        sync.RWMutex
	metrics   map[string]*endpointMetric
	analytics map[analyticsKey]*analyticsDelta
	controlMu  sync.Mutex
	controlDB  *sql.DB
	logger     *slog.Logger
	controls   map[string]EndpointControl
	controlErr error
	controlAt  time.Time
	analyticsDropped uint64
	analyticsBufferAlerted bool
	events    [eventCapacity]Event
	eventHead int
	eventSize int
}

func NewEngine() *Engine {
	return &Engine{
		startedAt: time.Now().UTC(),
		catalog:   newEndpointCatalog(),
		metrics:   make(map[string]*endpointMetric),
		analytics: make(map[analyticsKey]*analyticsDelta),
	}
}

func (e *Engine) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		endpoint, known := e.catalog.Resolve(r.Method, r.URL.Path)
		if !known {
			next.ServeHTTP(w, r)
			return
		}
		writer := &statusResponseWriter{ResponseWriter: w, status: http.StatusOK}
		started := time.Now()
		if endpoint.Pausable {
			controls, err := e.activeControls()
			if err != nil && e.logger != nil {
				e.logger.WarnContext(r.Context(), "load endpoint controls", "error", err)
			}
			control, paused := controls[endpointKey(endpoint)]
			if err != nil && (!paused || !control.ExpiresAt.After(time.Now())) {
				writer.Header().Set("Cache-Control", "no-store")
				writer.Header().Set("Content-Type", "application/json; charset=utf-8")
				writer.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(writer).Encode(map[string]any{
					"error": map[string]string{"code": "endpoint_controls_unavailable"},
				})
				e.record(endpoint, writer.status, time.Since(started), time.Now().UTC())
				return
			}
			if paused && control.ExpiresAt.After(time.Now()) {
				writer.Header().Set("Cache-Control", "no-store")
				writer.Header().Set("Content-Type", "application/json; charset=utf-8")
				retryAfter := int(time.Until(control.ExpiresAt).Seconds())
				if retryAfter < 1 {
					retryAfter = 1
				}
				writer.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				writer.WriteHeader(control.ResponseStatus)
				_ = json.NewEncoder(writer).Encode(map[string]any{
					"error": map[string]string{
						"code": "endpoint_paused", "message": control.ResponseMessage,
					},
					"expires_at": control.ExpiresAt,
				})
				e.record(endpoint, writer.status, time.Since(started), time.Now().UTC())
				return
			}
		}
		next.ServeHTTP(writer, r)
		e.record(endpoint, writer.status, time.Since(started), time.Now().UTC())
	})
}

func (e *Engine) SetControlStore(db *sql.DB, logger *slog.Logger) {
	e.controlMu.Lock()
	defer e.controlMu.Unlock()
	e.controlDB = db
	e.logger = logger
	e.controlAt = time.Time{}
}

func (e *Engine) InvalidateControlCache() {
	e.controlMu.Lock()
	e.controlAt = time.Time{}
	e.controlMu.Unlock()
}

func (e *Engine) activeControls() (map[string]EndpointControl, error) {
	e.controlMu.Lock()
	defer e.controlMu.Unlock()
	if time.Since(e.controlAt) < time.Second {
		return e.controls, e.controlErr
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return e.loadControlsLocked(ctx)
}

func (e *Engine) RefreshControls(ctx context.Context) error {
	e.controlMu.Lock()
	defer e.controlMu.Unlock()
	_, err := e.loadControlsLocked(ctx)
	return err
}

func (e *Engine) loadControlsLocked(ctx context.Context) (map[string]EndpointControl, error) {
	if e.controlDB == nil {
		e.controls = nil
		e.controlErr = nil
		e.controlAt = time.Now()
		return e.controls, nil
	}
	rows, err := e.controlDB.QueryContext(ctx, `
		SELECT method, route, response_status, response_message, expires_at
		FROM system_monitor.endpoint_controls
		WHERE expires_at > now()
	`)
	if err != nil {
		e.controlErr = err
		e.controlAt = time.Now()
		return e.controls, err
	}
	controls := make(map[string]EndpointControl)
	for rows.Next() {
		var method, route, message string
		var control EndpointControl
		if err := rows.Scan(&method, &route, &control.ResponseStatus, &message, &control.ExpiresAt); err != nil {
			_ = rows.Close()
			e.controlErr = err
			e.controlAt = time.Now()
			return e.controls, err
		}
		control.ResponseMessage = message
		controls[method+" "+route] = control
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		e.controlErr = err
		e.controlAt = time.Now()
		return e.controls, err
	}
	if err := rows.Close(); err != nil {
		e.controlErr = err
		e.controlAt = time.Now()
		return e.controls, err
	}
	e.controls = controls
	e.controlErr = nil
	e.controlAt = time.Now()
	return e.controls, nil
}

func (e *Engine) Endpoints() []EndpointHealth {
	catalogEndpoints := e.catalog.Endpoints()
	now := time.Now()
	controls, _ := e.activeControls()
	result := make([]EndpointHealth, 0, len(catalogEndpoints))
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, endpoint := range catalogEndpoints {
		health := EndpointHealth{
			Endpoint:           endpoint,
			Status:             "unobserved",
			StatusDistribution: make(map[int]uint64),
			LatencyBuckets:     map[string]uint64{
				"<10ms": 0, "<50ms": 0, "<100ms": 0, "<500ms": 0, "<1000ms": 0, ">=1000ms": 0,
			},
		}
		metric := e.metrics[endpointKey(endpoint)]
		if control, paused := controls[endpointKey(endpoint)]; paused && control.ExpiresAt.After(now) {
			expiresAt := control.ExpiresAt
			health.Paused = true
			health.PausedUntil = &expiresAt
			health.PauseStatus = control.ResponseStatus
			health.PauseMessage = control.ResponseMessage
		}
		if metric != nil {
			health.Requests = metric.requests
			health.FailedRequests = metric.failedRequests
			health.LastStatus = metric.lastStatus
			if !metric.lastRequestAt.IsZero() {
				lastRequestAt := metric.lastRequestAt
				health.LastRequestAt = &lastRequestAt
			}
			for status, count := range metric.statusDistribution {
				health.StatusDistribution[status] = count
			}
			setLatencyBuckets(&health, metric.latencyBuckets)
			requests, errors, serverErrors := recentCounts(metric, now)
			if requests > 0 {
				health.ErrorRate = float64(errors) / float64(requests)
				switch {
				case serverErrors >= 10 && health.ErrorRate >= 0.5:
					health.Status = "unhealthy"
				case serverErrors >= 3 && health.ErrorRate >= 0.2:
					health.Status = "degraded"
				default:
					health.Status = "healthy"
				}
			}
		}
		result = append(result, health)
	}
	return result
}

func (e *Engine) Events(limit int) []Event {
	if limit < 1 {
		limit = 50
	}
	if limit > eventCapacity {
		limit = eventCapacity
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	count := e.eventSize
	if count > limit {
		count = limit
	}
	result := make([]Event, 0, count)
	start := (e.eventHead - count + eventCapacity) % eventCapacity
	for index := 0; index < count; index++ {
		result = append(result, e.events[(start+index)%eventCapacity])
	}
	return result
}

func (e *Engine) Overview(ctx context.Context, db *sql.DB) Overview {
	result := Overview{
		StartedAt:     e.startedAt,
		UptimeSeconds: int64(time.Since(e.startedAt).Seconds()),
		EndpointCount:  len(e.catalog.Endpoints()),
		DatabaseStatus: "unavailable",
	}
	if db == nil {
		e.mu.RLock()
		result.AnalyticsPendingBuckets = len(e.analytics)
		result.AnalyticsDroppedRequests = e.analyticsDropped
		e.mu.RUnlock()
		return result
	}
	stats := db.Stats()
	result.Database = databaseMetrics{
		OpenConnections: stats.OpenConnections,
		InUse: stats.InUse,
		Idle: stats.Idle,
		WaitCount: stats.WaitCount,
		WaitDurationMS: stats.WaitDuration.Milliseconds(),
	}
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if db.PingContext(pingCtx) == nil {
		result.DatabaseStatus = "ready"
	}
	e.mu.RLock()
	result.AnalyticsPendingBuckets = len(e.analytics)
	result.AnalyticsDroppedRequests = e.analyticsDropped
	e.mu.RUnlock()
	return result
}

func (e *Engine) record(endpoint Endpoint, status int, duration time.Duration, now time.Time) {
	key := endpointKey(endpoint)
	bucketStart := now.Unix() / int64(recentBucketWidth/time.Second)
	eventType := "http_request"
	if strings.HasPrefix(endpoint.Route, "/api/v1/auth/") &&
		(endpoint.Route == "/api/v1/auth/login" || strings.Contains(endpoint.Route, "/login")) {
		eventType = "authentication_attempt"
		if status >= http.StatusBadRequest {
			eventType = "authentication_failure"
		}
	} else if status >= http.StatusInternalServerError {
		eventType = "server_error"
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	metric := e.metrics[key]
	if metric == nil {
		metric = &endpointMetric{statusDistribution: make(map[int]uint64)}
		e.metrics[key] = metric
	}
	metric.requests++
	if status >= http.StatusBadRequest {
		metric.failedRequests++
	}
	metric.lastStatus = status
	metric.lastRequestAt = now
	metric.statusDistribution[status]++
	latencyIndex := sort.Search(len(latencyBoundaries), func(index int) bool {
		return duration < latencyBoundaries[index]
	})
	metric.latencyBuckets[latencyIndex]++
	recentIndex := int(bucketStart % recentBucketCount)
	if metric.recent[recentIndex].startedAt != bucketStart {
		metric.recent[recentIndex] = windowBucket{startedAt: bucketStart}
	}
	metric.recent[recentIndex].requests++
	if status >= http.StatusBadRequest {
		metric.recent[recentIndex].errors++
	}
	if status >= http.StatusInternalServerError {
		metric.recent[recentIndex].serverErrors++
	}
	if !e.addAnalytics(endpoint, status, duration, now) && !e.analyticsBufferAlerted {
		e.analyticsBufferAlerted = true
		e.appendEvent(Event{
			OccurredAt: now,
			Type:       "analytics_buffer_full",
			Method:     endpoint.Method,
			Route:      endpoint.Route,
			Status:     status,
		})
	}
	e.appendEvent(Event{
		OccurredAt: now,
		Type: eventType,
		Method: endpoint.Method,
		Route: endpoint.Route,
		Status: status,
		DurationMS: duration.Milliseconds(),
	})
	if eventType == "authentication_failure" {
		_, recentFailures, _ := recentCounts(metric, now)
		if recentFailures == 10 {
			e.appendEvent(Event{
				OccurredAt: now,
				Type:       "authentication_failure_burst",
				Method:     endpoint.Method,
				Route:      endpoint.Route,
				Status:     status,
			})
		}
	}
}

func (e *Engine) appendEvent(event Event) {
	e.events[e.eventHead] = event
	e.eventHead = (e.eventHead + 1) % eventCapacity
	if e.eventSize < eventCapacity {
		e.eventSize++
	}
}

func endpointKey(endpoint Endpoint) string {
	return endpoint.Method + " " + endpoint.Route
}

func recentCounts(metric *endpointMetric, now time.Time) (requests, errors, serverErrors uint64) {
	currentBucket := now.Unix() / int64(recentBucketWidth/time.Second)
	for _, bucket := range metric.recent {
		age := currentBucket - bucket.startedAt
		if bucket.requests > 0 && age >= 0 && age < recentBucketCount {
			requests += bucket.requests
			errors += bucket.errors
			serverErrors += bucket.serverErrors
		}
	}
	return requests, errors, serverErrors
}

func setLatencyBuckets(health *EndpointHealth, buckets [len(latencyBoundaries) + 1]uint64) {
	health.LatencyBuckets["<10ms"] = buckets[0]
	health.LatencyBuckets["<50ms"] = buckets[1]
	health.LatencyBuckets["<100ms"] = buckets[2]
	health.LatencyBuckets["<500ms"] = buckets[3]
	health.LatencyBuckets["<1000ms"] = buckets[4]
	health.LatencyBuckets[">=1000ms"] = buckets[5]
}

type statusResponseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusResponseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.status = status
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusResponseWriter) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *statusResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *statusResponseWriter) FlushError() error {
	return http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *statusResponseWriter) Flush() {
	_ = w.FlushError()
}
