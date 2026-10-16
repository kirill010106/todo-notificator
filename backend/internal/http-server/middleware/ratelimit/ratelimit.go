package ratelimit

import (
	"github.com/go-chi/httprate"
	"github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
	"github.com/kirill010106/todo-notificator/internal/http-server/middleware/clientip"
	j "github.com/kirill010106/todo-notificator/internal/lib/jwt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Each instance owns an independent budget. User keys must run after auth.
func IP(limit int) func(http.Handler) http.Handler {
	return httprate.Limit(limit, time.Minute, httprate.WithKeyFuncs(func(r *http.Request) (string, error) {
		return ipKey(r), nil
	}))
}

func User(limit int) func(http.Handler) http.Handler {
	return httprate.Limit(limit, time.Minute, httprate.WithKeyFuncs(func(r *http.Request) (string, error) {
		if id, ok := auth.GetUserID(r.Context()); ok {
			return "user:" + strconv.FormatInt(id, 10), nil
		}
		return "ip:" + ipKey(r), nil
	}))
}

func ipKey(r *http.Request) string {
	clone := r.Clone(r.Context())
	clone.RemoteAddr = net.JoinHostPort(clientip.Address(r), "0")
	key, _ := httprate.KeyByIP(clone)
	return key
}

// Invalid credentials retain an IP budget without imposing a shared IP budget
// on authenticated accounts (for example coworkers behind the same NAT).
func InvalidAuth(secret string) func(http.Handler) http.Handler {
	limit := IP(20)
	return func(next http.Handler) http.Handler {
		limited := limit(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			parts := strings.Split(r.Header.Get("Authorization"), " ")
			if len(parts) == 2 && parts[0] == "Bearer" {
				if _, err := j.ParseAccessToken(parts[1], secret); err == nil {
					next.ServeHTTP(w, r)
					return
				}
			}
			limited.ServeHTTP(w, r)
		})
	}
}
