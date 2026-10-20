-- +goose Up
ALTER TABLE users ADD COLUMN display_name TEXT NOT NULL DEFAULT '' CHECK (char_length(display_name)<=80);
ALTER TABLE users ADD COLUMN auth_version BIGINT NOT NULL DEFAULT 0;
ALTER TABLE email_verification_tokens ADD COLUMN email TEXT;
UPDATE email_verification_tokens t SET email=u.email FROM users u WHERE u.id=t.user_id;
ALTER TABLE email_verification_tokens ALTER COLUMN email SET NOT NULL;
CREATE TABLE password_reset_tokens (
 token_hash TEXT PRIMARY KEY, user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 email TEXT NOT NULL, expires_at TIMESTAMPTZ NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX password_reset_tokens_user_idx ON password_reset_tokens(user_id);
CREATE TABLE password_reset_outbox (
 id BIGSERIAL PRIMARY KEY, user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 email TEXT NOT NULL, token TEXT, token_hash TEXT NOT NULL, expires_at TIMESTAMPTZ NOT NULL,
 status TEXT NOT NULL DEFAULT 'scheduled' CHECK(status IN ('scheduled','sent','failed','cancelled')),
 attempts INTEGER NOT NULL DEFAULT 0, next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
 lease_until TIMESTAMPTZ, lease_token TEXT, sent_at TIMESTAMPTZ, last_error TEXT,
 created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX password_reset_outbox_due_idx ON password_reset_outbox(next_attempt_at,id) WHERE status='scheduled';
CREATE INDEX password_reset_outbox_rate_idx ON password_reset_outbox(email,created_at);
-- +goose Down
DROP TABLE password_reset_outbox;
DROP TABLE password_reset_tokens;
ALTER TABLE email_verification_tokens DROP COLUMN email;
ALTER TABLE users DROP COLUMN auth_version;
ALTER TABLE users DROP COLUMN display_name;
