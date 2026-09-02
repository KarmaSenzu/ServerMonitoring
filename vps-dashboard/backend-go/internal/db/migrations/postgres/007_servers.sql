-- PostgreSQL version of 007_servers.sql
-- Phase 1 (Infrastructure Platform): Server Registry.

CREATE TABLE IF NOT EXISTS servers (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  hostname TEXT NOT NULL,
  ip_address TEXT NOT NULL DEFAULT '',
  ssh_port INTEGER NOT NULL DEFAULT 22,
  ssh_username TEXT NOT NULL DEFAULT '',
  credential_type TEXT NOT NULL CHECK(credential_type IN ('ssh_key','password','agent')) DEFAULT 'ssh_key',
  credential_ref TEXT NOT NULL DEFAULT '',
  operating_system TEXT NOT NULL DEFAULT '',
  architecture TEXT NOT NULL DEFAULT '',
  provider TEXT NOT NULL DEFAULT '',
  provider_instance_id TEXT NOT NULL DEFAULT '',
  environment TEXT NOT NULL CHECK(environment IN ('development','staging','production')) DEFAULT 'production',
  status TEXT NOT NULL CHECK(status IN ('online','degraded','offline','unknown')) DEFAULT 'unknown',
  status_detail TEXT NOT NULL DEFAULT '',
  last_seen_at TEXT NOT NULL DEFAULT '',
  notes TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_servers_hostname ON servers(hostname);
CREATE INDEX IF NOT EXISTS idx_servers_status ON servers(status);
CREATE INDEX IF NOT EXISTS idx_servers_environment ON servers(environment);
CREATE INDEX IF NOT EXISTS idx_servers_provider ON servers(provider);

-- Normalised tag catalogue.
CREATE TABLE IF NOT EXISTS tags (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE
);
CREATE INDEX IF NOT EXISTS idx_tags_name ON tags(name);

-- Many-to-many server ↔ tag membership.
CREATE TABLE IF NOT EXISTS server_tags (
  server_id TEXT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  tag_id TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  PRIMARY KEY (server_id, tag_id)
);
CREATE INDEX IF NOT EXISTS idx_server_tags_tag ON server_tags(tag_id);
