-- P2-6: stable public character identity; no geography changes.
ALTER TABLE planet_characters ADD COLUMN IF NOT EXISTS public_id uuid NOT NULL DEFAULT gen_random_uuid();
CREATE UNIQUE INDEX IF NOT EXISTS planet_character_public_id ON planet_characters(public_id);
CREATE OR REPLACE VIEW planet_session_lookup AS
 SELECT s.token_hash,s.user_id,s.expires_at,u.is_bot,u.username AS nickname
 FROM sessions s JOIN users u ON u.id=s.user_id;
UPDATE planet_schema_state SET version=6 WHERE id=1 AND version=5;
