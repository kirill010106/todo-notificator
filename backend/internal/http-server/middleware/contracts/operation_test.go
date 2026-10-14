package contracts

import (
	"context"
	"errors"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
	"github.com/kirill010106/todo-notificator/internal/lib/jwt"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type executorStub struct {
	t     *testing.T
	err   error
	calls int
	seen  bool
}

func (s *executorStub) ExecuteOperation(ctx context.Context, user int64, key, route string, body []byte, invoke func(context.Context) domain.OperationResponse) (domain.OperationResponse, error) {
	s.calls++
	require.Equal(s.t, int64(42), user)
	require.Equal(s.t, "POST /tasks?source=test", route)
	if s.err != nil {
		return domain.OperationResponse{}, s.err
	}
	return invoke(ctx), nil
}

type brokenBody struct{}

func (brokenBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (brokenBody) Close() error             { return nil }
func TestCommandMiddlewareRejectsInvalidRequestsBeforeExecution(t *testing.T) {
	for _, tc := range []struct {
		name, key   string
		body        io.ReadCloser
		err         error
		want, calls int
		auth        bool
	}{
		{name: "anonymous", want: 401},
		{name: "oversized key", key: strings.Repeat("x", 129), want: 400, auth: true},
		{name: "body too large", body: io.NopCloser(strings.NewReader(strings.Repeat("x", (1<<20)+1))), want: 413, auth: true},
		{name: "body read failed", body: brokenBody{}, want: 413, auth: true},
		{name: "conflict", err: &domain.ContractError{Code: "IDEMPOTENCY_KEY_REUSED", Message: "conflict", Status: 409}, want: 409, calls: 1, auth: true},
		{name: "storage failed", err: errors.New("database failed"), want: 500, calls: 1, auth: true},
		{name: "success", want: 201, calls: 1, auth: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &executorStub{t: t, err: tc.err}
			req := httptest.NewRequest("POST", "/tasks?source=test", strings.NewReader(`{"title":"Work"}`))
			if tc.body != nil {
				req.Body = tc.body
			}
			req.Header.Set("Idempotency-Key", tc.key)
			handler := Operation(stub, func(w http.ResponseWriter, r *http.Request) {
				stub.seen = true
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.JSONEq(t, `{"title":"Work"}`, string(body))
				w.WriteHeader(201)
				_, _ = w.Write([]byte(`{"id":7}`))
			})
			res := httptest.NewRecorder()
			if tc.auth {
				token, err := jwt.NewAccessToken(domain.User{ID: 42}, "jwt", time.Hour)
				require.NoError(t, err)
				req.Header.Set("Authorization", "Bearer "+token)
				auth.New("jwt")(handler).ServeHTTP(res, req)
			} else {
				handler(res, req)
			}
			require.Equal(t, tc.want, res.Code, res.Body.String())
			require.Equal(t, tc.calls, stub.calls)
			require.Equal(t, tc.want == 201, stub.seen)
		})
	}
}
