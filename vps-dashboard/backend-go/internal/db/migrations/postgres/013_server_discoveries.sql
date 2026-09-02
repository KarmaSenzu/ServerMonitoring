-- PostgreSQL version of 013_server_discoveries.sql
CREATE TABLE IF NOT EXISTS server_discoveries (
  id TEXT PRIMARY KEY,
  server_id TEXT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  pm2_json TEXT NOT NULL DEFAULT '[]',
  docker_json TEXT NOT NULL DEFAULT '[]',
  tunnels_json TEXT NOT NULL DEFAULT '[]',
  systemd_json TEXT NOT NULL DEFAULT '[]',
  ports_json TEXT NOT NULL DEFAULT '[]',
  hostname TEXT NOT NULL DEFAULT '',
  kernel TEXT NOT NULL DEFAULT '',
  os_name TEXT NOT NULL DEFAULT '',
  discovered_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_discoveries_server ON server_discoveries(server_id);
