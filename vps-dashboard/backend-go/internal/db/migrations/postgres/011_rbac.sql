-- PostgreSQL version of 011_rbac.sql
-- Phase 11 (Infrastructure Platform): RBAC 3-level.
--
-- SQLite version uses PRAGMA foreign_keys=OFF + table rebuild because
-- SQLite cannot ALTER a CHECK constraint in-place. PostgreSQL cannot
-- alter a CHECK constraint in-place either, so we use the same
-- backup/drop/recreate/restore pattern.
--
-- Instead of PRAGMA, we use SET session_replication_role = replica
-- to temporarily disable triggers and foreign key checks.

SET session_replication_role = replica;

-- Save existing data.
CREATE TABLE IF NOT EXISTS users_backup AS SELECT * FROM users;

-- Drop and recreate with the expanded CHECK constraint.
DROP TABLE IF EXISTS users;

CREATE TABLE users (
  id TEXT PRIMARY KEY,
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL CHECK(role IN ('admin','operator','viewer')),
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Restore data (existing 'admin' and 'viewer' roles are valid under
-- the new constraint; no data loss).
INSERT INTO users (id, username, password_hash, role, created_at, updated_at)
SELECT id, username, password_hash, role, created_at, updated_at
FROM users_backup;

DROP TABLE users_backup;

SET session_replication_role = DEFAULT;
