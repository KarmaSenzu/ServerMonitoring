-- PostgreSQL version of 006_deploy_backup.sql

-- Add deploy + environment columns to existing projects table.
ALTER TABLE projects ADD COLUMN IF NOT EXISTS environment TEXT NOT NULL DEFAULT 'production';
ALTER TABLE projects ADD COLUMN IF NOT EXISTS webhook_secret TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN IF NOT EXISTS deploy_command TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN IF NOT EXISTS deploy_timeout_seconds INTEGER NOT NULL DEFAULT 300;
ALTER TABLE projects ADD COLUMN IF NOT EXISTS deploy_working_dir TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN IF NOT EXISTS deploy_enabled INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_projects_environment ON projects(environment);

-- Per-deployment audit trail.
CREATE TABLE IF NOT EXISTS deployments (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL,
  triggered_by TEXT NOT NULL,
  triggered_at TIMESTAMP NOT NULL DEFAULT NOW(),
  finished_at TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'pending',
  exit_code INTEGER NOT NULL DEFAULT -1,
  stdout TEXT NOT NULL DEFAULT '',
  stderr TEXT NOT NULL DEFAULT '',
  remote_ref TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_deployments_project ON deployments(project_id, triggered_at);
CREATE INDEX IF NOT EXISTS idx_deployments_status ON deployments(status);

-- Backup history.
CREATE TABLE IF NOT EXISTS backups (
  id TEXT PRIMARY KEY,
  ts TIMESTAMP NOT NULL DEFAULT NOW(),
  path TEXT NOT NULL,
  size_bytes INTEGER NOT NULL DEFAULT 0,
  ok INTEGER NOT NULL DEFAULT 0,
  error TEXT NOT NULL DEFAULT '',
  trigger TEXT NOT NULL DEFAULT 'scheduled'
);
CREATE INDEX IF NOT EXISTS idx_backups_ts ON backups(ts);

-- Per-environment overrides.
CREATE TABLE IF NOT EXISTS environment_overrides (
  environment TEXT PRIMARY KEY,
  config_json TEXT NOT NULL DEFAULT '{}',
  updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
INSERT INTO environment_overrides(environment, config_json) VALUES
  ('development', '{"healthcheck_multiplier":3.0,"alert_severity_floor":"info"}'),
  ('staging',     '{"healthcheck_multiplier":1.5,"alert_severity_floor":"warning"}'),
  ('production',  '{"healthcheck_multiplier":1.0,"alert_severity_floor":"info"}')
ON CONFLICT (environment) DO NOTHING;
