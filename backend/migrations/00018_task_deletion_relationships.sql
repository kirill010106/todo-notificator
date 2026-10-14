-- +goose Up
ALTER TABLE tasks
    DROP CONSTRAINT tasks_category_id_fkey,
    ADD CONSTRAINT tasks_category_id_fkey
        FOREIGN KEY (category_id) REFERENCES categories(id) ON DELETE SET NULL;

ALTER TABLE pomodoro_sessions
    DROP CONSTRAINT pomodoro_sessions_task_id_fkey,
    ADD CONSTRAINT pomodoro_sessions_task_id_fkey
        FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE SET NULL;

-- Stop linked active sessions before the foreign key detaches their task IDs.
-- The trigger and DELETE share a transaction, including for bulk deletion.
-- +goose StatementBegin
CREATE FUNCTION abandon_pomodoros_on_task_delete() RETURNS trigger AS $$
BEGIN
    UPDATE pomodoro_sessions
    SET status = 'abandoned', completed_at = CURRENT_TIMESTAMP
    WHERE task_id = OLD.id AND status = 'active';
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER tasks_abandon_pomodoros_before_delete
    BEFORE DELETE ON tasks
    FOR EACH ROW EXECUTE FUNCTION abandon_pomodoros_on_task_delete();

-- +goose Down
DROP TRIGGER tasks_abandon_pomodoros_before_delete ON tasks;
DROP FUNCTION abandon_pomodoros_on_task_delete();

ALTER TABLE pomodoro_sessions
    DROP CONSTRAINT pomodoro_sessions_task_id_fkey,
    ADD CONSTRAINT pomodoro_sessions_task_id_fkey
        FOREIGN KEY (task_id) REFERENCES tasks(id);

ALTER TABLE tasks
    DROP CONSTRAINT tasks_category_id_fkey,
    ADD CONSTRAINT tasks_category_id_fkey
        FOREIGN KEY (category_id) REFERENCES categories(id);
