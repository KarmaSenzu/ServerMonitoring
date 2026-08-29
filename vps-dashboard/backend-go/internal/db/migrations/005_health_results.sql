CREATE TABLE IF NOT EXISTS health_results (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id TEXT NOT NULL,
  ts TEXT NOT NULL DEFAULT (datetime('now')),
  ok INTEGER NOT NULL,
  status_code INTEGER NOT NULL DEFAULT 0,
  latency_ms INTEGER NOT NULL DEFAULT -1,
  error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_health_project_ts ON health_results(project_id, ts);
