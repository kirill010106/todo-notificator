package helpers

import (
	"github.com/kirill010106/todo-notificator/internal/http-server/apiresponse"
	"net/http"
)

func SendError(w http.ResponseWriter, r *http.Request, status int, message string, code apiresponse.Code) {
	apiresponse.Write(w, r, apiresponse.Problem{Status: status, Message: message, Code: code}, apiresponse.Compatibility{StatusStyle: apiresponse.NumericStatus})
}
