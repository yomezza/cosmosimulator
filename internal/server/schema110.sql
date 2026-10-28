-- Remove the former gameplay ceiling while keeping browser-safe exact integers.
CREATE OR REPLACE FUNCTION enforce_credit_ceiling() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.credits<0 OR (NEW.credits>9007199254740991 AND (TG_OP='INSERT' OR NEW.credits>OLD.credits)) THEN
  RAISE EXCEPTION 'Сумма превышает технический диапазон' USING ERRCODE='23514',CONSTRAINT='credits_ceiling';
 END IF; RETURN NEW; END $$;
