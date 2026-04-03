-- +goose Up
ALTER TABLE tasks ADD COLUMN pomodoros_estimated INTEGER NOT NULL DEFAULT 1;

-- +goose Down
ALTER TABLE tasks DROP COLUMN IF EXISTS pomodoros_estimated;
