package health

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/render"
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
)

type DBHealthChecker interface {
	PingContext(context.Context) error
}

func New(log *slog.Logger, db DBHealthChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.health.New"
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		err := db.PingContext(ctx)
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			render.JSON(w, r, resp.Response{
				Status: "Error",
				Error:  "database is unreachable",
			})
			return
		}

		log = log.With(
			slog.String("op", op),
			slog.String("method", "GET"))

		render.JSON(w, r, resp.OK())
	}
}
