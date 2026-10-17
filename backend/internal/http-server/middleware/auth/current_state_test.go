package auth

import (
	"context"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/lib/jwt"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type currentAccount struct{}

func (currentAccount) GetUserByID(context.Context, int64) (*domain.User, error) {
	return &domain.User{ID: 1, IsPremium: true, IsVerified: true}, nil
}
func TestCurrentAccountOverridesStaleTokenFlags(t *testing.T) {
	token, err := jwt.NewAccessToken(domain.User{ID: 1}, "secret", time.Hour)
	require.NoError(t, err)
	handler := New("secret", currentAccount{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		premium, _ := GetPremiumStatus(r.Context())
		verified, _ := GetVerificationStatus(r.Context())
		require.True(t, premium)
		require.True(t, verified)
		w.WriteHeader(204)
	}))
	request := httptest.NewRequest("GET", "/", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, 204, response.Code)
}
