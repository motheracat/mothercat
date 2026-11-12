package systemmonitor

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"time"
)

const (
	analyticsBucketWidth  = time.Minute
	analyticsFlushPeriod  = 5 * time.Second
	maxAnalyticsRange     = 90 * 24 * time.Hour
	maxAnalyticsPoints    = 2000
	maxPendingBuckets     = 10000
)

var analyticsIntervals = map[string]time.Duration{
	"minute": time.Minute,
	"hour":   time.Hour,
	"day":    24 * time.Hour,
}

var (
	ErrEndpointNotFound      = errors.New("System Monitor endpoint not found")
	ErrInvalidAnalyticsQuery = errors.New("invalid endpoint analytics query")
)

type analyticsKey struct {
	method string
	route  string
	bucket time.Time
}

type analyticsDelta struct {
	requests         uint64
	clientErrors     uint64
	serverErrors     uint64
	latencyTotalMS   uint64
	latencyMaxMS     uint64
	latencyBuckets   [len(latencyBoundaries) + 1]uint64
	statusCounts     map[int]uint64
}

type AnalyticsQuery struct {
	Method   string
	Route    string
	From     time.Time
	To       time.Time
	Interval string
}

type AnalyticsPoint struct {
	Bucket              time.Time         `json:"bucket"`
	Requests            uint64            `json:"requests"`
	Successes           uint64            `json:"successes"`
	ClientErrors        uint64            `json:"client_errors"`
	ServerErrors        uint64            `json:"server_errors"`
	ErrorRate           float64           `json:"error_rate"`
	AverageLatencyMS    float64           `json:"average_latency_ms"`
	P95LatencyMS        int64             `json:"p95_latency_ms"`
	MaximumLatencyMS    uint64            `json:"maximum_latency_ms"`
	StatusDistribution  map[string]uint64 `json:"status_distribution"`
	LatencyDistribution map[string]uint64 `json:"latency_distribution"`
}

type AnalyticsReport struct {
	Method   string           `json:"method"`
	Route    string           `json:"route"`
	From     time.Time        `json:"from"`
	To       time.Time        `json:"to"`
	Interval string           `json:"interval"`
	Points   []AnalyticsPoint `json:"points"`
}

type storedAnalyticsPoint struct {
	bucket           time.Time
	requests         uint64
	clientErrors     uint64
	serverErrors     uint64
	latencyTotalMS   uint64
	latencyMaxMS     uint64
	latencyBuckets   [len(latencyBoundaries) + 1]uint64
	statusCountsJSON []byte
}

func (e *Engine) addAnalytics(endpoint Endpoint, status int, duration time.Duration, now time.Time) bool {
	bucket := now.UTC().Truncate(analyticsBucketWidth)
	key := analyticsKey{method: endpoint.Method, route: endpoint.Route, bucket: bucket}
	delta := e.analytics[key]
	if delta == nil {
		if len(e.analytics) >= maxPendingBuckets {
			e.analyticsDropped++
			return false
		}
		delta = &analyticsDelta{statusCounts: make(map[int]uint64)}
		e.analytics[key] = delta
	}
	delta.requests++
	if status >= 400 && status < 500 {
		delta.clientErrors++
	}
	if status >= 500 {
		delta.serverErrors++
	}
	if duration > 0 {
		latencyMS := uint64(math.Ceil(float64(duration) / float64(time.Millisecond)))
		delta.latencyTotalMS += latencyMS
		if latencyMS > delta.latencyMaxMS {
			delta.latencyMaxMS = latencyMS
		}
	}
	latencyIndex := sort.Search(len(latencyBoundaries), func(index int) bool {
		return duration < latencyBoundaries[index]
	})
	delta.latencyBuckets[latencyIndex]++
	delta.statusCounts[status]++
	return true
}

func (e *Engine) FlushAnalytics(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return errors.New("System Monitor analytics database is unavailable")
	}
	e.mu.Lock()
	if len(e.analytics) == 0 {
		e.analyticsBufferAlerted = false
		e.mu.Unlock()
		return nil
	}
	pending := e.analytics
	e.analytics = make(map[analyticsKey]*analyticsDelta)
	e.mu.Unlock()

	keys := make([]analyticsKey, 0, len(pending))
	for key := range pending {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].bucket.Equal(keys[j].bucket) {
			if keys[i].route == keys[j].route {
				return keys[i].method < keys[j].method
			}
			return keys[i].route < keys[j].route
		}
		return keys[i].bucket.Before(keys[j].bucket)
	})

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		e.restoreAnalytics(pending)
		return fmt.Errorf("begin endpoint analytics flush: %w", err)
	}
	for _, key := range keys {
		delta := pending[key]
		statusCounts := make(map[string]uint64, len(delta.statusCounts))
		for status, count := range delta.statusCounts {
			statusCounts[strconv.Itoa(status)] = count
		}
		statusJSON, err := json.Marshal(statusCounts)
		if err != nil {
			_ = tx.Rollback()
			e.restoreAnalytics(pending)
			return fmt.Errorf("encode endpoint analytics status counts: %w", err)
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO system_monitor.endpoint_analytics AS stored (
				method, route, bucket_start, request_count, client_error_count,
				server_error_count, latency_total_ms, latency_max_ms,
				latency_lt_10ms, latency_lt_50ms, latency_lt_100ms,
				latency_lt_500ms, latency_lt_1000ms, latency_gte_1000ms, status_counts
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15::jsonb)
			ON CONFLICT (method, route, bucket_start) DO UPDATE SET
				request_count = stored.request_count + EXCLUDED.request_count,
				client_error_count = stored.client_error_count + EXCLUDED.client_error_count,
				server_error_count = stored.server_error_count + EXCLUDED.server_error_count,
				latency_total_ms = stored.latency_total_ms + EXCLUDED.latency_total_ms,
				latency_max_ms = GREATEST(stored.latency_max_ms, EXCLUDED.latency_max_ms),
				latency_lt_10ms = stored.latency_lt_10ms + EXCLUDED.latency_lt_10ms,
				latency_lt_50ms = stored.latency_lt_50ms + EXCLUDED.latency_lt_50ms,
				latency_lt_100ms = stored.latency_lt_100ms + EXCLUDED.latency_lt_100ms,
				latency_lt_500ms = stored.latency_lt_500ms + EXCLUDED.latency_lt_500ms,
				latency_lt_1000ms = stored.latency_lt_1000ms + EXCLUDED.latency_lt_1000ms,
				latency_gte_1000ms = stored.latency_gte_1000ms + EXCLUDED.latency_gte_1000ms,
				status_counts = system_monitor.add_jsonb_counters(stored.status_counts, EXCLUDED.status_counts)
		`, key.method, key.route, key.bucket, delta.requests, delta.clientErrors, delta.serverErrors,
			delta.latencyTotalMS, delta.latencyMaxMS, delta.latencyBuckets[0], delta.latencyBuckets[1],
			delta.latencyBuckets[2], delta.latencyBuckets[3], delta.latencyBuckets[4],
			delta.latencyBuckets[5], string(statusJSON))
		if err != nil {
			_ = tx.Rollback()
			e.restoreAnalytics(pending)
			return fmt.Errorf("upsert endpoint analytics bucket: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		e.restoreAnalytics(pending)
		return fmt.Errorf("commit endpoint analytics flush: %w", err)
	}
	e.mu.Lock()
	e.analyticsBufferAlerted = false
	e.mu.Unlock()
	return nil
}

func (e *Engine) restoreAnalytics(pending map[analyticsKey]*analyticsDelta) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for key, older := range pending {
		current := e.analytics[key]
		if current == nil {
			e.analytics[key] = older
			continue
		}
		current.requests += older.requests
		current.clientErrors += older.clientErrors
		current.serverErrors += older.serverErrors
		current.latencyTotalMS += older.latencyTotalMS
		if older.latencyMaxMS > current.latencyMaxMS {
			current.latencyMaxMS = older.latencyMaxMS
		}
		for index, count := range older.latencyBuckets {
			current.latencyBuckets[index] += count
		}
		for status, count := range older.statusCounts {
			current.statusCounts[status] += count
		}
	}
}

func RunAnalyticsFlusher(ctx context.Context, db *sql.DB, engine *Engine, logger *slog.Logger) {
	if db == nil || engine == nil {
		if logger != nil {
			logger.ErrorContext(ctx, "run endpoint analytics flusher", "error", "database or engine unavailable")
		}
		return
	}
	flush := func(flushCtx context.Context) {
		if err := engine.FlushAnalytics(flushCtx, db); err != nil && logger != nil {
			logger.ErrorContext(flushCtx, "flush endpoint analytics", "error", err)
		}
	}
	prune := func(pruneCtx context.Context) {
		result, err := db.ExecContext(pruneCtx, `
			DELETE FROM system_monitor.endpoint_analytics
			WHERE bucket_start < now() - interval '90 days'
		`)
		if err != nil {
			if logger != nil {
				logger.ErrorContext(pruneCtx, "prune endpoint analytics", "error", err)
			}
			return
		}
		deleted, err := result.RowsAffected()
		if err != nil {
			if logger != nil {
				logger.ErrorContext(pruneCtx, "count pruned endpoint analytics", "error", err)
			}
			return
		}
		if deleted > 0 && logger != nil {
			logger.InfoContext(pruneCtx, "pruned endpoint analytics", "buckets", deleted)
		}
	}
	prune(ctx)
	ticker := time.NewTicker(analyticsFlushPeriod)
	defer ticker.Stop()
	pruneTicker := time.NewTicker(24 * time.Hour)
	defer pruneTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			flush(flushCtx)
			return
		case <-ticker.C:
			flush(ctx)
		case <-pruneTicker.C:
			prune(ctx)
		}
	}
}

func EndpointAnalytics(ctx context.Context, db *sql.DB, catalog *endpointCatalog, query AnalyticsQuery) (AnalyticsReport, error) {
	endpoint, exists := catalog.Lookup(query.Method, query.Route)
	if !exists {
		return AnalyticsReport{}, ErrEndpointNotFound
	}
	intervalWidth, validInterval := analyticsIntervals[query.Interval]
	if !validInterval || !query.To.After(query.From) ||
		query.To.Sub(query.From) > maxAnalyticsRange {
		return AnalyticsReport{}, ErrInvalidAnalyticsQuery
	}
	if db == nil {
		return AnalyticsReport{}, errors.New("System Monitor analytics database is unavailable")
	}
	from := query.From.UTC()
	firstBucket := from.Truncate(intervalWidth)
	to := query.To.UTC()
	pointCount := int(math.Ceil(to.Sub(firstBucket).Seconds() / intervalWidth.Seconds()))
	if pointCount < 1 || pointCount > maxAnalyticsPoints {
		return AnalyticsReport{}, ErrInvalidAnalyticsQuery
	}

	rows, err := db.QueryContext(ctx, `
		WITH filtered AS (
			SELECT *
			FROM system_monitor.endpoint_analytics
			WHERE method = $1 AND route = $2
			  AND bucket_start >= $3 AND bucket_start < $4
		),
		period_metrics AS (
			SELECT date_trunc($5, bucket_start AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' AS period,
			       SUM(request_count) AS requests,
			       SUM(client_error_count) AS client_errors,
			       SUM(server_error_count) AS server_errors,
			       SUM(latency_total_ms) AS latency_total_ms,
			       MAX(latency_max_ms) AS latency_max_ms,
			       SUM(latency_lt_10ms) AS latency_lt_10ms,
			       SUM(latency_lt_50ms) AS latency_lt_50ms,
			       SUM(latency_lt_100ms) AS latency_lt_100ms,
			       SUM(latency_lt_500ms) AS latency_lt_500ms,
			       SUM(latency_lt_1000ms) AS latency_lt_1000ms,
			       SUM(latency_gte_1000ms) AS latency_gte_1000ms
			FROM filtered
			GROUP BY 1
		),
		period_status_counts AS (
			SELECT date_trunc($5, f.bucket_start AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' AS period,
			       counters.key AS status,
			       SUM(counters.value::bigint) AS status_count
			FROM filtered f
			CROSS JOIN LATERAL jsonb_each_text(f.status_counts) AS counters
			GROUP BY 1, 2
		),
		status_distribution AS (
			SELECT period, jsonb_object_agg(status, to_jsonb(status_count)) AS counts
			FROM period_status_counts
			GROUP BY period
		)
		SELECT metrics.period, metrics.requests, metrics.client_errors, metrics.server_errors,
		       metrics.latency_total_ms, metrics.latency_max_ms,
		       metrics.latency_lt_10ms, metrics.latency_lt_50ms, metrics.latency_lt_100ms,
		       metrics.latency_lt_500ms, metrics.latency_lt_1000ms, metrics.latency_gte_1000ms,
		       COALESCE(status_distribution.counts, '{}'::jsonb)
		FROM period_metrics AS metrics
		LEFT JOIN status_distribution USING (period)
		ORDER BY metrics.period
	`, endpoint.Method, endpoint.Route, from, to, query.Interval)
	if err != nil {
		return AnalyticsReport{}, fmt.Errorf("query endpoint analytics: %w", err)
	}
	defer func() { _ = rows.Close() }()

	pointsByBucket := make(map[time.Time]AnalyticsPoint, pointCount)
	for rows.Next() {
		var stored storedAnalyticsPoint
		if err := rows.Scan(&stored.bucket, &stored.requests, &stored.clientErrors, &stored.serverErrors,
			&stored.latencyTotalMS, &stored.latencyMaxMS,
			&stored.latencyBuckets[0], &stored.latencyBuckets[1], &stored.latencyBuckets[2],
			&stored.latencyBuckets[3], &stored.latencyBuckets[4], &stored.latencyBuckets[5],
			&stored.statusCountsJSON); err != nil {
			return AnalyticsReport{}, fmt.Errorf("read endpoint analytics point: %w", err)
		}
		var statusCounts map[string]uint64
		if err := json.Unmarshal(stored.statusCountsJSON, &statusCounts); err != nil {
			return AnalyticsReport{}, fmt.Errorf("decode endpoint status distribution: %w", err)
		}
		pointsByBucket[stored.bucket.UTC()] = analyticsPoint(stored, statusCounts)
	}
	if err := rows.Err(); err != nil {
		return AnalyticsReport{}, fmt.Errorf("iterate endpoint analytics: %w", err)
	}
	if err := rows.Close(); err != nil {
		return AnalyticsReport{}, fmt.Errorf("close endpoint analytics: %w", err)
	}

	points := make([]AnalyticsPoint, 0, pointCount)
	for bucket := firstBucket; bucket.Before(to); bucket = bucket.Add(intervalWidth) {
		point, exists := pointsByBucket[bucket]
		if !exists {
			point = emptyAnalyticsPoint(bucket)
		}
		points = append(points, point)
	}
	return AnalyticsReport{
		Method: endpoint.Method, Route: endpoint.Route, From: from,
		To: to, Interval: query.Interval, Points: points,
	}, nil
}

func analyticsPoint(stored storedAnalyticsPoint, statusCounts map[string]uint64) AnalyticsPoint {
	point := emptyAnalyticsPoint(stored.bucket.UTC())
	point.Requests = stored.requests
	point.ClientErrors = stored.clientErrors
	point.ServerErrors = stored.serverErrors
	if point.Requests >= point.ClientErrors+point.ServerErrors {
		point.Successes = point.Requests - point.ClientErrors - point.ServerErrors
	}
	if point.Requests > 0 {
		point.ErrorRate = float64(point.ClientErrors+point.ServerErrors) / float64(point.Requests)
		point.AverageLatencyMS = float64(stored.latencyTotalMS) / float64(point.Requests)
	}
	point.MaximumLatencyMS = stored.latencyMaxMS
	point.StatusDistribution = statusCounts
	point.LatencyDistribution = map[string]uint64{
		"<10ms": stored.latencyBuckets[0], "<50ms": stored.latencyBuckets[1],
		"<100ms": stored.latencyBuckets[2], "<500ms": stored.latencyBuckets[3],
		"<1000ms": stored.latencyBuckets[4], ">=1000ms": stored.latencyBuckets[5],
	}
	point.P95LatencyMS = approximateLatencyPercentile(stored.latencyBuckets, point.Requests, 0.95)
	return point
}

func emptyAnalyticsPoint(bucket time.Time) AnalyticsPoint {
	return AnalyticsPoint{
		Bucket: bucket,
		StatusDistribution: make(map[string]uint64),
		LatencyDistribution: map[string]uint64{
			"<10ms": 0, "<50ms": 0, "<100ms": 0, "<500ms": 0, "<1000ms": 0, ">=1000ms": 0,
		},
	}
}

func approximateLatencyPercentile(buckets [len(latencyBoundaries) + 1]uint64, requests uint64, percentile float64) int64 {
	if requests == 0 {
		return 0
	}
	target := uint64(math.Ceil(float64(requests) * percentile))
	var cumulative uint64
	for index, count := range buckets {
		cumulative += count
		if cumulative >= target {
			if index < len(latencyBoundaries) {
				return latencyBoundaries[index].Milliseconds()
			}
			return latencyBoundaries[len(latencyBoundaries)-1].Milliseconds()
		}
	}
	return latencyBoundaries[len(latencyBoundaries)-1].Milliseconds()
}
