-- +goose Up
CREATE TABLE reward_wallets (
 user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
 data JSONB NOT NULL,
 CHECK ((data->>'coins')::BIGINT >= 0)
);
CREATE TABLE reward_ledger (
 id BIGSERIAL PRIMARY KEY,
 user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 event_key TEXT NOT NULL,
 coins BIGINT NOT NULL,
 description TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
 UNIQUE(user_id,event_key)
);
-- +goose Down
DROP TABLE reward_ledger;
DROP TABLE reward_wallets;
