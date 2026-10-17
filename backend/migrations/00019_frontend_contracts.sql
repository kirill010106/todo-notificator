-- +goose Up
ALTER TABLE users ADD COLUMN timezone TEXT NOT NULL DEFAULT 'UTC';
ALTER TABLE pomodoro_sessions ADD COLUMN break_started_at TIMESTAMPTZ;
ALTER TABLE pomodoro_sessions ADD COLUMN break_ends_at TIMESTAMPTZ;
CREATE TABLE focus_states (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    completed_in_cycle INTEGER NOT NULL DEFAULT 0 CHECK (completed_in_cycle BETWEEN 0 AND 3),
    recovery_ends_at TIMESTAMPTZ,
    recovery_task_id BIGINT REFERENCES tasks(id) ON DELETE SET NULL
);
CREATE TABLE api_operations (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    idempotency_key TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    response JSONB NOT NULL,
    status_code INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, idempotency_key)
);
CREATE TABLE daily_reward_claims (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reward_date DATE NOT NULL,
    code TEXT NOT NULL,
    xp INTEGER NOT NULL,
    claimed_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, reward_date, code)
);
CREATE INDEX tasks_user_deadline_idx ON tasks(user_id, deadline) WHERE status = 'pending';

-- +goose Down
DROP INDEX tasks_user_deadline_idx;
DROP TABLE daily_reward_claims;
DROP TABLE api_operations;
DROP TABLE focus_states;
ALTER TABLE pomodoro_sessions DROP COLUMN break_ends_at;
ALTER TABLE pomodoro_sessions DROP COLUMN break_started_at;
ALTER TABLE users DROP COLUMN timezone;
