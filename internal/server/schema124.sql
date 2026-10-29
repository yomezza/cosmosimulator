-- Tombstones prevent ordinary viewing from recreating an administratively reset
-- surface. Revision is independent of generator format version and never reused.
CREATE TABLE IF NOT EXISTS planet_surface_controls (
 star int NOT NULL, planet int NOT NULL, revision bigint NOT NULL DEFAULT 0 CHECK(revision>=0),
 deleted boolean NOT NULL DEFAULT false, PRIMARY KEY(star,planet)
);
