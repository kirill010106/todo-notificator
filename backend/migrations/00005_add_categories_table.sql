-- +goose Up
CREATE TABLE categories (
    id SERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(64) NOT NULL,
    UNIQUE (user_id, name)
);

ALTER TABLE tasks ADD COLUMN category_id INTEGER REFERENCES categories(id);

CREATE INDEX idx_tasks_category_id ON tasks(category_id);

-- +goose Down
DROP TABLE categories;
ALTER TABLE tasks DROP COLUMN category_id;
DROP INDEX idx_tasks_category_id;