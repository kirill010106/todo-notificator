package helpers

import (
	"encoding/json"
	"net/http"

	"github.com/kirill010106/todo-notificator/internal/domain"
)

func SendError(w http.ResponseWriter, status int, message string, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(domain.APIError{
		Status:  status,
		Message: message,
		Code:    code,
	})
}
