-- Add cloudflare_json column to server_discoveries (Cloudflare tunnel detection)
ALTER TABLE server_discoveries ADD COLUMN cloudflare_json TEXT NOT NULL DEFAULT '[]';
