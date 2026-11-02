-- +goose Up
-- Revoke plaintext sessions, including tokens exposed by older database copies.
-- Writers must be stopped while migrating, as required by the release procedure.
DELETE FROM refresh_tokens;
ALTER TABLE refresh_tokens RENAME COLUMN token TO token_hash;
ALTER TABLE refresh_tokens ADD CONSTRAINT refresh_tokens_hash_format
    CHECK (token_hash ~ '^[0-9a-f]{64}$');
ALTER INDEX idx_refresh_tokens_token RENAME TO idx_refresh_tokens_token_hash;

-- +goose Down
-- Hashes cannot be converted back into bearer tokens. Revoke sessions on rollback.
DELETE FROM refresh_tokens;
ALTER TABLE refresh_tokens DROP CONSTRAINT refresh_tokens_hash_format;
ALTER TABLE refresh_tokens RENAME COLUMN token_hash TO token;
ALTER INDEX idx_refresh_tokens_token_hash RENAME TO idx_refresh_tokens_token;
