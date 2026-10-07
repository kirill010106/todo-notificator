-- +goose Up
ALTER TABLE user_stats ADD COLUMN IF NOT EXISTS last_activity_date DATE;

CREATE TABLE IF NOT EXISTS achievements (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code VARCHAR(64) NOT NULL,
    unlocked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT unique_user_achievement UNIQUE(user_id, code)
);

CREATE INDEX IF NOT EXISTS idx_achievements_user_id ON achievements(user_id);

-- +goose Down
DROP TABLE IF EXISTS achievements;
ALTER TABLE user_stats DROP COLUMN IF EXISTS last_activity_date;
