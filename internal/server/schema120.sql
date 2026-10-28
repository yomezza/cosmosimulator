CREATE TABLE IF NOT EXISTS planet_settings (
 id int PRIMARY KEY CHECK(id=1), config jsonb NOT NULL
);
INSERT INTO planet_settings VALUES(1,'{"min_level":30,"personal_limit":5,"min_land":30,"counts":[50,100,160,200],"funding_days":30,"build_ticks":120,"refund_percent":90,"cost":{"credits":1000000,"iron":10000,"carbon":2000,"silicon":1500,"water":1000,"colony_core":1}}') ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS planet_surfaces (
 id bigserial PRIMARY KEY, star int NOT NULL, planet int NOT NULL, version int NOT NULL,
 seed text NOT NULL, name text NOT NULL, kind text NOT NULL, data jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(star,planet)
);
CREATE TABLE IF NOT EXISTS planet_sectors (
 surface bigint NOT NULL REFERENCES planet_surfaces, number int NOT NULL,
 data jsonb NOT NULL, PRIMARY KEY(surface,number)
);
CREATE TABLE IF NOT EXISTS planet_projects (
 id bigserial PRIMARY KEY,surface bigint NOT NULL,sector int NOT NULL,
 initiator bigint NOT NULL REFERENCES users, owner_user bigint REFERENCES users,owner_company bigint REFERENCES companies,
 status text NOT NULL DEFAULT 'funding' CHECK(status IN('funding','building','complete','cancelled','expired')),
 required jsonb NOT NULL,received jsonb NOT NULL DEFAULT '{}',build_ticks int NOT NULL CHECK(build_ticks>0),
 progress int NOT NULL DEFAULT 0 CHECK(progress>=0 AND progress<=build_ticks),last_tick bigint NOT NULL DEFAULT 0,
 refund_percent int NOT NULL CHECK(refund_percent BETWEEN 0 AND 100),
 expires_at timestamptz NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),finished_at timestamptz,
 CHECK((owner_user IS NULL)<>(owner_company IS NULL)),FOREIGN KEY(surface,sector) REFERENCES planet_sectors(surface,number)
);
CREATE UNIQUE INDEX IF NOT EXISTS planet_sector_claim ON planet_projects(surface,sector) WHERE status IN('funding','building','complete');
CREATE INDEX IF NOT EXISTS planet_active_projects ON planet_projects(status,expires_at,id) WHERE status IN('funding','building');
CREATE INDEX IF NOT EXISTS planet_user_projects ON planet_projects(owner_user,status);
CREATE INDEX IF NOT EXISTS planet_company_projects ON planet_projects(owner_company,status);
CREATE TABLE IF NOT EXISTS planet_contributions (
 id bigserial PRIMARY KEY, project bigint NOT NULL REFERENCES planet_projects,
 actor bigint NOT NULL REFERENCES users, admin_grant boolean NOT NULL DEFAULT false,
 resource text NOT NULL, amount bigint NOT NULL CHECK(amount>0),returned boolean NOT NULL DEFAULT false,
 source_star int NOT NULL DEFAULT 0,source_station text NOT NULL DEFAULT '',created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS planet_contribution_project ON planet_contributions(project,id);
CREATE TABLE IF NOT EXISTS planet_events (
 id bigserial PRIMARY KEY,project bigint NOT NULL REFERENCES planet_projects,actor bigint REFERENCES users,
 kind text NOT NULL,data jsonb NOT NULL DEFAULT '{}',created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS planet_event_project ON planet_events(project,id);
CREATE TABLE IF NOT EXISTS planet_requests (
 user_id bigint NOT NULL REFERENCES users,request text NOT NULL,digest text NOT NULL,result jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(user_id,request)
);
CREATE TABLE IF NOT EXISTS planet_refunds (
 id bigserial PRIMARY KEY,contribution bigint NOT NULL UNIQUE REFERENCES planet_contributions,
 user_id bigint NOT NULL REFERENCES users,resource text NOT NULL,amount bigint NOT NULL CHECK(amount>=0),
 star int NOT NULL,station text NOT NULL,claimed boolean NOT NULL DEFAULT false,created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS planet_refund_user ON planet_refunds(user_id,claimed);
CREATE TABLE IF NOT EXISTS planet_colonies (
 id bigserial PRIMARY KEY,project bigint NOT NULL UNIQUE REFERENCES planet_projects,surface bigint NOT NULL,sector int NOT NULL,
 owner_user bigint REFERENCES users,owner_company bigint REFERENCES companies,created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(surface,sector) REFERENCES planet_sectors(surface,number),UNIQUE(surface,sector),CHECK((owner_user IS NULL)<>(owner_company IS NULL))
);
CREATE TABLE IF NOT EXISTS planet_visits (
 user_id bigint PRIMARY KEY REFERENCES users, colony bigint NOT NULL REFERENCES planet_colonies,ship bigint NOT NULL REFERENCES ships,
 entered_at timestamptz NOT NULL DEFAULT now()
);
