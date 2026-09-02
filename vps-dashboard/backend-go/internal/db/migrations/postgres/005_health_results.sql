-- PostgreSQL version of 005_health_results.sql
-- Note: SQLite "INTEGER PRIMARY KEY AUTOINCREMENT" → PostgreSQL "SERIAL"

CREATE TABLE IF NOT EXISTS health_results (
  id SERIAL PRIMARY KEY,
  project_id TEXT NOT NULL,
  ts TIMESTAMP NOT NULL DEFAULT NOW(),
  ok INTEGER NOT NULL,
  status_code INTEGER NOT NULL DEFAULT 0,
  latency_ms INTEGER NOT NULL DEFAULT -1,
  error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_health_project_ts ON health_results(project_id, ts);
