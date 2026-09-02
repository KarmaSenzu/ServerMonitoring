-- Add credential_password column for direct password storage
-- (used when credential_type = 'password')
-- Note: SQLite does NOT support "IF NOT EXISTS" in ALTER TABLE ADD COLUMN.
-- If this migration was already applied (column exists), it will error
-- but the migration system skips already-applied versions.
ALTER TABLE servers ADD COLUMN credential_password TEXT NOT NULL DEFAULT '';
