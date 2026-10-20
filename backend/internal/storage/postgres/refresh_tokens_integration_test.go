//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	todonotificator "github.com/kirill010106/todo-notificator"
	"github.com/kirill010106/todo-notificator/internal/config"
	"github.com/kirill010106/todo-notificator/internal/http-server/router"
	"github.com/kirill010106/todo-notificator/internal/storage"
	"github.com/kirill010106/todo-notificator/internal/storage/postgres"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestRefreshTokenSecurityIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	// Local runs can use a disposable native PostgreSQL cluster when Docker is
	// unavailable. Refuse a nonempty database before applying any migration.
	dsn := os.Getenv("REFRESH_SECURITY_TEST_DSN")
	if dsn == "" {
		pg, err := tcpostgres.Run(ctx, "postgres:16-alpine", tcpostgres.WithDatabase("refresh_security"),
			tcpostgres.WithUsername("postgres"), tcpostgres.WithPassword("postgres"),
			testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)))
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, pg.Terminate(context.Background())) })
		dsn, err = pg.ConnectionString(ctx, "sslmode=disable")
		require.NoError(t, err)
	}
	s, err := postgres.New(dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	var tables int
	require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema NOT IN ('pg_catalog','information_schema') AND table_schema NOT LIKE 'pg_toast%'`).Scan(&tables))
	require.Zero(t, tables, "REFRESH_SECURITY_TEST_DSN must reference an empty disposable database")
	goose.SetBaseFS(todonotificator.MigrationsFS)
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.UpTo(s.DB, "migrations", 25))
	legacyUser, err := s.SaveUser(ctx, "legacy@test.local", []byte("legacy"))
	require.NoError(t, err)
	_, err = s.DB.ExecContext(ctx, `INSERT INTO refresh_tokens(user_id,token,expires_at) VALUES($1,$2,$3)`, legacyUser, "legacy-plaintext", time.Now().Add(time.Hour))
	require.NoError(t, err)
	require.NoError(t, goose.Up(s.DB, "migrations"))
	var count int
	require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM refresh_tokens`).Scan(&count))
	require.Zero(t, count, "upgrade must revoke all legacy plaintext tokens")
	_, err = s.RotateRefreshToken(ctx, "legacy-plaintext", "unused", time.Now().Add(time.Hour))
	require.ErrorIs(t, err, storage.ErrRefreshTokenInvalid)
	require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.columns WHERE table_name='refresh_tokens' AND column_name='token'`).Scan(&count))
	require.Zero(t, count, "old binaries must not be able to insert plaintext into the old column")

	h := router.New(router.Config{Storage: s, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		AppConfig: &config.Config{Env: "prod", AppSecret: "refresh-security-test-secret", AccessTokenTTL: time.Minute, RefreshTokenTTL: time.Hour}})
	request := func(path, access string, body any, status int) map[string]any {
		t.Helper()
		data, err := json.Marshal(body)
		require.NoError(t, err)
		r := httptest.NewRequest("POST", "/api/v1"+path, bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/json")
		if access != "" {
			r.Header.Set("Authorization", "Bearer "+access)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		require.Equal(t, status, w.Code, "%s: %s", path, w.Body.String())
		var result map[string]any
		if w.Body.Len() > 0 {
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
		}
		return result
	}
	digest := func(raw string) string { sum := sha256.Sum256([]byte(raw)); return hex.EncodeToString(sum[:]) }
	assertStored := func(raw string) {
		t.Helper()
		var stored string
		require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT token_hash FROM refresh_tokens WHERE token_hash=$1`, digest(raw)).Scan(&stored))
		require.Equal(t, digest(raw), stored)
		require.NotEqual(t, raw, stored)
	}
	credentials := map[string]string{"email": "secure@test.local", "password": "password123"}
	registered := request("/register", "", credentials, 200)
	registeredToken := registered["refresh_token"].(string)
	assertStored(registeredToken)
	accountID := int64(registered["user_id"].(float64))
	require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM verification_email_outbox WHERE user_id=$1`, accountID).Scan(&count))
	require.Equal(t, 1, count, "registration still queues verification atomically")
	loggedIn := request("/login", "", credentials, 200)
	loginToken := loggedIn["refresh_token"].(string)
	assertStored(loginToken)
	// A read-only database leak must not provide a usable refresh credential.
	request("/refresh", "", map[string]string{"refresh_token": digest(loginToken)}, 401)
	rotated := request("/refresh", "", map[string]string{"refresh_token": loginToken}, 200)
	rotatedToken := rotated["refresh_token"].(string)
	assertStored(rotatedToken)
	request("/refresh", "", map[string]string{"refresh_token": loginToken}, 401)
	// A copied digest must not revoke the legitimate bearer token either.
	request("/logout", rotated["access_token"].(string), map[string]string{"refresh_token": digest(rotatedToken)}, 204)
	assertStored(rotatedToken)
	request("/logout", rotated["access_token"].(string), map[string]string{"refresh_token": rotatedToken}, 204)
	request("/refresh", "", map[string]string{"refresh_token": rotatedToken}, 401)

	changed := request("/me/password", registered["access_token"].(string),
		map[string]string{"current_password": "password123", "new_password": "password456"}, 200)
	changedToken := changed["refresh_token"].(string)
	assertStored(changedToken)
	request("/refresh", "", map[string]string{"refresh_token": registeredToken}, 401)
	request("/password/forgot", "", map[string]string{"email": credentials["email"]}, 202)
	var reset string
	require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT token FROM password_reset_outbox WHERE user_id=$1 AND status='scheduled'`, accountID).Scan(&reset))
	request("/password/reset", "", map[string]string{"token": reset, "new_password": "password789"}, 200)
	request("/refresh", "", map[string]string{"refresh_token": changedToken}, 401)
	credentials["password"] = "password789"
	current := request("/login", "", credentials, 200)
	currentToken := current["refresh_token"].(string)
	assertStored(currentToken)
	// Hashing must preserve the existing single-winner rotation guarantee.
	winners := make(chan string, 8)
	errors := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			next := fmt.Sprintf("concurrent-replacement-%d", i)
			_, err := s.RotateRefreshToken(ctx, currentToken, next, time.Now().Add(time.Hour))
			if err == nil {
				winners <- next
			} else {
				errors <- err
			}
		}(i)
	}
	wg.Wait()
	require.Len(t, winners, 1)
	require.Len(t, errors, 7)
	for len(errors) > 0 {
		require.ErrorIs(t, <-errors, storage.ErrRefreshTokenInvalid)
	}
	assertStored(<-winners)
	request("/logout", current["access_token"].(string), map[string]any{"refresh_token": currentToken, "all_devices": true}, 204)
	require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM refresh_tokens WHERE user_id=$1`, accountID).Scan(&count))
	require.Zero(t, count)
	// Rollback cannot restore bearer values from digests; both directions revoke
	// sessions while preserving user and verification/outbox data.
	require.NoError(t, s.SaveRefreshToken(ctx, accountID, "before-rollback", time.Now().Add(time.Hour)))
	require.NoError(t, goose.Down(s.DB, "migrations"))
	require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM refresh_tokens`).Scan(&count))
	require.Zero(t, count)
	require.NoError(t, goose.Up(s.DB, "migrations"))
	_, err = s.GetUserByID(ctx, accountID)
	require.NoError(t, err)
	require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM verification_email_outbox WHERE user_id=$1`, accountID).Scan(&count))
	require.Equal(t, 1, count)
}
