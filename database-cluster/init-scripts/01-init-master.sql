-- ===============================================================================
-- PostgreSQL Master Initialization Script
-- Dijalankan otomatis saat container pertama kali start
-- ===============================================================================

-- Enable TimescaleDB extension (untuk time-series optimization)
CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE;

-- Enable useful extensions
CREATE EXTENSION IF NOT EXISTS pg_stat_statements; -- Query performance tracking
CREATE EXTENSION IF NOT EXISTS pgcrypto;           -- Crypto functions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";        -- UUID generation

-- ===============================================================================
-- REPLICATION USER SETUP
-- ===============================================================================
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_user WHERE usename = 'replicator') THEN
        CREATE ROLE replicator WITH REPLICATION PASSWORD 'REPLICATION_PASSWORD_HERE' LOGIN;
        RAISE NOTICE 'Replication user created';
    END IF;
END
$$;

-- Grant necessary permissions untuk replication user
GRANT CONNECT ON DATABASE postgres TO replicator;

-- ===============================================================================
-- DATABASE EXAMPLES (uncomment untuk auto-create databases)
-- ===============================================================================

-- Database untuk monitoring server
-- CREATE DATABASE monitoring_db OWNER dbadmin;
-- COMMENT ON DATABASE monitoring_db IS 'VPS Monitoring & Metrics Database';

-- Database untuk aplikasi lain (contoh)
-- CREATE DATABASE webapp_db OWNER dbadmin;
-- CREATE DATABASE ecommerce_db OWNER dbadmin;
-- CREATE DATABASE analytics_db OWNER dbadmin;

-- ===============================================================================
-- SAMPLE SCHEMA: Monitoring Metrics (Time-Series)
-- ===============================================================================

-- Uncomment untuk create sample monitoring schema
/*
\c monitoring_db

CREATE SCHEMA IF NOT EXISTS metrics;

-- CPU Metrics Table (time-series)
CREATE TABLE IF NOT EXISTS metrics.cpu_usage (
    time        TIMESTAMPTZ NOT NULL,
    server_id   TEXT NOT NULL,
    usage       DOUBLE PRECISION NOT NULL,
    cores       INTEGER,
    load_avg_1  DOUBLE PRECISION,
    load_avg_5  DOUBLE PRECISION,
    load_avg_15 DOUBLE PRECISION
);

-- Convert to TimescaleDB hypertable (auto-partitioning by time)
SELECT create_hypertable('metrics.cpu_usage', 'time', if_not_exists => TRUE);

-- Memory Metrics Table
CREATE TABLE IF NOT EXISTS metrics.memory_usage (
    time        TIMESTAMPTZ NOT NULL,
    server_id   TEXT NOT NULL,
    total       BIGINT NOT NULL,
    used        BIGINT NOT NULL,
    free        BIGINT NOT NULL,
    available   BIGINT NOT NULL,
    percent     DOUBLE PRECISION NOT NULL
);

SELECT create_hypertable('metrics.memory_usage', 'time', if_not_exists => TRUE);

-- Disk Metrics Table
CREATE TABLE IF NOT EXISTS metrics.disk_usage (
    time        TIMESTAMPTZ NOT NULL,
    server_id   TEXT NOT NULL,
    mount_point TEXT NOT NULL,
    total       BIGINT NOT NULL,
    used        BIGINT NOT NULL,
    free        BIGINT NOT NULL,
    percent     DOUBLE PRECISION NOT NULL
);

SELECT create_hypertable('metrics.disk_usage', 'time', if_not_exists => TRUE);

-- Network Metrics Table
CREATE TABLE IF NOT EXISTS metrics.network_stats (
    time           TIMESTAMPTZ NOT NULL,
    server_id      TEXT NOT NULL,
    interface      TEXT NOT NULL,
    bytes_sent     BIGINT NOT NULL,
    bytes_recv     BIGINT NOT NULL,
    packets_sent   BIGINT NOT NULL,
    packets_recv   BIGINT NOT NULL,
    errors_in      BIGINT,
    errors_out     BIGINT
);

SELECT create_hypertable('metrics.network_stats', 'time', if_not_exists => TRUE);

-- Create indexes untuk faster queries
CREATE INDEX IF NOT EXISTS idx_cpu_server_time ON metrics.cpu_usage (server_id, time DESC);
CREATE INDEX IF NOT EXISTS idx_memory_server_time ON metrics.memory_usage (server_id, time DESC);
CREATE INDEX IF NOT EXISTS idx_disk_server_time ON metrics.disk_usage (server_id, time DESC);
CREATE INDEX IF NOT EXISTS idx_network_server_time ON metrics.network_stats (server_id, time DESC);

-- Retention policy: Auto-delete data older than 90 days
SELECT add_retention_policy('metrics.cpu_usage', INTERVAL '90 days', if_not_exists => TRUE);
SELECT add_retention_policy('metrics.memory_usage', INTERVAL '90 days', if_not_exists => TRUE);
SELECT add_retention_policy('metrics.disk_usage', INTERVAL '90 days', if_not_exists => TRUE);
SELECT add_retention_policy('metrics.network_stats', INTERVAL '90 days', if_not_exists => TRUE);

-- Compression policy: Auto-compress data older than 7 days
SELECT add_compression_policy('metrics.cpu_usage', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_compression_policy('metrics.memory_usage', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_compression_policy('metrics.disk_usage', INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_compression_policy('metrics.network_stats', INTERVAL '7 days', if_not_exists => TRUE);

-- Sample continuous aggregates (pre-computed materialized views)
CREATE MATERIALIZED VIEW IF NOT EXISTS metrics.cpu_usage_hourly
WITH (timescaledb.continuous) AS
SELECT 
    time_bucket('1 hour', time) AS hour,
    server_id,
    AVG(usage) as avg_usage,
    MAX(usage) as max_usage,
    MIN(usage) as min_usage
FROM metrics.cpu_usage
GROUP BY hour, server_id
WITH NO DATA;

SELECT add_continuous_aggregate_policy('metrics.cpu_usage_hourly',
    start_offset => INTERVAL '7 days',
    end_offset => INTERVAL '1 hour',
    schedule_interval => INTERVAL '1 hour',
    if_not_exists => TRUE
);

GRANT ALL ON SCHEMA metrics TO dbadmin;
GRANT SELECT ON ALL TABLES IN SCHEMA metrics TO replicator;
*/

-- ===============================================================================
-- COMPLETION
-- ===============================================================================
\echo '✓ Master database initialization completed'
\echo '✓ TimescaleDB extension enabled'
\echo '✓ Replication user configured'
\echo ''
\echo 'Next steps:'
\echo '1. Uncomment database creation sections as needed'
\echo '2. Connect slaves to master'
\echo '3. Verify replication status: SELECT * FROM pg_stat_replication;'
