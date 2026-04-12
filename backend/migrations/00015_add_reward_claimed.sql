-- +gooseUp
ALTER TABLE tasks ADD COLUMN reward_claimed BOOLEAN NOT NULL DEFAULT FALSE;
-- +gooseDown
ALTER TABLE tasks DROP COLUMN IF EXISTS reward_claimed;