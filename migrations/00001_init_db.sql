-- +goose Up
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'task_status') THEN
CREATE TYPE task_status AS ENUM ('pending', 'done');
END IF;
END $$;
-- +goose StatementEnd

CREATE TABLE IF NOT EXISTS users (
                                     id SERIAL PRIMARY KEY,
                                     email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    telegram_id BIGINT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT now()
    );

CREATE TABLE IF NOT EXISTS tasks (
                                     id SERIAL PRIMARY KEY,
                                     user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    deadline TIMESTAMP,
    status task_status DEFAULT 'pending',
    is_notified BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT now()
    );

-- +goose Down
DROP TABLE IF EXISTS tasks;
DROP TABLE IF EXISTS users;
DROP TYPE IF EXISTS task_status;