-- ClickHouse Schema for Log-Zero
-- Run this against your ClickHouse instance

-- Create database
CREATE DATABASE IF NOT EXISTS logzero;

USE logzero;

-- Compressed logs table
CREATE TABLE IF NOT EXISTS compressed_logs (
    log_id UUID DEFAULT generateUUIDv4(),
    timestamp DateTime64(3),
    template_id String,
    source String,
    variables Map(String, String),
    original_size UInt32,
    compressed_size UInt32,
    created_at DateTime DEFAULT now()
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(timestamp)
ORDER BY (source, template_id, timestamp)
TTL timestamp + INTERVAL 90 DAY
SETTINGS index_granularity = 8192;

-- Templates table
CREATE TABLE IF NOT EXISTS templates (
    template_id String,
    pattern String,
    log_count UInt64,
    first_seen DateTime64(3),
    last_seen DateTime64(3),
    created_at DateTime DEFAULT now()
) ENGINE = ReplacingMergeTree(last_seen)
ORDER BY template_id;

-- Aggregation materialized view for analytics
CREATE MATERIALIZED VIEW IF NOT EXISTS logs_by_template_hourly
ENGINE = SummingMergeTree()
ORDER BY (source, template_id, hour)
AS SELECT
    source,
    template_id,
    toStartOfHour(timestamp) as hour,
    count() as log_count,
    sum(original_size) as total_original_size,
    sum(compressed_size) as total_compressed_size
FROM compressed_logs
GROUP BY source, template_id, hour;

-- Error rate tracking view
CREATE MATERIALIZED VIEW IF NOT EXISTS error_rates
ENGINE = SummingMergeTree()
ORDER BY (source, minute)
AS SELECT
    source,
    toStartOfMinute(timestamp) as minute,
    countIf(pattern LIKE '%ERROR%') as error_count,
    countIf(pattern LIKE '%WARN%') as warn_count,
    count() as total_count
FROM compressed_logs
LEFT JOIN templates ON compressed_logs.template_id = templates.template_id
GROUP BY source, minute;

-- Sample queries for verification
-- SELECT template_id, count() as cnt FROM compressed_logs GROUP BY template_id ORDER BY cnt DESC LIMIT 10;
-- SELECT source, sum(log_count) as total, sum(total_original_size) as orig, sum(total_compressed_size) as comp FROM logs_by_template_hourly GROUP BY source;
