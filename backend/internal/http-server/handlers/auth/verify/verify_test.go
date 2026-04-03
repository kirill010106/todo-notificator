package verify

import (
"context"
"encoding/json"
"errors"
"log/slog"
"net/http"
"net/http/httptest"
"testing"
"time"

"github.com/kirill010106/todo-notificator/internal/domain"
"github.com/kirill010106/todo-notificator/internal/storage"
"github.com/stretchr/testify/assert"
)

type mockEmailVerifier struct {
TokenInfo domain.EmailVerificationToken
GetErr    error
VerifyErr error
DelErr    error
}

func (m *mockEmailVerifier) GetEmailVerificationToken(ctx context.Context, token string) (domain.EmailVerificationToken, error) {
return m.TokenInfo, m.GetErr
}

func (m *mockEmailVerifier) VerifyUserEmail(ctx context.Context, userID int64) error {
return m.VerifyErr
}

func (m *mockEmailVerifier) DeleteEmailVerificationToken(ctx context.Context, token string) error {
return m.DelErr
}

func TestVerify(t *testing.T) {
validToken := domain.EmailVerificationToken{
Token:     "valid-token",
UserID:    1,
ExpiresAt: time.Now().Add(1 * time.Hour),
}

expiredToken := domain.EmailVerificationToken{
Token:     "expired-token",
UserID:    2,
ExpiresAt: time.Now().Add(-1 * time.Hour),
}

tests := []struct {
name       string
token      string
mock       *mockEmailVerifier
wantCode   int
wantStatus string
}{
{
name:  "happy path",
token: "valid-token",
mock: &mockEmailVerifier{
TokenInfo: validToken,
GetErr:    nil,
},
wantCode:   http.StatusOK,
wantStatus: "OK",
},
{
name:  "missing token",
token: "",
mock:  &mockEmailVerifier{},
wantCode:   http.StatusBadRequest,
wantStatus: "Error",
},
{
name:  "invalid token",
token: "bad-token",
mock: &mockEmailVerifier{
GetErr: storage.ErrTokenNotFound,
},
wantCode:   http.StatusBadRequest,
wantStatus: "Error",
},
{
name:  "expired token",
token: "expired-token",
mock: &mockEmailVerifier{
TokenInfo: expiredToken,
GetErr:    nil,
},
wantCode:   http.StatusBadRequest,
wantStatus: "Error",
},
{
name:  "db get error",
token: "valid-token",
mock: &mockEmailVerifier{
GetErr: errors.New("db error"),
},
wantCode:   http.StatusInternalServerError,
wantStatus: "Error",
},
}

for _, tt := range tests {
t.Run(tt.name, func(t *testing.T) {
handler := New(slog.New(slog.DiscardHandler), tt.mock)

req := httptest.NewRequest("GET", "/verify", nil)
if tt.token != "" {
q := req.URL.Query()
q.Add("token", tt.token)
req.URL.RawQuery = q.Encode()
}

w := httptest.NewRecorder()

handler(w, req)

var response Response
err := json.NewDecoder(w.Body).Decode(&response)
assert.NoError(t, err)
assert.Equal(t, tt.wantCode, w.Code)
assert.Equal(t, tt.wantStatus, response.Response.Status)
})
}
}
