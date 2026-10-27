-- Archive unused generated ports without deleting their stock or history.
ALTER TABLE stations ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT TRUE;
CREATE INDEX IF NOT EXISTS stations_active_star ON stations(star) WHERE active;
CREATE INDEX IF NOT EXISTS admin_log_created_at ON admin_log(created_at DESC,id DESC);
