package jwt

import (
	"testing"
	"time"

	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestNewAccessToken_And_Parse(t *testing.T) {
	secret := "test-secret-key-32-chars-minimum!!"
	user := domain.User{
		ID:         42,
		Email:      "user@example.com",
		IsVerified: true,
		IsPremium:  true,
	}

	token, err := NewAccessToken(user, secret, 15*time.Minute)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	claims, err := ParseAccessToken(token, secret)
	require.NoError(t, err)
	require.NotNil(t, claims)
	require.Equal(t, int64(42), claims.UserID)
	require.Equal(t, "user@example.com", claims.Email)
	require.True(t, claims.IsVerified)
	require.True(t, claims.IsPremium)
}

func TestParseAccessToken_NonPremiumNonVerified(t *testing.T) {
	secret := "test-secret-key-32-chars-minimum!!"
	user := domain.User{
		ID:         100,
		Email:      "free@example.com",
		IsVerified: false,
		IsPremium:  false,
	}

	token, err := NewAccessToken(user, secret, 15*time.Minute)
	require.NoError(t, err)

	claims, err := ParseAccessToken(token, secret)
	require.NoError(t, err)
	require.Equal(t, int64(100), claims.UserID)
	require.False(t, claims.IsVerified)
	require.False(t, claims.IsPremium)
}

func TestParseAccessToken_InvalidSecret(t *testing.T) {
	user := domain.User{ID: 1, Email: "test@test.com"}
	token, err := NewAccessToken(user, "secret-one-12345678901234567890", time.Minute)
	require.NoError(t, err)

	_, err = ParseAccessToken(token, "secret-two-12345678901234567890")
	require.Error(t, err)
}

func TestParseAccessToken_Expired(t *testing.T) {
	secret := "test-secret-key-32-chars-minimum!!"
	user := domain.User{ID: 1, Email: "test@test.com"}
	token, err := NewAccessToken(user, secret, -time.Minute)
	require.NoError(t, err)

	_, err = ParseAccessToken(token, secret)
	require.Error(t, err)
}

func TestNewRefreshToken(t *testing.T) {
	t1, err := NewRefreshToken()
	require.NoError(t, err)
	require.Len(t, t1, 64)

	t2, err := NewRefreshToken()
	require.NoError(t, err)
	require.Len(t, t2, 64)
	require.NotEqual(t, t1, t2)
}
