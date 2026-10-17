package account

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMalformedAccountRequestsNeverReachStorage(t *testing.T) {
	h := Handler{}
	for _, body := range []string{`{}`, `{"email":"invalid"}`, `{"email":"user@example.com","extra":true}`} {
		w := httptest.NewRecorder()
		h.Forgot(w, httptest.NewRequest("POST", "/password/forgot", strings.NewReader(body)))
		require.Equal(t, 400, w.Code)
	}
	for _, body := range []string{`{"email":"user@example.com","password":"short"}`, `{"email":"user@example.com","password":"password123","display_name":"` + strings.Repeat("я", 81) + `"}`} {
		w := httptest.NewRecorder()
		h.Register(w, httptest.NewRequest("POST", "/register", strings.NewReader(body)))
		require.Equal(t, 400, w.Code)
	}
}
