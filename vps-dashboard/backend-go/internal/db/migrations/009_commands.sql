-- Phase 6 (Infrastructure Platform): Multi-Host Command Engine.
--
-- Stores reusable command snippets and per-host execution audit
-- records (PROJECT ARCHITECTURE.md §17, §20).

CREATE TABLE IF NOT EXISTS command_snippets (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  description TEXT NOT NULL DEFAULT '',
  command TEXT NOT NULL,
  variables_json TEXT NOT NULL DEFAULT '[]',  -- ["container", "service"]
  danger_level TEXT NOT NULL CHECK(danger_level IN ('safe','caution','dangerous')) DEFAULT 'safe',
  created_by TEXT NOT NULL DEFAULT '',
  updated_by TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_snippets_name ON command_snippets(name);
CREATE INDEX IF NOT EXISTS idx_snippets_danger ON command_snippets(danger_level);

-- Per-host execution audit trail (§17 Command Run entity).
CREATE TABLE IF NOT EXISTS command_runs (
  id TEXT PRIMARY KEY,
  snippet_id TEXT NOT NULL DEFAULT '',      -- empty for ad-hoc commands
  server_id TEXT NOT NULL,
  server_name TEXT NOT NULL,                -- denormalised for history
  user_id TEXT NOT NULL DEFAULT '',
  command TEXT NOT NULL,
  exit_code INTEGER NOT NULL DEFAULT 0,
  stdout TEXT NOT NULL DEFAULT '',
  stderr TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL CHECK(status IN ('running','success','failed','timeout','error')) DEFAULT 'running',
  started_at TEXT NOT NULL DEFAULT (datetime('now')),
  finished_at TEXT NOT NULL DEFAULT '',
  duration_ms INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_runs_server ON command_runs(server_id);
CREATE INDEX IF NOT EXISTS idx_runs_started ON command_runs(started_at);
CREATE INDEX IF NOT EXISTS idx_runs_snippet ON command_runs(snippet_id);
CREATE INDEX IF NOT EXISTS idx_runs_status ON command_runs(status);
