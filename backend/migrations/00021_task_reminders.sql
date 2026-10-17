-- +goose Up
-- Legacy deadline values are UTC wall times, as returned by the old API.
ALTER TABLE tasks ALTER COLUMN deadline TYPE TIMESTAMPTZ USING deadline AT TIME ZONE 'UTC';
CREATE TABLE task_reminders (
    id BIGSERIAL PRIMARY KEY,
    task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    offset_minutes BIGINT CHECK (offset_minutes BETWEEN 0 AND 52560000),
    scheduled_at TIMESTAMPTZ NOT NULL,
    legacy_absolute BOOLEAN NOT NULL DEFAULT false,
    status TEXT NOT NULL DEFAULT 'scheduled' CHECK (status IN ('scheduled','sent','failed','cancelled')),
    sent_at TIMESTAMPTZ,
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL,
    lease_until TIMESTAMPTZ,
    lease_token TEXT,
    last_error TEXT,
    CHECK ((legacy_absolute AND offset_minutes IS NULL) OR (NOT legacy_absolute AND offset_minutes IS NOT NULL))
);
CREATE INDEX task_reminders_task_idx ON task_reminders(task_id);
CREATE INDEX task_reminders_due_idx ON task_reminders(next_attempt_at) WHERE status='scheduled';
CREATE TABLE reminder_delivery_attempts (
    id BIGSERIAL PRIMARY KEY,
    reminder_id BIGINT NOT NULL REFERENCES task_reminders(id) ON DELETE CASCADE,
    attempted_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    attempt_number INTEGER NOT NULL,
    lease_token TEXT NOT NULL,
    error TEXT,
    succeeded BOOLEAN NOT NULL
);

-- Preserve exact legacy instants, including sub-minute differences. A relative
-- reminder requires an integral minute offset; the rest remain legacy exceptions.
INSERT INTO task_reminders(task_id,offset_minutes,scheduled_at,legacy_absolute,status,next_attempt_at)
SELECT id,
  CASE WHEN deadline IS NOT NULL AND reminder_at<=deadline
    AND EXTRACT(EPOCH FROM (deadline-reminder_at)) / 60 BETWEEN 0 AND 52560000
    AND MOD(EXTRACT(EPOCH FROM (deadline-reminder_at)),60)=0
    THEN (EXTRACT(EPOCH FROM (deadline-reminder_at))/60)::BIGINT END,
  reminder_at,
  NOT (deadline IS NOT NULL AND reminder_at<=deadline
    AND EXTRACT(EPOCH FROM (deadline-reminder_at))/60 BETWEEN 0 AND 52560000
    AND MOD(EXTRACT(EPOCH FROM (deadline-reminder_at)),60)=0),
  CASE WHEN COALESCE(is_notified,false) THEN 'sent'
       WHEN status<>'pending' THEN 'cancelled' ELSE 'scheduled' END,
  reminder_at
FROM tasks WHERE reminder_at IS NOT NULL;
-- +goose StatementBegin
DO $$ BEGIN
  IF (SELECT COUNT(*) FROM task_reminders) <> (SELECT COUNT(*) FROM tasks WHERE reminder_at IS NOT NULL) THEN
    RAISE EXCEPTION 'Reminder migration count mismatch';
  END IF;
END $$;
-- +goose StatementEnd

-- Cancellation applies to every writer (single, bulk, focus, and direct SQL).
-- +goose StatementBegin
CREATE FUNCTION cancel_task_reminders() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.status <> 'pending' THEN
    UPDATE task_reminders SET status='cancelled',lease_token=NULL,lease_until=NULL
      WHERE task_id=NEW.id AND status IN ('scheduled','failed');
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER cancel_task_reminders_trigger AFTER UPDATE OF status ON tasks
FOR EACH ROW EXECUTE FUNCTION cancel_task_reminders();

-- +goose Down
-- Down cannot reconstruct multiple reminders in one legacy column. Back up first.
DROP TRIGGER cancel_task_reminders_trigger ON tasks;
DROP FUNCTION cancel_task_reminders();
DROP TABLE reminder_delivery_attempts;
DROP TABLE task_reminders;
ALTER TABLE tasks ALTER COLUMN deadline TYPE TIMESTAMP USING deadline AT TIME ZONE 'UTC';
