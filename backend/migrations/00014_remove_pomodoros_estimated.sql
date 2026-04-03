-- +gooseUp

ALTER TABLE tasks DROP COLUMN IF EXISTS pomodoros_estimated;

-- +gooseDown

ALTER TABLE tasks ADD COLUMN pomodoros_estimated INTEGER NOT NULL DEFAULT 1;