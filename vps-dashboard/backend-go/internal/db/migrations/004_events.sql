CREATE TABLE IF NOT EXISTS events (
  id TEXT PRIMARY KEY,
  ts TEXT NOT NULL DEFAULT (datetime('now')),
  category TEXT NOT NULL,
  severity TEXT NOT NULL CHECK(severity IN ('info','warning','error','critical')) DEFAULT 'info',
  source TEXT NOT NULL DEFAULT '',
  project_id TEXT NOT NULL DEFAULT '',
  message TEXT NOT NULL,
  data_json TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_events_ts ON events(ts);
CREATE INDEX IF NOT EXISTS idx_events_category ON events(category);
CREATE INDEX IF NOT EXISTS idx_events_severity ON events(severity);
CREATE INDEX IF NOT EXISTS idx_events_project ON events(project_id);

CREATE TABLE IF NOT EXISTS notification_channels (
  id TEXT PRIMARY KEY,
  type TEXT NOT NULL CHECK(type IN ('telegram')) DEFAULT 'telegram',
  name TEXT NOT NULL UNIQUE,
  enabled INTEGER NOT NULL DEFAULT 1,
  config_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_channels_enabled ON notification_channels(enabled);

CREATE TABLE IF NOT EXISTS alert_rules (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  enabled INTEGER NOT NULL DEFAULT 1,
  type TEXT NOT NULL,
  threshold REAL NOT NULL DEFAULT 0,
  comparator TEXT NOT NULL DEFAULT 'gte',
  for_seconds INTEGER NOT NULL DEFAULT 60,
  cooldown_seconds INTEGER NOT NULL DEFAULT 600,
  severity TEXT NOT NULL CHECK(severity IN ('info','warning','error','critical')) DEFAULT 'warning',
  channels_json TEXT NOT NULL DEFAULT '[]',
  scope_json TEXT NOT NULL DEFAULT '{}',
  last_triggered_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
