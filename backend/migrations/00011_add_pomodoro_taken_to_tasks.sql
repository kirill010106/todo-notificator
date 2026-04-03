-- +gooseUp
ALTER TABLE tasks ADD COLUMN pomodoro_taken INTEGER NOT NULL DEFAULT 0;

-- +gooseDown

ALTER TABLE tasks DROP COLUMN IF EXISTS pomodoro_taken;