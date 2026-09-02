-- PostgreSQL version of 003_projects.sql

CREATE TABLE IF NOT EXISTS projects (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  description TEXT NOT NULL DEFAULT '',
  domain TEXT NOT NULL DEFAULT '',
  port INTEGER NOT NULL DEFAULT 0,
  container_name TEXT NOT NULL DEFAULT '',
  pm2_name TEXT NOT NULL DEFAULT '',
  tunnel_service TEXT NOT NULL DEFAULT '',
  health_url TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  tags TEXT NOT NULL DEFAULT '',
  notes TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_projects_name ON projects(name);
CREATE INDEX IF NOT EXISTS idx_projects_domain ON projects(domain);
CREATE INDEX IF NOT EXISTS idx_projects_container ON projects(container_name);
CREATE INDEX IF NOT EXISTS idx_projects_pm2 ON projects(pm2_name);
CREATE INDEX IF NOT EXISTS idx_projects_tunnel ON projects(tunnel_service);
