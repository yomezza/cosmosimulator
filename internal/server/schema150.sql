-- P2-1 foundation. Only the cosmic migration runner applies this DDL.
CREATE TABLE IF NOT EXISTS planet_schema_state (
 id integer PRIMARY KEY CHECK (id=1),
 version integer NOT NULL CHECK (version>0)
);
CREATE TABLE IF NOT EXISTS planet_runtime_leases (
 id integer PRIMARY KEY CHECK (id=1),
 owner_id uuid,
 fence bigint NOT NULL DEFAULT 0 CHECK (fence>=0),
 expires_at timestamptz NOT NULL DEFAULT '-infinity'
);
INSERT INTO planet_runtime_leases(id) VALUES (1) ON CONFLICT DO NOTHING;
INSERT INTO planet_schema_state(id,version) VALUES (1,1) ON CONFLICT DO NOTHING;
