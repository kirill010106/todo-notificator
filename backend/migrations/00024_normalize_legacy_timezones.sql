-- +goose Up
-- Go previously accepted the special name Local, which PostgreSQL cannot use.
-- Its original device zone cannot be recovered; use UTC until the user selects one.
UPDATE users SET timezone = 'UTC'
WHERE NOT EXISTS (SELECT 1 FROM pg_timezone_names WHERE name = users.timezone);

-- +goose Down
-- The original invalid zone is deliberately not restored.
SELECT 1;
