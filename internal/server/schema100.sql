ALTER TABLE navigation_notes ADD COLUMN IF NOT EXISTS label TEXT NOT NULL DEFAULT '' CHECK(char_length(label)<=60);
ALTER TABLE automation_tasks ADD COLUMN IF NOT EXISTS priority INTEGER NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS automation_tasks_priority ON automation_tasks(ship_id,priority DESC,id) WHERE status IN('queued','running','paused');
