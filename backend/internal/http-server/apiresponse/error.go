// Package apiresponse renders complete JSON errors without capturing responses.
// Callers choose public messages and log internal causes at the handling boundary.
package apiresponse

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/kirill010106/todo-notificator/internal/domain"
)

// Problem describes a public error. Message must be safe to expose to clients.
type Problem struct {
	Status  int
	Code    Code
	Message string
	Fields  map[string]string
	// ActiveSession preserves the existing start-command conflict response.
	ActiveSession *domain.PomodoroSession
}

// Response is the complete wire DTO. Status has existing string, numeric and
// absent variants; ValidationErrors is an optional legacy alias of FieldErrors.
type Response struct {
	Status           any                     `json:"status,omitempty"`
	Code             Code                    `json:"code"`
	Message          string                  `json:"message"`
	Error            string                  `json:"error"`
	RequestID        string                  `json:"request_id"`
	FieldErrors      map[string]string       `json:"field_errors"`
	ValidationErrors *map[string]string      `json:"validation_errors,omitempty"`
	ActiveSession    *domain.PomodoroSession `json:"active_session,omitempty"`
}

type StatusStyle uint8

const (
	StringStatus  StatusStyle = iota // "Error", the default for existing handlers
	NumericStatus                    // HTTP status, used by helpers.SendError
	OmitStatus                       // e.g. rate limiting and shop input errors
)

// Compatibility preserves observable legacy variants during handler migration.
// The zero value emits status:"Error" and field_errors:{} without validation_errors.
// PreserveNullFields applies only to nil Fields; an empty map remains an object.
type Compatibility struct {
	StatusStyle        StatusStyle
	PreserveNullFields bool
	ValidationErrors   bool
}

// FromError supports wrapped ContractError without guessing from error text.
// ContractError messages are public by convention. Other causes remain private;
// callers retain the original error for logging rather than logging it here too.
func FromError(err error) Problem {
	var contract *domain.ContractError
	if errors.As(err, &contract) && contract != nil {
		return Problem{Status: contract.Status, Code: Code(contract.Code), Message: contract.Message, Fields: contract.Fields}
	}
	return Problem{Status: http.StatusInternalServerError, Code: InternalError, Message: "internal server error"}
}

// Write emits an error before a response has started. RequestID middleware must
// run first. Codes and HTTP status are explicit; messages never select a code.
// Headers such as Retry-After are retained. Webhooks keep their separate writer.
func Write(w http.ResponseWriter, r *http.Request, problem Problem, compatibility Compatibility) {
	fields := problem.Fields
	if fields == nil && !compatibility.PreserveNullFields {
		fields = map[string]string{}
	}
	payload := Response{
		Code: problem.Code, Message: problem.Message, Error: problem.Message,
		RequestID: middleware.GetReqID(r.Context()), FieldErrors: fields,
		ActiveSession: problem.ActiveSession,
	}
	switch compatibility.StatusStyle {
	case StringStatus:
		payload.Status = "Error"
	case NumericStatus:
		payload.Status = problem.Status
	}
	if compatibility.ValidationErrors {
		payload.ValidationErrors = &fields
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(problem.Status)
	_ = json.NewEncoder(w).Encode(payload)
}
