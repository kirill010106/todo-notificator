package helpers

import (
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/kirill010106/todo-notificator/internal/http-server/apiresponse"
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
)

// RenderValidationError retains the existing field names and validator messages.
// Formatting fields remains shared with the legacy response adapter until its
// remaining consumers are retired; all HTTP serialization goes through Write.
func RenderValidationError(w http.ResponseWriter, r *http.Request, errs validator.ValidationErrors) {
	legacy := resp.ValidationError(errs)
	apiresponse.Write(w, r, apiresponse.Problem{
		Status: http.StatusBadRequest, Code: apiresponse.ValidationError,
		Message: legacy.Error, Fields: legacy.ValidationErrors,
	}, apiresponse.Compatibility{ValidationErrors: len(legacy.ValidationErrors) > 0})
}
