package create

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
	"github.com/kirill010106/todo-notificator/internal/lib/jwt"
	"github.com/rvinnie/yookassa-sdk-go/yookassa"
	"github.com/rvinnie/yookassa-sdk-go/yookassa/opts"
	"github.com/stretchr/testify/require"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type paymentTransport func(*http.Request) (*http.Response, error)

func (f paymentTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type checkedSaver struct {
	t     *testing.T
	err   error
	calls int
}

func (s *checkedSaver) CreatePayment(_ context.Context, id string, user int64, amount, currency, status, description string) (int64, error) {
	s.calls++
	require.Equal(s.t, "payment-1", id)
	require.Equal(s.t, int64(42), user)
	require.Equal(s.t, "49.00", amount)
	require.Equal(s.t, "RUB", currency)
	require.Equal(s.t, "pending", status)
	require.NotEmpty(s.t, description)
	return 1, s.err
}
func TestPaymentGatewayContract(t *testing.T) {
	for _, tc := range []struct {
		name, returnURL, response string
		networkErr, saveErr       error
		want, saves               int
	}{
		{name: "redirect", returnURL: "https://app.test", response: `{"id":"payment-1","status":"pending","confirmation":{"type":"redirect","confirmation_url":"https://checkout.test/pay"}}`, want: 200, saves: 1},
		{name: "return with query", returnURL: "https://app.test/?source=web", response: `{"id":"payment-1","status":"pending","confirmation":{"type":"redirect","confirmation_url":"https://checkout.test/pay"}}`, want: 200, saves: 1},
		{name: "network failure", networkErr: errors.New("connection lost"), want: 500},
		{name: "invalid gateway response", response: `{`, want: 500},
		{name: "database failure", response: `{"id":"payment-1","status":"pending","confirmation":{"type":"redirect","confirmation_url":"https://checkout.test/pay"}}`, saveErr: errors.New("write failed"), want: 500, saves: 1},
		{name: "missing redirect", response: `{"id":"payment-1","status":"pending","confirmation":{"type":"embedded"}}`, want: 500, saves: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			saver := &checkedSaver{t: t, err: tc.saveErr}
			transport := paymentTransport(func(r *http.Request) (*http.Response, error) {
				require.Equal(t, "POST", r.Method)
				require.Equal(t, "api.yookassa.ru", r.URL.Host)
				require.Equal(t, "client-operation", r.Header.Get("Idempotence-Key"))
				account, secret, ok := r.BasicAuth()
				require.True(t, ok)
				require.Equal(t, "shop", account)
				require.Equal(t, "key", secret)
				var payload map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
				require.Equal(t, map[string]any{"value": "49.00", "currency": "RUB"}, payload["amount"])
				require.Equal(t, map[string]any{"user_id": "42"}, payload["metadata"])
				confirmation := payload["confirmation"].(map[string]any)
				expected := strings.TrimRight(tc.returnURL, "/") + "/?payment=check"
				if strings.Contains(tc.returnURL, "?") {
					expected = tc.returnURL + "&payment=check"
				}
				require.Equal(t, expected, confirmation["return_url"])
				if tc.networkErr != nil {
					return nil, tc.networkErr
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.response))}, nil
			})
			client := yookassa.NewClient("shop", "key", opts.WithHTTPClient(http.Client{Transport: transport}))
			handler := auth.New("jwt")(New(slog.New(slog.DiscardHandler), saver, client, tc.returnURL))
			token, err := jwt.NewAccessToken(domain.User{ID: 42}, "jwt", time.Hour)
			require.NoError(t, err)
			req := httptest.NewRequest("POST", "/payments/create", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Idempotency-Key", "client-operation")
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			require.Equal(t, tc.want, res.Code, res.Body.String())
			require.Equal(t, tc.saves, saver.calls)
			if tc.want == 200 {
				require.Contains(t, res.Body.String(), "https://checkout.test/pay")
			}
		})
	}
}
