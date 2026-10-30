ALTER TABLE player_ideas ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected'));
