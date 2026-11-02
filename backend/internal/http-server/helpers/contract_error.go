package helpers

import (
	"errors"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/apiresponse"
	"net/http"
)

func RenderContractError(w http.ResponseWriter, r *http.Request, err error) bool {
	var problem *domain.ContractError
	if !errors.As(err, &problem) {
		return false
	}
	apiresponse.Write(w, r, apiresponse.FromError(err), apiresponse.Compatibility{PreserveNullFields: true})
	return true
}
