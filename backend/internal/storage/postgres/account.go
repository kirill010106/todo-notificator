package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

func accountError(status int, code, message string) error {
	return &domain.ContractError{Status: status, Code: code, Message: message}
}
func ValidatePassword(password string) error {
	if utf8.RuneCountInString(password) < 8 || len(password) > 72 {
		return accountError(400, "VALIDATION_ERROR", "Пароль должен содержать не менее 8 символов и не более 72 байт")
	}
	return nil
}
func ValidateDisplayName(name string) error {
	if utf8.RuneCountInString(strings.TrimSpace(name)) > 80 {
		return accountError(400, "VALIDATION_ERROR", "Имя должно содержать не более 80 символов")
	}
	return nil
}
func ResetTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// All credential operations lock the user before tokens, including verification and refresh.
func lockAccount(ctx context.Context, tx *sql.Tx, id int64) (domain.User, error) {
	var u domain.User
	err := tx.QueryRowContext(ctx, `SELECT id,email,password_hash,is_verified,is_premium,display_name,auth_version FROM users WHERE id=$1 FOR UPDATE`, id).Scan(&u.ID, &u.Email, &u.PassHash, &u.IsVerified, &u.IsPremium, &u.DisplayName, &u.AuthVersion)
	return u, err
}
func queueVerification(ctx context.Context, tx *sql.Tx, id int64, email, token string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM email_verification_tokens WHERE user_id=$1`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE verification_email_outbox SET status='cancelled',lease_token=NULL,lease_until=NULL WHERE user_id=$1 AND status='scheduled'`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO email_verification_tokens(user_id,email,token,expires_at) VALUES($1,$2,$3,CURRENT_TIMESTAMP+INTERVAL '24 hours')`, id, email, token); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO verification_email_outbox(user_id,email,token,expires_at) VALUES($1,$2,$3,CURRENT_TIMESTAMP+INTERVAL '24 hours')`, id, email, token)
	return err
}
func retireReset(ctx context.Context, tx *sql.Tx, id int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM password_reset_tokens WHERE user_id=$1`, id); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE password_reset_outbox SET status='cancelled',token=NULL,lease_token=NULL,lease_until=NULL WHERE user_id=$1 AND status='scheduled'`, id)
	return err
}
func createSession(ctx context.Context, tx *sql.Tx, id int64, token string, expires time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO refresh_tokens(user_id,token,expires_at) VALUES($1,$2,$3)`, id, token, expires.UTC())
	return err
}
func (s *Storage) RegisterAccount(ctx context.Context, email, name string, hash []byte, verification, refresh string, expires time.Time) (domain.User, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return domain.User{}, err
	}
	defer tx.Rollback()
	u := domain.User{Email: email, DisplayName: strings.TrimSpace(name)}
	err = tx.QueryRowContext(ctx, `INSERT INTO users(email,password_hash,display_name) VALUES($1,$2,$3) RETURNING id`, email, hash, u.DisplayName).Scan(&u.ID)
	if err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23505" {
			return u, storage.ErrUserExists
		}
		return u, err
	}
	if err = queueVerification(ctx, tx, u.ID, email, verification); err != nil {
		return u, err
	}
	if err = createSession(ctx, tx, u.ID, refresh, expires); err != nil {
		return u, err
	}
	return u, tx.Commit()
}
func (s *Storage) UpdateAccountProfile(ctx context.Context, id int64, name, zone *string) error {
	if name != nil {
		if err := ValidateDisplayName(*name); err != nil {
			return err
		}
		v := strings.TrimSpace(*name)
		name = &v
	}
	if zone != nil {
		if *zone == "" || *zone == "Local" {
			return accountError(400, "VALIDATION_ERROR", "Некорректный часовой пояс")
		}
		if _, err := time.LoadLocation(*zone); err != nil {
			return accountError(400, "VALIDATION_ERROR", "Некорректный часовой пояс")
		}
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET display_name=COALESCE($2,display_name),timezone=COALESCE($3,timezone) WHERE id=$1`, id, name, zone)
	return err
}
func (s *Storage) ChangeCredentials(ctx context.Context, id, version int64, current string, email *string, hash []byte, verification, refresh string, expires time.Time) (domain.User, bool, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return domain.User{}, false, err
	}
	defer tx.Rollback()
	u, err := lockAccount(ctx, tx, id)
	if err != nil {
		return u, false, err
	}
	if u.AuthVersion != version {
		return u, false, accountError(401, "UNAUTHORIZED", "Войдите в аккаунт заново")
	}
	if bcrypt.CompareHashAndPassword(u.PassHash, []byte(current)) != nil {
		return u, false, accountError(400, "INVALID_PASSWORD", "Неверный текущий пароль")
	}
	if email != nil && *email == u.Email {
		return u, false, nil
	}
	if email != nil {
		u.Email = *email
		u.IsVerified = false
	}
	if hash == nil {
		hash = u.PassHash
	}
	u.AuthVersion++
	_, err = tx.ExecContext(ctx, `UPDATE users SET email=$2,password_hash=$3,is_verified=$4,auth_version=$5 WHERE id=$1`, id, u.Email, hash, u.IsVerified, u.AuthVersion)
	if err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23505" {
			return u, false, accountError(409, "EMAIL_EXISTS", "Этот email уже используется")
		}
		return u, false, err
	}
	if err = retireReset(ctx, tx, id); err != nil {
		return u, false, err
	}
	if email != nil {
		if err = queueVerification(ctx, tx, id, u.Email, verification); err != nil {
			return u, false, err
		}
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE user_id=$1`, id); err != nil {
		return u, false, err
	}
	if err = createSession(ctx, tx, id, refresh, expires); err != nil {
		return u, false, err
	}
	return u, true, tx.Commit()
}
func (s *Storage) RequestPasswordReset(ctx context.Context, email, token string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE email=$1 FOR UPDATE`, email).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var allowed bool
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FILTER(WHERE created_at>CURRENT_TIMESTAMP-INTERVAL '1 minute')=0 AND COUNT(*)<5 FROM password_reset_outbox WHERE email=$1 AND created_at>CURRENT_TIMESTAMP-INTERVAL '1 hour'`, email).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return nil
	}
	if err = retireReset(ctx, tx, id); err != nil {
		return err
	}
	digest := ResetTokenHash(token)
	if _, err = tx.ExecContext(ctx, `INSERT INTO password_reset_tokens(user_id,email,token_hash,expires_at) VALUES($1,$2,$3,CURRENT_TIMESTAMP+INTERVAL '30 minutes')`, id, email, digest); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO password_reset_outbox(user_id,email,token,token_hash,expires_at) VALUES($1,$2,$3,$4,CURRENT_TIMESTAMP+INTERVAL '30 minutes')`, id, email, token, digest); err != nil {
		return err
	}
	return tx.Commit()
}
func invalidReset() error {
	return accountError(400, "INVALID_RESET_TOKEN", "Ссылка недействительна или срок её действия истёк. Запросите новое письмо")
}
func (s *Storage) ResetPassword(ctx context.Context, token string, hash []byte) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	digest := ResetTokenHash(token)
	var id int64
	err = tx.QueryRowContext(ctx, `SELECT user_id FROM password_reset_tokens WHERE token_hash=$1`, digest).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return invalidReset()
	}
	if err != nil {
		return err
	}
	u, err := lockAccount(ctx, tx, id)
	if err != nil {
		return err
	}
	var valid bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM password_reset_tokens WHERE token_hash=$1 AND user_id=$2 AND email=$3 AND expires_at>CURRENT_TIMESTAMP)`, digest, id, u.Email).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return invalidReset()
	}
	if _, err = tx.ExecContext(ctx, `UPDATE users SET password_hash=$2,auth_version=auth_version+1 WHERE id=$1`, id, hash); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE user_id=$1`, id); err != nil {
		return err
	}
	if err = retireReset(ctx, tx, id); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Storage) ConfirmEmail(ctx context.Context, token string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRowContext(ctx, `SELECT user_id FROM email_verification_tokens WHERE token=$1`, token).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return accountError(400, "INVALID_TOKEN", "Ссылка недействительна или срок её действия истёк")
	}
	if err != nil {
		return err
	}
	u, err := lockAccount(ctx, tx, id)
	if err != nil {
		return err
	}
	var valid bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM email_verification_tokens WHERE token=$1 AND user_id=$2 AND email=$3 AND expires_at>CURRENT_TIMESTAMP)`, token, id, u.Email).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return accountError(400, "INVALID_TOKEN", "Ссылка недействительна или срок её действия истёк")
	}
	if _, err = tx.ExecContext(ctx, `UPDATE users SET is_verified=true WHERE id=$1`, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM email_verification_tokens WHERE user_id=$1`, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE verification_email_outbox SET status='cancelled',lease_token=NULL,lease_until=NULL WHERE user_id=$1 AND status='scheduled'`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Storage) SaveRefreshTokenVersion(ctx context.Context, id, version int64, token string, expires time.Time) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	u, err := lockAccount(ctx, tx, id)
	if err != nil {
		return err
	}
	if u.AuthVersion != version {
		return accountError(401, "UNAUTHORIZED", "Данные аккаунта изменились. Войдите заново")
	}
	if err = createSession(ctx, tx, id, token, expires); err != nil {
		return err
	}
	return tx.Commit()
}
