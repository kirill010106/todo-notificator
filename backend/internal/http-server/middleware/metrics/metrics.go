package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Счетчик общего количества HTTP-запросов
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests processed, partitioned by status code, method and path pattern.",
		},
		[]string{"method", "path", "status"},
	)

	// Гистограмма времени выполнения запросов
	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latencies in seconds.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"method", "path"},
	)
)

// New возвращает middleware для сбора метрик HTTP-запросов в Prometheus
func New() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Оборачиваем ResponseWriter стандартной оберткой Chi, чтобы перехватить статус-код
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			// Определяем шаблон пути (например, /tasks/{task_id} вместо конкретного id)
			pattern := ""
			if routeCtx := chi.RouteContext(r.Context()); routeCtx != nil {
				pattern = routeCtx.RoutePattern()
			}
			if pattern == "" {
				pattern = r.URL.Path
			}

			duration := time.Since(start).Seconds()
			status := strconv.Itoa(ww.Status())

			// Фиксируем метрики
			httpRequestsTotal.WithLabelValues(r.Method, pattern, status).Inc()
			httpRequestDuration.WithLabelValues(r.Method, pattern).Observe(duration)
		})
	}
}
