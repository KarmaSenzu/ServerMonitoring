-- Add per-item discovery tracking: first_seen, last_seen, status
-- This allows tracking when services appear/disappear over time
-- instead of replacing the entire discovery blob each poll.
ALTER TABLE server_discoveries ADD COLUMN first_seen TEXT NOT NULL DEFAULT '';
ALTER TABLE server_discoveries ADD COLUMN last_status TEXT NOT NULL DEFAULT 'active';
