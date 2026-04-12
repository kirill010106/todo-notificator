-- +gooseUp
ALTER TABLE rewards ADD COLUMN reward_claimed BOOLEAN NOT NULL DEFAULT FALSE;
-- +gooseDown
ALTER TABLE rewards DROP COLUMN IF EXISTS reward_claimed;