-- +goose Up
CREATE TABLE user_stats (
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    points INT DEFAULT 0,           -- Заработанные очки
    level INT DEFAULT 1,            -- Уровень (1-100)
    total_pomodoros INT DEFAULT 0,  -- Всего завершённых помодоро
    total_burnt_tasks INT DEFAULT 0,
    current_streak INT DEFAULT 0,   -- Текущая серия дней активности
    best_streak INT DEFAULT 0,      -- Лучшая серия
    updated_at TIMESTAMP DEFAULT NOW()
);

-- CREATE TABLE achievements (
--     id SERIAL PRIMARY KEY,
--     user_id INT NOT NULL REFERENCES users(id),
--     name VARCHAR(100) NOT NULL,     -- "First Pomodoro", "Streak 7 days"
--     earned_at TIMESTAMP DEFAULT NOW(),
--     UNIQUE(user_id, name)
-- );

CREATE TABLE pomodoro_sessions (
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL,
    task_id INT NOT NULL REFERENCES tasks(id),
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    duration_minutes INT,
    breaks_used INT DEFAULT 0,
    created_at TIMESTAMP DEFAULT NOW()
);

ALTER TABLE tasks ADD COLUMN burnt BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
DROP TABLE IF EXISTS user_stats;
-- DROP TABLE IF EXISTS achievements;
DROP TABLE IF EXISTS pomodoro_sessions;
ALTER TABLE tasks DROP COLUMN IF EXISTS burnt;