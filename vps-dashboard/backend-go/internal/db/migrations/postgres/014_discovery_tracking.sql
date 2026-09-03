-- PostgreSQL version of 014_discovery_tracking.sql
ALTER TABLE server_discoveries ADD COLUMN IF NOT EXISTS first_seen TEXT NOT NULL DEFAULT '';
ALTER TABLE server_discoveries ADD COLUMN IF NOT EXISTS last_status TEXT NOT NULL DEFAULT 'active';
