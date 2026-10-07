package premium

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
	"github.com/kirill010106/todo-notificator/internal/lib/jwt"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockPremiumToggler struct {
	mock.Mock
}

func (m *mockPremiumToggler) GetUserByID(ctx context.Context, userID int64) (*domain.User, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.User), args.Error(1)
}

func (m *mockPremiumToggler) SetPremiumStatus(ctx context.Context, userID int64, isPremium bool) error {
	args := m.Called(ctx, userID, isPremium)
	return args.Error(0)
}

func TestTogglePremium_Success(t *testing.T) {
	secret := "test-secret-key-32-chars-minimum!!"
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	toggler := new(mockPremiumToggler)

	toggler.On("GetUserByID", mock.Anything, int64(42)).Return(&domain.User{
		ID:        42,
		IsPremium: false,
	}, nil)
	toggler.On("SetPremiumStatus", mock.Anything, int64(42), true).Return(nil)

	h := New(log, toggler)
	mw := auth.New(secret)
	wrapped := mw(h)

	token, err := jwt.NewAccessToken(domain.User{ID: 42, Email: "u@t.com", IsPremium: false}, secret, time.Hour)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/dev/toggle-premium", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"is_premium":true`)
}
