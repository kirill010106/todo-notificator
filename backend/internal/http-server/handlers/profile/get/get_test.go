package get

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/kirill010106/todo-notificator/internal/domain"
	authmw "github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
	"github.com/kirill010106/todo-notificator/internal/lib/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockProfileProvider struct {
	mock.Mock
}

func (m *mockProfileProvider) GetUserByID(ctx context.Context, userID int64) (*domain.User, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.User), args.Error(1)
}

func (m *mockProfileProvider) CountCategories(ctx context.Context, userID int64) (int, error) {
	args := m.Called(ctx, userID)
	return args.Int(0), args.Error(1)
}

func (m *mockProfileProvider) CountActiveReminders(ctx context.Context, userID int64) (int, error) {
	args := m.Called(ctx, userID)
	return args.Int(0), args.Error(1)
}

func TestProfileGet_Success(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockP := new(mockProfileProvider)

	secret := "secret"
	userID := int64(10)
	user := &domain.User{
		ID:         userID,
		Email:      "user@example.com",
		IsVerified: true,
		IsPremium:  true,
	}

	tok, err := jwt.NewAccessToken(*user, secret, time.Hour)
	require.NoError(t, err)

	mockP.On("GetUserByID", mock.Anything, userID).Return(user, nil)
	mockP.On("CountCategories", mock.Anything, userID).Return(5, nil)
	mockP.On("CountActiveReminders", mock.Anything, userID).Return(2, nil)

	handler := New(log, mockP)
	r := chi.NewRouter()
	r.Use(authmw.New(secret))
	r.Get("/api/v1/me/profile", handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/profile", nil)
	req.Header.Set("Authorization", "Bearer "+tok)

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var resp Response
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "OK", resp.Status)
	assert.Equal(t, "user@example.com", resp.Profile.Email)
	assert.True(t, resp.Profile.IsPremium)
	assert.Equal(t, -1, resp.Profile.CategoriesLimit)
}
