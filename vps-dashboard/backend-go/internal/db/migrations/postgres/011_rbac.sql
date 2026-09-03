-- PostgreSQL version of 011_rbac.sql
-- Phase 11 (Infrastructure Platform): RBAC 3-level (admin/operator/viewer).
--
-- Supabase-compatible: does NOT use SET session_replication_role,
-- which requires SUPERUSER (Supabase does not grant it to the postgres
-- role). Instead we drop and re-add the auto-named CHECK constraint
-- via plain ALTER TABLE, which any table owner can do.

-- Drop the old auto-named constraint. PostgreSQL names single-column
-- CHECK constraints as <table>_<column>_check, i.e. users_role_check.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;

-- Add the expanded constraint (admin/operator/viewer).
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('admin','operator','viewer'));
