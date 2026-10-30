-- P2-3: derived immutable local scenery, written only by the planetary service.
-- One pinned generator per source revision. Future regeneration needs an explicit
-- migration, never an INSERT with a newer generator next to an old active scene.
CREATE TABLE IF NOT EXISTS planet_local_locations (
 surface bigint NOT NULL,
 sector int NOT NULL,
 revision bigint NOT NULL CHECK(revision>=0),
 generator_version int NOT NULL CHECK(generator_version>0),
 format_version int NOT NULL CHECK(format_version>0),
 walk_policy_version int NOT NULL CHECK(walk_policy_version>0),
 location_id text NOT NULL UNIQUE CHECK(length(location_id)=32),
 source_checksum text NOT NULL CHECK(length(source_checksum)=64),
 decor_seed text NOT NULL CHECK(length(decor_seed)=64),
 dimensions int NOT NULL DEFAULT 256 CHECK(dimensions=256),
 geometry bytea NOT NULL CHECK(octet_length(geometry) BETWEEN 1 AND 2097152),
 geometry_checksum bytea NOT NULL CHECK(octet_length(geometry_checksum)=32),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(surface,sector,revision),
 FOREIGN KEY(surface,sector) REFERENCES planet_sectors(surface,number)
);
UPDATE planet_schema_state SET version=3 WHERE id=1 AND version=2;
