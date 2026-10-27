ALTER TABLE automation_tasks DROP CONSTRAINT IF EXISTS automation_tasks_kind_check;
ALTER TABLE automation_tasks ADD CONSTRAINT automation_tasks_kind_check CHECK(kind IN('mine','haul','load','unload'));
ALTER TABLE automation_tasks ADD COLUMN IF NOT EXISTS source_kind TEXT NOT NULL DEFAULT 'personal';
ALTER TABLE automation_tasks ADD COLUMN IF NOT EXISTS dest_kind TEXT NOT NULL DEFAULT 'personal';
ALTER TABLE automation_tasks ADD COLUMN IF NOT EXISTS company_id BIGINT REFERENCES companies ON DELETE SET NULL;
CREATE TABLE IF NOT EXISTS logistics_operations (
 user_id BIGINT NOT NULL REFERENCES users ON DELETE CASCADE,request_id TEXT NOT NULL,
 digest TEXT NOT NULL,result JSONB NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(user_id,request_id)
);
CREATE INDEX IF NOT EXISTS logistics_operations_history ON logistics_operations(user_id,created_at DESC);
