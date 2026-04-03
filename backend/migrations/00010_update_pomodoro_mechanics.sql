-- +goose Up
-- +goose StatementBegin
ALTER TABLE pomodoro_sessions ALTER COLUMN task_id DROP NOT NULL;
ALTER TABLE pomodoro_sessions ADD COLUMN status VARCHAR(20) NOT NULL DEFAULT 'active';
ALTER TYPE task_status ADD VALUE IF NOT EXISTS 'burnt';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE pomodoro_sessions DROP COLUMN IF EXISTS status;
DELETE FROM pomodoro_sessions WHERE task_id IS NULL;
ALTER TABLE pomodoro_sessions ALTER COLUMN task_id SET NOT NULL;
-- +goose StatementEnd
