-- PostgreSQL version of 002_seed_meta.sql
-- Note: SQLite "INSERT OR IGNORE" → PostgreSQL "ON CONFLICT DO NOTHING"

CREATE TABLE IF NOT EXISTS app_meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

INSERT INTO app_meta(key, value) VALUES
  ('schema_version_app', '1'),
  ('installed_at', to_char(NOW(), 'YYYY-MM-DD"T"HH24:MI:SS'))
ON CONFLICT (key) DO NOTHING;
