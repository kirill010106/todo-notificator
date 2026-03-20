-- +goose Up
ALTER TABLE tasks ADD COLUMN reminder_at TIMESTAMP;

-- +goose Down
ALTER TABLE tasks DROP COLUMN reminder_at;