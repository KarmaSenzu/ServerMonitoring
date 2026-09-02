-- PostgreSQL version of 010_tunnels.sql
-- Phase 8 (Infrastructure Platform): SSH Tunnel Manager.

CREATE TABLE IF NOT EXISTS tunnels (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  server_id TEXT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  type TEXT NOT NULL CHECK(type IN ('local','remote','socks')),
  local_addr TEXT NOT NULL DEFAULT '',
  remote_addr TEXT NOT NULL DEFAULT '',
  auto_start INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL CHECK(status IN ('stopped','connecting','active','error')) DEFAULT 'stopped',
  started_at TEXT NOT NULL DEFAULT '',
  started_by TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_tunnels_server ON tunnels(server_id);
CREATE INDEX IF NOT EXISTS idx_tunnels_status ON tunnels(status);
CREATE INDEX IF NOT EXISTS idx_tunnels_type ON tunnels(type);
