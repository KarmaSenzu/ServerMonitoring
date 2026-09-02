-- Add credential_password column for direct password storage
-- (encrypted at rest, used when credential_type = 'password')
ALTER TABLE servers ADD COLUMN IF NOT EXISTS credential_password TEXT NOT NULL DEFAULT '';
