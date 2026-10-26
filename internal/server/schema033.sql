-- Additive, transactional migration from 0.3.2. IDs and assets survive upgrades.
ALTER TABLE ships ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE ships ADD COLUMN IF NOT EXISTS extractor TEXT NOT NULL DEFAULT 'universal';
ALTER TABLE ships ADD COLUMN IF NOT EXISTS reserved_fuel INTEGER NOT NULL DEFAULT 0;
ALTER TABLE ships ADD COLUMN IF NOT EXISTS waypoints JSONB NOT NULL DEFAULT '[]';
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='ships'::regclass AND contype='p' AND pg_get_constraintdef(oid)='PRIMARY KEY (user_id)') THEN
  ALTER TABLE ships DROP CONSTRAINT ships_pkey;
  ALTER TABLE ships ADD PRIMARY KEY(id);
 END IF;
END $$;
UPDATE ships s SET active=TRUE WHERE id=(SELECT min(id) FROM ships t WHERE t.user_id=s.user_id) AND NOT EXISTS(SELECT 1 FROM ships t WHERE t.user_id=s.user_id AND t.active);
CREATE UNIQUE INDEX IF NOT EXISTS one_active_ship ON ships(user_id) WHERE active;
ALTER TABLE mining_jobs ADD COLUMN IF NOT EXISTS ship_id BIGINT REFERENCES ships(id) ON DELETE CASCADE;
UPDATE mining_jobs j SET ship_id=s.id FROM ships s WHERE s.user_id=j.user_id AND s.active AND j.ship_id IS NULL;
ALTER TABLE mining_jobs ALTER COLUMN ship_id SET NOT NULL;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='mining_jobs'::regclass AND contype='p' AND pg_get_constraintdef(oid)='PRIMARY KEY (user_id)') THEN
  ALTER TABLE mining_jobs DROP CONSTRAINT mining_jobs_pkey;
  ALTER TABLE mining_jobs ADD PRIMARY KEY(ship_id);
 END IF;
END $$;
ALTER TABLE game_settings ADD COLUMN IF NOT EXISTS scan_seconds INTEGER NOT NULL DEFAULT 1 CHECK(scan_seconds BETWEEN 1 AND 3600);
ALTER TABLE game_settings ADD COLUMN IF NOT EXISTS company_cost BIGINT NOT NULL DEFAULT 1000000 CHECK(company_cost BETWEEN 0 AND 1000000000);
ALTER TABLE game_settings ADD COLUMN IF NOT EXISTS rescue_cost INTEGER NOT NULL DEFAULT 100 CHECK(rescue_cost>=0);
ALTER TABLE game_settings ADD COLUMN IF NOT EXISTS reservation_hours INTEGER NOT NULL DEFAULT 72 CHECK(reservation_hours BETWEEN 1 AND 720);
ALTER TABLE companies ADD COLUMN IF NOT EXISTS founder BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE companies ADD COLUMN IF NOT EXISTS founder_key TEXT UNIQUE;
ALTER TABLE companies ADD COLUMN IF NOT EXISTS credits BIGINT NOT NULL DEFAULT 0 CHECK(credits>=0);
ALTER TABLE companies ADD COLUMN IF NOT EXISTS flag_design JSONB NOT NULL DEFAULT '{}';
INSERT INTO companies(name,tag,color,flag,founder,founder_key) VALUES
 ('Корпорация-основатель 01','F01','#70bccc','◆',TRUE,'founder-01'),
 ('Корпорация-основатель 02','F02','#b19ed5','✦',TRUE,'founder-02') ON CONFLICT DO NOTHING;
UPDATE companies SET founder_key=CASE WHEN tag='F01' THEN 'founder-01' ELSE 'founder-02' END WHERE founder AND founder_key IS NULL AND tag IN('F01','F02');
-- A player's pre-existing name/tag must never be renamed or appropriated.
DO $$ DECLARE ordinal INTEGER; stable_key TEXT; display_name TEXT; candidate_tag TEXT; suffix INTEGER; BEGIN
 FOR ordinal IN 1..2 LOOP
  stable_key:='founder-0'||ordinal;
  IF NOT EXISTS(SELECT 1 FROM companies WHERE founder_key=stable_key) THEN
   candidate_tag:='F0'||ordinal;suffix:=0;
   WHILE EXISTS(SELECT 1 FROM companies WHERE tag=candidate_tag) LOOP
    suffix:=suffix+1;
    IF suffix>65535 THEN RAISE EXCEPTION 'No free temporary founder tag'; END IF;
    candidate_tag:=upper(lpad(to_hex(suffix),4,'0'));
   END LOOP;
   display_name:='Корпорация-основатель 0'||ordinal;suffix:=1;
   WHILE EXISTS(SELECT 1 FROM companies WHERE name=display_name) LOOP
    suffix:=suffix+1;display_name:='Корпорация-основатель 0'||ordinal||' · '||suffix;
   END LOOP;
   INSERT INTO companies(name,tag,color,flag,founder,founder_key) VALUES(display_name,candidate_tag,CASE WHEN ordinal=1 THEN '#70bccc' ELSE '#b19ed5' END,CASE WHEN ordinal=1 THEN '◆' ELSE '✦' END,TRUE,stable_key);
  END IF;
 END LOOP;
END $$;
CREATE TABLE IF NOT EXISTS company_roles(id BIGSERIAL PRIMARY KEY,company_id BIGINT NOT NULL REFERENCES companies ON DELETE CASCADE,name TEXT NOT NULL,permissions JSONB NOT NULL DEFAULT '[]',UNIQUE(company_id,name));
ALTER TABLE users ADD COLUMN IF NOT EXISTS company_role BIGINT REFERENCES company_roles ON DELETE SET NULL;
ALTER TABLE users ADD COLUMN IF NOT EXISTS last_active TIMESTAMPTZ NOT NULL DEFAULT '2000-01-01';
ALTER TABLE users ADD COLUMN IF NOT EXISTS virtual_run BIGINT;
CREATE TABLE IF NOT EXISTS company_requests(id BIGSERIAL PRIMARY KEY,company_id BIGINT NOT NULL REFERENCES companies ON DELETE CASCADE,user_id BIGINT NOT NULL REFERENCES users ON DELETE CASCADE,kind TEXT NOT NULL CHECK(kind IN('application','invitation')),UNIQUE(company_id,user_id,kind));
CREATE TABLE IF NOT EXISTS company_finance(id BIGSERIAL PRIMARY KEY,company_id BIGINT NOT NULL REFERENCES companies ON DELETE CASCADE,actor BIGINT REFERENCES users ON DELETE SET NULL,amount BIGINT NOT NULL,reason TEXT NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS reputation(user_id BIGINT REFERENCES users ON DELETE CASCADE,company_id BIGINT REFERENCES companies ON DELETE CASCADE,points INTEGER NOT NULL DEFAULT 0,PRIMARY KEY(user_id,company_id));
CREATE TABLE IF NOT EXISTS financial_ledger(id BIGSERIAL PRIMARY KEY,user_id BIGINT NOT NULL REFERENCES users ON DELETE CASCADE,amount BIGINT NOT NULL,balance BIGINT NOT NULL,reason TEXT NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE OR REPLACE FUNCTION ledger_credit_change() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.credits<>OLD.credits THEN INSERT INTO financial_ledger(user_id,amount,balance,reason) VALUES(NEW.id,NEW.credits-OLD.credits,NEW.credits,COALESCE(NULLIF(current_setting('cosmos.reason',true),''),'Игровая операция')); END IF; RETURN NEW; END $$;
DROP TRIGGER IF EXISTS credits_ledger ON users;
CREATE TRIGGER credits_ledger AFTER UPDATE OF credits ON users FOR EACH ROW EXECUTE FUNCTION ledger_credit_change();
CREATE TABLE IF NOT EXISTS notifications(id BIGSERIAL PRIMARY KEY,user_id BIGINT NOT NULL REFERENCES users ON DELETE CASCADE,kind TEXT NOT NULL,level TEXT NOT NULL DEFAULT 'success',message TEXT NOT NULL DEFAULT '',data JSONB NOT NULL DEFAULT '{}',seen BOOLEAN NOT NULL DEFAULT FALSE,created_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE INDEX IF NOT EXISTS notifications_user ON notifications(user_id,id DESC);
CREATE OR REPLACE FUNCTION event_notification() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.kind IN('arrival','mining_complete','research_complete','delivery_complete','company_invitation','project_complete','portal_open') THEN INSERT INTO notifications(user_id,kind,data) VALUES(NEW.user_id,NEW.kind,NEW.data); END IF; RETURN NEW; END $$;
DROP TRIGGER IF EXISTS event_notifications ON game_events;
CREATE TRIGGER event_notifications AFTER INSERT ON game_events FOR EACH ROW EXECUTE FUNCTION event_notification();
CREATE TABLE IF NOT EXISTS research_jobs(ship_id BIGINT PRIMARY KEY REFERENCES ships ON DELETE CASCADE,user_id BIGINT NOT NULL REFERENCES users ON DELETE CASCADE,star INTEGER NOT NULL,scanned INTEGER NOT NULL DEFAULT 0,total INTEGER NOT NULL,next_at TIMESTAMPTZ NOT NULL,paused BOOLEAN NOT NULL DEFAULT FALSE,seconds INTEGER NOT NULL,UNIQUE(user_id,star));
CREATE TABLE IF NOT EXISTS navigation_notes(user_id BIGINT REFERENCES users ON DELETE CASCADE,star INTEGER NOT NULL,object_id TEXT NOT NULL DEFAULT '',note TEXT NOT NULL DEFAULT '',bookmark BOOLEAN NOT NULL DEFAULT FALSE,PRIMARY KEY(user_id,star,object_id));
CREATE TABLE IF NOT EXISTS survey_offers(id BIGSERIAL PRIMARY KEY,seller BIGINT NOT NULL REFERENCES users ON DELETE CASCADE,star INTEGER NOT NULL,station TEXT NOT NULL,region INTEGER NOT NULL,systems INTEGER[] NOT NULL,price BIGINT NOT NULL CHECK(price>0),created_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS stations(star INTEGER NOT NULL,object_id TEXT NOT NULL,name TEXT NOT NULL,specialization TEXT NOT NULL,founder BIGINT REFERENCES companies,shipyard BOOLEAN NOT NULL DEFAULT FALSE,ship_stock INTEGER NOT NULL DEFAULT 0,PRIMARY KEY(star,object_id));
CREATE TABLE IF NOT EXISTS warehouses(user_id BIGINT REFERENCES users ON DELETE CASCADE,star INTEGER NOT NULL,station TEXT NOT NULL,cargo JSONB NOT NULL DEFAULT '{}',PRIMARY KEY(user_id,star,station));
CREATE TABLE IF NOT EXISTS sell_orders(id BIGSERIAL PRIMARY KEY,seller BIGINT NOT NULL REFERENCES users ON DELETE CASCADE,star INTEGER NOT NULL,station TEXT NOT NULL,resource TEXT NOT NULL,quantity INTEGER NOT NULL CHECK(quantity>=0),price BIGINT NOT NULL CHECK(price>0),created_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS deliveries(id BIGSERIAL PRIMARY KEY,owner BIGINT NOT NULL REFERENCES users ON DELETE CASCADE,carrier BIGINT REFERENCES users ON DELETE SET NULL,ship_id BIGINT REFERENCES ships ON DELETE SET NULL,from_star INTEGER NOT NULL,from_station TEXT NOT NULL,to_star INTEGER NOT NULL,to_station TEXT NOT NULL,resource TEXT NOT NULL,quantity INTEGER NOT NULL CHECK(quantity>0),reward BIGINT NOT NULL CHECK(reward>=0),collateral BIGINT NOT NULL DEFAULT 0 CHECK(collateral>=0),status TEXT NOT NULL DEFAULT 'open',deadline TIMESTAMPTZ NOT NULL,arrival TIMESTAMPTZ,created_at TIMESTAMPTZ NOT NULL DEFAULT now());
ALTER TABLE reservations ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;
UPDATE reservations SET expires_at=created_at+interval '72 hours' WHERE expires_at IS NULL;
ALTER TABLE reservations ALTER COLUMN expires_at SET DEFAULT (now()+interval '72 hours');
CREATE TABLE IF NOT EXISTS company_projects(id BIGSERIAL PRIMARY KEY,company_id BIGINT NOT NULL REFERENCES companies ON DELETE CASCADE,name TEXT NOT NULL,kind TEXT NOT NULL,star INTEGER NOT NULL,required JSONB NOT NULL,contributed JSONB NOT NULL DEFAULT '{}',status TEXT NOT NULL DEFAULT 'active');
CREATE TABLE IF NOT EXISTS project_contributions(id BIGSERIAL PRIMARY KEY,project_id BIGINT NOT NULL REFERENCES company_projects ON DELETE CASCADE,user_id BIGINT REFERENCES users ON DELETE SET NULL,resource TEXT NOT NULL,quantity INTEGER NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS portal(id INTEGER PRIMARY KEY CHECK(id=1),required BIGINT NOT NULL DEFAULT 1000000,contributed BIGINT NOT NULL DEFAULT 0,active BOOLEAN NOT NULL DEFAULT FALSE);
INSERT INTO portal(id) VALUES(1) ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS portal_contributions(id BIGSERIAL PRIMARY KEY,user_id BIGINT REFERENCES users ON DELETE SET NULL,amount BIGINT NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS benchmark_runs(id BIGSERIAL PRIMARY KEY,actor BIGINT REFERENCES users ON DELETE SET NULL,players INTEGER NOT NULL,seconds INTEGER NOT NULL,scenario TEXT NOT NULL,status TEXT NOT NULL DEFAULT 'running',requests BIGINT NOT NULL DEFAULT 0,errors BIGINT NOT NULL DEFAULT 0,metrics JSONB NOT NULL DEFAULT '{}',started_at TIMESTAMPTZ NOT NULL DEFAULT now(),finished_at TIMESTAMPTZ);
UPDATE benchmark_runs SET status='interrupted',finished_at=now() WHERE status='running';
ALTER TABLE companies ADD COLUMN IF NOT EXISTS stock JSONB NOT NULL DEFAULT '{}';
ALTER TABLE stations ADD COLUMN IF NOT EXISTS company_id BIGINT REFERENCES companies ON DELETE SET NULL;
CREATE UNIQUE INDEX IF NOT EXISTS one_running_benchmark ON benchmark_runs((status)) WHERE status='running';
ALTER TABLE surveys ADD COLUMN IF NOT EXISTS received_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE surveys ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'legacy';
ALTER TABLE company_projects ADD COLUMN IF NOT EXISTS station TEXT NOT NULL DEFAULT 'station';
CREATE TABLE IF NOT EXISTS company_storage(company_id BIGINT REFERENCES companies ON DELETE CASCADE,star INTEGER NOT NULL,station TEXT NOT NULL,cargo JSONB NOT NULL DEFAULT '{}',PRIMARY KEY(company_id,star,station));
