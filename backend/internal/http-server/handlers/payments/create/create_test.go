package create

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

type mockPaymentSaver struct {
	mock.Mock
}

func (m *mockPaymentSaver) CreatePayment(ctx context.Context, yookassaID string, userID int64, amount string, currency string, status string, description string) (int64, error) {
	args := m.Called(ctx, yookassaID, userID, amount, currency, status, description)
	return args.Get(0).(int64), args.Error(1)
}

func TestCreatePayment_WhenClientNil_ReturnsServiceUnavailable(t *testing.T) {
	secret := "test-secret-key-32-chars-minimum!!"
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	saver := new(mockPaymentSaver)

	// Passing nil yooClient (simulates unconfigured YooKassa)
	h := New(log, saver, nil, "http://localhost:3000/return")
	mw := auth.New(secret)
	wrappedHandler := mw(h)

	token, err := jwt.NewAccessToken(domain.User{
		ID:         42,
		Email:      "user@test.com",
		IsVerified: true,
		IsPremium:  false,
	}, secret, time.Hour)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/payments/create", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	saver.AssertNotCalled(t, "CreatePayment")
}
