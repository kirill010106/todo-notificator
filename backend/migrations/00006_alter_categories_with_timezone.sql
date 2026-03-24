-- +goose Up
ALTER TABLE tasks 
ALTER COLUMN reminder_at TYPE TIMESTAMPTZ 
USING reminder_at::timestamptz;

ALTER TABLE tasks ALTER COLUMN reminder_at SET DEFAULT NULL;

-- +goose Down
ALTER TABLE tasks 
ALTER COLUMN reminder_at TYPE TIMESTAMP 
USING reminder_at::timestamp;