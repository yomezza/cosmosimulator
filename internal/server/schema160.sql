-- P2-2: cosmic-owned territory work and persistent initial landing sites.
CREATE UNIQUE INDEX IF NOT EXISTS planet_colony_location_key ON planet_colonies(id,surface,sector);
CREATE TABLE IF NOT EXISTS planet_runtime_controls (
 surface bigint PRIMARY KEY REFERENCES planet_surfaces,
 geography_revision bigint NOT NULL CHECK(geography_revision>=0),
 claims_epoch bigint NOT NULL DEFAULT 1 CHECK(claims_epoch>0),
 processed_epoch bigint NOT NULL DEFAULT 0 CHECK(processed_epoch>=0 AND processed_epoch<=claims_epoch),
 layout_epoch bigint NOT NULL DEFAULT 0,
 policy_version int NOT NULL DEFAULT 1,
 state text NOT NULL DEFAULT 'enabled' CHECK(state IN('enabled','retiring')),
 components jsonb NOT NULL DEFAULT '[]' CHECK(jsonb_typeof(components)='array'),
 issues jsonb NOT NULL DEFAULT '[]' CHECK(jsonb_typeof(issues)='array'),
 retry_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS planet_territory_dirty ON planet_runtime_controls(retry_at,surface)
 WHERE state='enabled' AND (claims_epoch>processed_epoch OR issues<>'[]'::jsonb);
CREATE TABLE IF NOT EXISTS planet_landing_sites (
 id bigserial PRIMARY KEY,
 colony bigint NOT NULL UNIQUE,
 surface bigint NOT NULL,
 sector int NOT NULL,
 geography_revision bigint NOT NULL CHECK(geography_revision>=0),
 layout_version int NOT NULL DEFAULT 1 CHECK(layout_version>0),
 x_q int NOT NULL CHECK(x_q BETWEEN 0 AND 262144),
 y_q int NOT NULL CHECK(y_q BETWEEN 0 AND 262144),
 spawn_x_q int NOT NULL CHECK(spawn_x_q BETWEEN 0 AND 262144),
 spawn_y_q int NOT NULL CHECK(spawn_y_q BETWEEN 0 AND 262144),
 enabled boolean NOT NULL DEFAULT true,
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(colony,surface,sector) REFERENCES planet_colonies(id,surface,sector),
 FOREIGN KEY(surface,sector) REFERENCES planet_sectors(surface,number)
);
CREATE INDEX IF NOT EXISTS planet_landing_surface ON planet_landing_sites(surface,sector);
CREATE OR REPLACE FUNCTION planet_territory_dirty() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE sid bigint;
BEGIN
 IF TG_OP='DELETE' THEN sid:=OLD.surface; ELSE sid:=NEW.surface; END IF;
 IF TG_OP='UPDATE' AND (OLD.surface<>NEW.surface OR OLD.sector<>NEW.sector) THEN
  RAISE EXCEPTION 'colony location is immutable';
 END IF;
 EXECUTE format('INSERT INTO %1$I.planet_runtime_controls AS existing(surface,geography_revision)
   SELECT s.id,COALESCE(c.revision,0) FROM %1$I.planet_surfaces s
   LEFT JOIN %1$I.planet_surface_controls c ON c.star=s.star AND c.planet=s.planet WHERE s.id=$1
   ON CONFLICT(surface) DO UPDATE SET claims_epoch=existing.claims_epoch+1,
   retry_at=now() WHERE existing.state=''enabled''',TG_TABLE_SCHEMA) USING sid;
 IF TG_OP='DELETE' THEN RETURN OLD; ELSE RETURN NEW; END IF;
END $$;
DROP TRIGGER IF EXISTS planet_colony_dirty ON planet_colonies;
CREATE TRIGGER planet_colony_dirty AFTER INSERT OR UPDATE OR DELETE ON planet_colonies
 FOR EACH ROW EXECUTE FUNCTION planet_territory_dirty();
-- Repeatable backfill: queue each old surface once, not a module per old colony.
INSERT INTO planet_runtime_controls(surface,geography_revision)
 SELECT DISTINCT s.id,COALESCE(c.revision,0) FROM planet_colonies p
 JOIN planet_surfaces s ON s.id=p.surface
 LEFT JOIN planet_surface_controls c ON c.star=s.star AND c.planet=s.planet
 ON CONFLICT DO NOTHING;
UPDATE planet_schema_state SET version=2 WHERE id=1 AND version=1;
