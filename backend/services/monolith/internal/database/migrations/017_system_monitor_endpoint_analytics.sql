CREATE TABLE system_monitor.endpoint_analytics (
    method varchar(10) NOT NULL,
    route varchar(300) NOT NULL,
    bucket_start timestamptz NOT NULL,
    request_count bigint NOT NULL CHECK (request_count >= 0),
    client_error_count bigint NOT NULL CHECK (client_error_count >= 0),
    server_error_count bigint NOT NULL CHECK (server_error_count >= 0),
    latency_total_ms bigint NOT NULL CHECK (latency_total_ms >= 0),
    latency_max_ms bigint NOT NULL CHECK (latency_max_ms >= 0),
    latency_lt_10ms bigint NOT NULL CHECK (latency_lt_10ms >= 0),
    latency_lt_50ms bigint NOT NULL CHECK (latency_lt_50ms >= 0),
    latency_lt_100ms bigint NOT NULL CHECK (latency_lt_100ms >= 0),
    latency_lt_500ms bigint NOT NULL CHECK (latency_lt_500ms >= 0),
    latency_lt_1000ms bigint NOT NULL CHECK (latency_lt_1000ms >= 0),
    latency_gte_1000ms bigint NOT NULL CHECK (latency_gte_1000ms >= 0),
    status_counts jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(status_counts) = 'object'),
    PRIMARY KEY (method, route, bucket_start)
);

CREATE INDEX system_monitor_endpoint_analytics_bucket_idx
    ON system_monitor.endpoint_analytics (bucket_start DESC);

CREATE FUNCTION system_monitor.add_jsonb_counters(current_counts jsonb, delta_counts jsonb)
RETURNS jsonb
LANGUAGE sql
IMMUTABLE
AS $$
    SELECT COALESCE(jsonb_object_agg(status, to_jsonb(total)), '{}'::jsonb)
    FROM (
        SELECT key AS status, SUM(value::bigint) AS total
        FROM (
            SELECT key, value FROM jsonb_each_text(current_counts)
            UNION ALL
            SELECT key, value FROM jsonb_each_text(delta_counts)
        ) AS counts
        GROUP BY key
    ) AS merged
$$;
