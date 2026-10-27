-- Monetary values remain BIGINT; one consistent ceiling for each wallet.
CREATE OR REPLACE FUNCTION enforce_credit_ceiling() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.credits<0 OR (NEW.credits>500000000000 AND (TG_OP='INSERT' OR NEW.credits>OLD.credits)) THEN
  RAISE EXCEPTION 'Достигнут лимит 500 млрд CR' USING ERRCODE='23514',CONSTRAINT='credits_ceiling';
 END IF; RETURN NEW; END $$;
DROP TRIGGER IF EXISTS credits_ceiling ON users;
CREATE TRIGGER credits_ceiling BEFORE INSERT OR UPDATE OF credits ON users FOR EACH ROW EXECUTE FUNCTION enforce_credit_ceiling();
DROP TRIGGER IF EXISTS credits_ceiling ON companies;
CREATE TRIGGER credits_ceiling BEFORE INSERT OR UPDATE OF credits ON companies FOR EACH ROW EXECUTE FUNCTION enforce_credit_ceiling();
DROP TRIGGER IF EXISTS credits_ceiling ON local_markets;
CREATE TRIGGER credits_ceiling BEFORE INSERT OR UPDATE OF credits ON local_markets FOR EACH ROW EXECUTE FUNCTION enforce_credit_ceiling();
CREATE TABLE IF NOT EXISTS automation_tasks (
 id BIGSERIAL PRIMARY KEY,user_id BIGINT NOT NULL REFERENCES users ON DELETE CASCADE,
 ship_id BIGINT NOT NULL REFERENCES ships ON DELETE CASCADE,kind TEXT NOT NULL CHECK(kind IN('mine','haul')),
 source_star INTEGER NOT NULL,source_object TEXT NOT NULL,dest_star INTEGER NOT NULL,dest_station TEXT NOT NULL DEFAULT '',
 resource TEXT NOT NULL,quantity INTEGER NOT NULL CHECK(quantity BETWEEN 1 AND 100000),progress INTEGER NOT NULL DEFAULT 0,
 carried INTEGER NOT NULL DEFAULT 0,pending INTEGER NOT NULL DEFAULT 0,phase TEXT NOT NULL DEFAULT 'source',
 status TEXT NOT NULL DEFAULT 'queued' CHECK(status IN('queued','running','paused','completed','cancelled')),
 error TEXT NOT NULL DEFAULT '',created_at TIMESTAMPTZ NOT NULL DEFAULT now(),updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS automation_ship_queue ON automation_tasks(ship_id,id) WHERE status IN('queued','running','paused');
ALTER TABLE mining_jobs ADD COLUMN IF NOT EXISTS automation_id BIGINT REFERENCES automation_tasks(id) ON DELETE SET NULL;
