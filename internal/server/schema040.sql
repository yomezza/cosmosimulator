-- Station inventories and hull classes. Existing ships and market balances survive.
ALTER TABLE ships ADD COLUMN IF NOT EXISTS class TEXT NOT NULL DEFAULT 'pilgrim';
ALTER TABLE ships DROP CONSTRAINT IF EXISTS ships_fuel_check;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='ships'::regclass AND conname='ships_class_fuel') THEN
  ALTER TABLE ships ADD CONSTRAINT ships_class_fuel CHECK (
   fuel>=0 AND fuel<=CASE class WHEN 'pilgrim' THEN 40 WHEN 'wanderer' THEN 80 WHEN 'forge' THEN 60 WHEN 'ark' THEN 80 WHEN 'hermes' THEN 100 ELSE -1 END
  );
 END IF;
END $$;
ALTER TABLE local_markets ADD COLUMN IF NOT EXISTS station TEXT NOT NULL DEFAULT 'station';
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='local_markets'::regclass AND contype='p' AND pg_get_constraintdef(oid)='PRIMARY KEY (star)') THEN
  ALTER TABLE local_markets DROP CONSTRAINT local_markets_pkey;
  ALTER TABLE local_markets ADD PRIMARY KEY(star,station);
 END IF;
END $$;
ALTER TABLE reservations ADD COLUMN IF NOT EXISTS station TEXT NOT NULL DEFAULT 'station';
ALTER TABLE stations ADD COLUMN IF NOT EXISTS goods TEXT[];
CREATE TABLE IF NOT EXISTS shipyard_stock (
 star INTEGER NOT NULL,station TEXT NOT NULL,ship_class TEXT NOT NULL,
 quantity INTEGER NOT NULL CHECK(quantity>=0),price BIGINT NOT NULL CHECK(price>0),
 PRIMARY KEY(star,station,ship_class),FOREIGN KEY(star,station) REFERENCES stations(star,object_id) ON DELETE CASCADE
);
ALTER TABLE stations ADD COLUMN IF NOT EXISTS yard_seeded BOOLEAN NOT NULL DEFAULT FALSE;
