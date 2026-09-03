-- Add cloudflare_json column to server_discoveries (Cloudflare tunnel detection)
ALTER TABLE server_discoveries ADD COLUMN IF NOT EXISTS cloudflare_json TEXT NOT NULL DEFAULT '[]';
