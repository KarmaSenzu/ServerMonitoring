CREATE TABLE IF NOT EXISTS app_meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

INSERT OR IGNORE INTO app_meta(key, value) VALUES
  ('schema_version_app', '1'),
  ('installed_at', datetime('now'));
