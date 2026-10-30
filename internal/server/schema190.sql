-- P2-5: movement/checkpoint protocol; immutable geography is unchanged.
UPDATE planet_schema_state SET version=5 WHERE id=1 AND version=4;
