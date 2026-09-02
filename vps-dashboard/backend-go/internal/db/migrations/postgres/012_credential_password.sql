-- PostgreSQL version of 012_credential_password.sql
ALTER TABLE servers ADD COLUMN IF NOT EXISTS credential_password TEXT NOT NULL DEFAULT '';
