-- Phase 3 (Infrastructure Platform): Remote Monitoring.
--
-- Stores metric snapshots collected from registered servers via the SSH
-- engine. The local server's metrics continue to flow through the
-- in-memory Recorder ring buffer; this table is for *remote* servers.
--
-- Keep one row per (server_id, ts) sample. Timestamps are RFC3339 for
-- portability and time-series ordering. Numerics are REAL to preserve
-- fractional percentages.

CREATE TABLE IF NOT EXISTS server_metrics (
  id TEXT PRIMARY KEY,
  server_id TEXT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  ts TEXT NOT NULL,                        -- RFC3339Nano UTC
  cpu_usage REAL NOT NULL DEFAULT 0,       -- percent (0..100)
  cpu_load1 REAL NOT NULL DEFAULT 0,
  cpu_load5 REAL NOT NULL DEFAULT 0,
  cpu_load15 REAL NOT NULL DEFAULT 0,
  mem_total REAL NOT NULL DEFAULT 0,       -- bytes
  mem_used REAL NOT NULL DEFAULT 0,
  mem_percent REAL NOT NULL DEFAULT 0,      -- percent
  disk_total REAL NOT NULL DEFAULT 0,      -- bytes (root fs)
  disk_used REAL NOT NULL DEFAULT 0,
  disk_percent REAL NOT NULL DEFAULT 0,
  net_bytes_sent REAL NOT NULL DEFAULT 0,   -- cumulative
  net_bytes_recv REAL NOT NULL DEFAULT 0,
  uptime REAL NOT NULL DEFAULT 0,          -- seconds
  error TEXT NOT NULL DEFAULT ''           -- collection error, if any
);
CREATE INDEX IF NOT EXISTS idx_metrics_server_ts ON server_metrics(server_id, ts);
CREATE INDEX IF NOT EXISTS idx_metrics_ts ON server_metrics(ts);
