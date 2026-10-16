package resend

import (
	"context"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
	"github.com/kirill010106/todo-notificator/internal/lib/jwt"
	"github.com/stretchr/testify/require"
)

type resenderStub struct {
	user               domain.User
	lookupErr, saveErr error
	saves              int
	token              string
	expires            time.Time
}

func (s *resenderStub) GetUserByID(_ context.Context, id int64) (*domain.User, error) {
	if id != s.user.ID {
		panic("wrong account")
	}
	return &s.user, s.lookupErr
}
func (s *resenderStub) SaveEmailVerificationToken(_ context.Context, id int64, token string, expires time.Time) error {
	if id != s.user.ID {
		panic("wrong token owner")
	}
	s.saves++
	s.token = token
	s.expires = expires
	return s.saveErr
}
func TestResendVerification(t *testing.T) {
	for _, tc := range []struct {
		name                              string
		verified                          bool
		lookupErr, saveErr                error
		webhookStatus, want, saves, calls int
	}{
		{name: "queued", webhookStatus: 204, want: 200, saves: 1},
		{name: "already verified", verified: true, want: 400},
		{name: "user lookup failed", lookupErr: errors.New("database unavailable"), want: 500},
		{name: "token not saved", saveErr: errors.New("write failed"), want: 500, saves: 1},
		{name: "email service is temporarily down", webhookStatus: 503, want: 200, saves: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &resenderStub{user: domain.User{ID: 42, Email: "test@example.com", IsVerified: tc.verified}, lookupErr: tc.lookupErr, saveErr: tc.saveErr}
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(tc.webhookStatus)
			}))
			defer server.Close()
			token, err := jwt.NewAccessToken(stub.user, "jwt", time.Hour)
			require.NoError(t, err)
			req := httptest.NewRequest("POST", "/verify/resend", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			response := httptest.NewRecorder()
			auth.New("jwt")(New(slog.New(slog.DiscardHandler), stub, server.URL, "secret")).ServeHTTP(response, req)
			require.Equal(t, tc.want, response.Code, response.Body.String())
			require.Equal(t, tc.saves, stub.saves)
			require.Equal(t, tc.calls, int(calls.Load()))
			if stub.saves > 0 {
				decoded, err := hex.DecodeString(stub.token)
				require.NoError(t, err)
				require.Len(t, decoded, 32)
				require.WithinDuration(t, time.Now().Add(24*time.Hour), stub.expires, time.Second)
			}
		})
	}
}
func TestResendRequiresAuthentication(t *testing.T) {
	response := httptest.NewRecorder()
	New(slog.New(slog.DiscardHandler), nil, "", "")(response, httptest.NewRequest("POST", "/verify/resend", nil))
	require.Equal(t, 401, response.Code)
}
func TestVerificationWebhookFailures(t *testing.T) {
	require.Error(t, sendVerificationWebhook(context.Background(), ":invalid", "", "a@b.com", "token"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.Error(t, sendVerificationWebhook(ctx, "http://127.0.0.1:1", "", "a@b.com", "token"))
	first, err := generateToken()
	require.NoError(t, err)
	second, err := generateToken()
	require.NoError(t, err)
	require.NotEqual(t, first, second)
}
