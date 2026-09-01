-- Phase 11 (Infrastructure Platform): RBAC 3-level.
--
-- Expands the users.role CHECK constraint to include 'operator'
-- (PROJECT ARCHITECTURE.md §32).
--
-- SQLite cannot ALTER a CHECK constraint in-place, so we rebuild
-- the users table. The migrate framework wraps each file in a
-- transaction already, so we do not need explicit BEGIN/COMMIT.

PRAGMA foreign_keys=OFF;

-- Save existing data.
CREATE TABLE IF NOT EXISTS users_backup AS SELECT * FROM users;

-- Drop and recreate with the expanded CHECK.
DROP TABLE IF EXISTS users;

CREATE TABLE users (
  id TEXT PRIMARY KEY,
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL CHECK(role IN ('admin','operator','viewer')),
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Restore data (existing 'admin' and 'viewer' roles are valid under
-- the new constraint; no data loss).
INSERT INTO users (id, username, password_hash, role, created_at, updated_at)
SELECT id, username, password_hash, role, created_at, updated_at
FROM users_backup;

DROP TABLE users_backup;

PRAGMA foreign_keys=ON;
