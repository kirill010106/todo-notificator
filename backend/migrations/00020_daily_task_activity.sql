-- +goose Up
CREATE TABLE daily_task_activity (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    activity_date DATE NOT NULL,
    task_id BIGINT NOT NULL,
    action TEXT NOT NULL CHECK (action IN ('created','completed')),
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY(user_id,activity_date,task_id,action)
);
CREATE INDEX daily_task_activity_user_time_idx ON daily_task_activity(user_id,recorded_at);
INSERT INTO daily_task_activity(user_id,activity_date,task_id,action,recorded_at)
SELECT t.user_id,(t.created_at AT TIME ZONE u.timezone)::date,t.id,'created',t.created_at FROM tasks t JOIN users u ON u.id=t.user_id;
INSERT INTO daily_task_activity(user_id,activity_date,task_id,action,recorded_at)
SELECT t.user_id,(t.updated_at AT TIME ZONE u.timezone)::date,t.id,'completed',t.updated_at FROM tasks t JOIN users u ON u.id=t.user_id WHERE t.status='done';
-- +goose StatementBegin
CREATE FUNCTION record_daily_task_activity() RETURNS TRIGGER AS $$
DECLARE zone TEXT;
BEGIN
    SELECT timezone INTO zone FROM users WHERE id=NEW.user_id;
    IF TG_OP='INSERT' THEN
        INSERT INTO daily_task_activity(user_id,activity_date,task_id,action)
        VALUES(NEW.user_id,(CURRENT_TIMESTAMP AT TIME ZONE zone)::date,NEW.id,'created') ON CONFLICT DO NOTHING;
    END IF;
    IF NEW.status='done' AND (TG_OP='INSERT' OR OLD.status IS DISTINCT FROM 'done') THEN
        INSERT INTO daily_task_activity(user_id,activity_date,task_id,action)
        VALUES(NEW.user_id,(CURRENT_TIMESTAMP AT TIME ZONE zone)::date,NEW.id,'completed') ON CONFLICT DO NOTHING;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER tasks_daily_activity AFTER INSERT OR UPDATE ON tasks FOR EACH ROW EXECUTE FUNCTION record_daily_task_activity();

-- +goose Down
DROP TRIGGER tasks_daily_activity ON tasks;
DROP FUNCTION record_daily_task_activity();
DROP TABLE daily_task_activity;
