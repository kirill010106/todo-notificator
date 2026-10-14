package login

import (
	"errors"
	"github.com/kirill010106/todo-notificator/internal/config"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLoginValidationAndTokenPersistence(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	require.NoError(t, err)
	for _, tc := range []struct {
		name, body string
		saveErr    error
		want       int
	}{
		{"empty", "", nil, 400}, {"malformed", "{", nil, 400}, {"invalid email", `{"email":"bad","password":"password123"}`, nil, 400},
		{"short password", `{"email":"u@test.com","password":"x"}`, nil, 400},
		{"incorrect password", `{"email":"u@test.com","password":"incorrect"}`, nil, 401},
		{"cannot persist token", `{"email":"u@test.com","password":"password123"}`, errors.New("db down"), 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{AppSecret: "secret", AccessTokenTTL: time.Hour, RefreshTokenTTL: time.Hour}
			provider := mockUserProvider{user: domain.User{ID: 42, PassHash: hash}, saveErr: tc.saveErr}
			res := httptest.NewRecorder()
			New(slog.New(slog.DiscardHandler), provider, cfg, nil)(res, httptest.NewRequest("POST", "/login", strings.NewReader(tc.body)))
			require.Equal(t, tc.want, res.Code, res.Body.String())
			require.NotContains(t, res.Body.String(), `"access_token"`)
		})
	}
}
