package contracts

import (
	"bytes"
	"context"
	"errors"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/apiresponse"
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
			writeError(w, r, 401, apiresponse.Unauthorized, "Войдите в аккаунт")
			return
		}
		key := r.Header.Get("Idempotency-Key")
		if len(key) > 128 {
			writeError(w, r, 400, apiresponse.InvalidIdempotencyKey, "Ключ не должен превышать 128 символов")
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
		if err != nil || len(body) > 1<<20 {
			writeError(w, r, 413, apiresponse.BodyTooLarge, "Слишком большой запрос")
			return
		}
		// Validate the current task wire shape before replaying an old operation.
		// Cached pre-migration responses must not revive removed write fields.
		if strings.HasPrefix(r.URL.Path, "/api/v1/tasks") && (r.Method == "POST" || r.Method == "PATCH") {
			if err := domain.ValidateReminderFields(body); err != nil {
				var problem *domain.ContractError
				if errors.As(err, &problem) {
					apiresponse.Write(w, r, apiresponse.FromError(err), apiresponse.Compatibility{PreserveNullFields: true})
				} else {
					writeError(w, r, 400, apiresponse.InvalidTaskRequest, err.Error())
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
				apiresponse.Write(w, r, apiresponse.FromError(err), apiresponse.Compatibility{PreserveNullFields: true})
			} else {
				writeError(w, r, 500, apiresponse.InternalError, "Не удалось сохранить действие")
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(result.Status)
		_, _ = w.Write(result.Body)
	}
}
func writeError(w http.ResponseWriter, r *http.Request, status int, code apiresponse.Code, message string) {
	apiresponse.Write(w, r, apiresponse.Problem{Status: status, Code: code, Message: message}, apiresponse.Compatibility{})
}
