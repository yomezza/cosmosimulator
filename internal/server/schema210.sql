-- P2-7: planetary-owned deduplication/audit of sector transitions.
CREATE TABLE IF NOT EXISTS planet_transition_receipts (
 user_id bigint NOT NULL REFERENCES users(id), operation uuid NOT NULL, offer uuid NOT NULL,
 visit_id uuid NOT NULL, surface bigint NOT NULL, revision bigint NOT NULL,
 from_sector integer NOT NULL, to_sector integer NOT NULL,
 source_x_q integer NOT NULL, source_y_q integer NOT NULL,
 arrival_x_q integer NOT NULL, arrival_y_q integer NOT NULL,
 connection_epoch bigint NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(user_id,operation), UNIQUE(user_id,offer),
 CHECK(from_sector<>to_sector),
 CHECK(source_x_q>=0 AND source_x_q<262144 AND source_y_q>=0 AND source_y_q<262144),
 CHECK(arrival_x_q>=0 AND arrival_x_q<262144 AND arrival_y_q>=0 AND arrival_y_q<262144)
);
-- Receipts keep values, not FKs to mutable/deletable geography or visit rows.
UPDATE planet_schema_state SET version=7 WHERE id=1 AND version=6;
