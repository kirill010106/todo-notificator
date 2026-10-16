package contracts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
)

type Executor interface {
	ExecuteOperation(context.Context, int64, string, string, []byte, func(context.Context) domain.OperationResponse) (domain.OperationResponse, error)
}

func Operation(store Executor, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.GetUserID(r.Context())
		if !ok {
			writeError(w, 401, "UNAUTHORIZED", "Войдите в аккаунт")
			return
		}
		key := r.Header.Get("Idempotency-Key")
		if len(key) > 128 {
			writeError(w, 400, "INVALID_IDEMPOTENCY_KEY", "Ключ не должен превышать 128 символов")
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
		if err != nil || len(body) > 1<<20 {
			writeError(w, 413, "BODY_TOO_LARGE", "Слишком большой запрос")
			return
		}
		// Validate the current task wire shape before replaying an old operation.
		// Cached pre-migration responses must not revive removed write fields.
		if strings.HasPrefix(r.URL.Path, "/api/v1/tasks") && (r.Method == "POST" || r.Method == "PATCH") {
			if err := domain.ValidateReminderFields(body); err != nil {
				var problem *domain.ContractError
				if errors.As(err, &problem) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(problem.Status)
					_ = json.NewEncoder(w).Encode(map[string]any{"status": "Error", "code": problem.Code, "message": problem.Message, "error": problem.Message, "field_errors": problem.Fields})
				} else {
					writeError(w, 400, "INVALID_TASK_REQUEST", err.Error())
				}
				return
			}
		}
		result, err := store.ExecuteOperation(r.Context(), userID, key, r.Method+" "+r.URL.RequestURI(), body, func(ctx context.Context) domain.OperationResponse {
			recorder := httptest.NewRecorder()
			request := r.WithContext(ctx)
			request.Body = io.NopCloser(bytes.NewReader(body))
			handler(recorder, request)
			return domain.OperationResponse{Body: recorder.Body.Bytes(), Status: recorder.Code}
		})
		if err != nil {
			var problem *domain.ContractError
			if errors.As(err, &problem) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(problem.Status)
				_ = json.NewEncoder(w).Encode(map[string]any{"status": "Error", "code": problem.Code, "message": problem.Message, "error": problem.Message, "field_errors": problem.Fields})
			} else {
				writeError(w, 500, "INTERNAL_ERROR", "Не удалось сохранить действие")
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(result.Status)
		_, _ = w.Write(result.Body)
	}
}
func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "Error", "code": code, "message": message, "error": message})
}
func Errors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/webhooks/") {
			next.ServeHTTP(w, r)
			return
		}
		recorder := httptest.NewRecorder()
		next.ServeHTTP(recorder, r)
		for key, values := range recorder.Header() {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		body := recorder.Body.Bytes()
		if recorder.Code >= 400 {
			payload := map[string]any{}
			_ = json.Unmarshal(body, &payload)
			message, _ := payload["message"].(string)
			if message == "" {
				message, _ = payload["error"].(string)
			}
			if message == "" {
				message = http.StatusText(recorder.Code)
			}
			code, _ := payload["code"].(string)
			if code == "" {
				code = defaultCode(recorder.Code, message)
			}
			payload["code"] = code
			payload["message"] = message
			payload["error"] = message
			payload["request_id"] = middleware.GetReqID(r.Context())
			if fields, ok := payload["validation_errors"]; ok {
				payload["field_errors"] = fields
			} else if _, exists := payload["field_errors"]; !exists {
				payload["field_errors"] = map[string]string{}
			}
			body, _ = json.Marshal(payload)
			w.Header().Set("Content-Type", "application/json")
		}
		w.Header().Del("Content-Length")
		w.WriteHeader(recorder.Code)
		_, _ = w.Write(body)
	})
}
func defaultCode(status int, message string) string {
	switch {
	case strings.Contains(message, "extra break is already used"):
		return "BREAK_ALREADY_USED"
	case strings.Contains(message, "активных напоминаний"):
		return "REMINDER_LIMIT_REACHED"
	case strings.Contains(message, "не более 3 категорий"):
		return "CATEGORY_LIMIT_REACHED"
	}
	switch status {
	case 400:
		return "VALIDATION_ERROR"
	case 401:
		return "UNAUTHORIZED"
	case 403:
		return "FORBIDDEN"
	case 404:
		return "NOT_FOUND"
	case 405:
		return "METHOD_NOT_ALLOWED"
	case 409:
		return "CONFLICT"
	case 429:
		return "RATE_LIMITED"
	default:
		return "INTERNAL_ERROR"
	}
}
