package helpers

import (
	"errors"
	"github.com/go-chi/render"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"net/http"
)

func RenderContractError(w http.ResponseWriter, r *http.Request, err error) bool {
	var problem *domain.ContractError
	if !errors.As(err, &problem) {
		return false
	}
	render.Status(r, problem.Status)
	render.JSON(w, r, map[string]any{"status": "Error", "code": problem.Code, "error": problem.Message, "message": problem.Message, "field_errors": problem.Fields})
	return true
}
