-- PostgreSQL version of 008_server_metrics.sql
-- Phase 3 (Infrastructure Platform): Remote Monitoring.

CREATE TABLE IF NOT EXISTS server_metrics (
  id TEXT PRIMARY KEY,
  server_id TEXT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  ts TEXT NOT NULL,                        -- RFC3339Nano UTC
  cpu_usage DOUBLE PRECISION NOT NULL DEFAULT 0,
  cpu_load1 DOUBLE PRECISION NOT NULL DEFAULT 0,
  cpu_load5 DOUBLE PRECISION NOT NULL DEFAULT 0,
  cpu_load15 DOUBLE PRECISION NOT NULL DEFAULT 0,
  mem_total DOUBLE PRECISION NOT NULL DEFAULT 0,
  mem_used DOUBLE PRECISION NOT NULL DEFAULT 0,
  mem_percent DOUBLE PRECISION NOT NULL DEFAULT 0,
  disk_total DOUBLE PRECISION NOT NULL DEFAULT 0,
  disk_used DOUBLE PRECISION NOT NULL DEFAULT 0,
  disk_percent DOUBLE PRECISION NOT NULL DEFAULT 0,
  net_bytes_sent DOUBLE PRECISION NOT NULL DEFAULT 0,
  net_bytes_recv DOUBLE PRECISION NOT NULL DEFAULT 0,
  uptime DOUBLE PRECISION NOT NULL DEFAULT 0,
  error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_metrics_server_ts ON server_metrics(server_id, ts);
CREATE INDEX IF NOT EXISTS idx_metrics_ts ON server_metrics(ts);
