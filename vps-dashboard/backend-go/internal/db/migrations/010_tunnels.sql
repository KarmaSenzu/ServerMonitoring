-- Phase 8 (Infrastructure Platform): SSH Tunnel Manager.
--
-- Stores tunnel definitions and their live status. Tunnels are
-- persistent definitions; the manager starts/stops them on demand
-- using the SSH engine (PROJECT ARCHITECTURE.md §22).

CREATE TABLE IF NOT EXISTS tunnels (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  server_id TEXT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  type TEXT NOT NULL CHECK(type IN ('local','remote','socks')),
  local_addr TEXT NOT NULL DEFAULT '',     -- e.g. "127.0.0.1:5432" or "0.0.0.0:8080"
  remote_addr TEXT NOT NULL DEFAULT '',   -- e.g. "db.internal:5432" (empty for socks)
  auto_start INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL CHECK(status IN ('stopped','connecting','active','error')) DEFAULT 'stopped',
  started_at TEXT NOT NULL DEFAULT '',
  started_by TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_tunnels_server ON tunnels(server_id);
CREATE INDEX IF NOT EXISTS idx_tunnels_status ON tunnels(status);
CREATE INDEX IF NOT EXISTS idx_tunnels_type ON tunnels(type);
