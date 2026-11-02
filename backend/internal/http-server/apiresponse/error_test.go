package apiresponse_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/apiresponse"
	"github.com/stretchr/testify/require"
)

func TestWrite(t *testing.T) {
	for _, test := range []struct {
		name   string
		compat apiresponse.Compatibility
		fields map[string]string
		want   string
	}{
		{"default", apiresponse.Compatibility{}, nil, `"status":"Error","field_errors":{}`},
		{"numeric", apiresponse.Compatibility{StatusStyle: apiresponse.NumericStatus}, nil, `"status":400,"field_errors":{}`},
		{"absent", apiresponse.Compatibility{StatusStyle: apiresponse.OmitStatus}, nil, `"field_errors":{}`},
		{"null", apiresponse.Compatibility{PreserveNullFields: true}, nil, `"status":"Error","field_errors":null`},
		{"empty_object", apiresponse.Compatibility{PreserveNullFields: true}, map[string]string{}, `"status":"Error","field_errors":{}`},
		{"validation_alias", apiresponse.Compatibility{ValidationErrors: true}, map[string]string{"reminders.0": "invalid offset"}, `"status":"Error","field_errors":{"reminders.0":"invalid offset"},"validation_errors":{"reminders.0":"invalid offset"}`},
		{"empty_alias", apiresponse.Compatibility{ValidationErrors: true}, nil, `"status":"Error","field_errors":{},"validation_errors":{}`},
		{"null_alias", apiresponse.Compatibility{PreserveNullFields: true, ValidationErrors: true}, nil, `"status":"Error","field_errors":null,"validation_errors":null`},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", "60")
				apiresponse.Write(w, r, apiresponse.Problem{Status: 400, Code: apiresponse.ValidationError, Message: "check input", Fields: test.fields}, test.compat)
			}))
			r := httptest.NewRequest("POST", "/api/v1/tasks", nil)
			r.Header.Set("X-Request-Id", "current-request")
			handler.ServeHTTP(response, r)
			require.Equal(t, 400, response.Code)
			require.Equal(t, "application/json", response.Header().Get("Content-Type"))
			require.Equal(t, "60", response.Header().Get("Retry-After"))
			require.JSONEq(t, `{"code":"VALIDATION_ERROR","message":"check input","error":"check input","request_id":"current-request",`+test.want+`}`, response.Body.String())
		})
	}
}

func TestFromError(t *testing.T) {
	contract := &domain.ContractError{Status: 403, Code: "REMINDER_LIMIT_REACHED", Message: "public message", Fields: map[string]string{"reminders": "too many"}}
	problem := apiresponse.FromError(fmt.Errorf("private wrapper: %w", contract))
	require.Equal(t, apiresponse.Problem{Status: 403, Code: apiresponse.ReminderLimitReached, Message: "public message", Fields: contract.Fields}, problem)

	// Non-catalog domain codes remain intact while migration is incremental.
	require.Equal(t, apiresponse.Code("FOCUS_NOT_ON_BREAK"), apiresponse.FromError(&domain.ContractError{Status: 409, Code: "FOCUS_NOT_ON_BREAK", Message: "no break"}).Code)
	var typedNil *domain.ContractError
	for _, err := range []error{errors.New("private database credentials"), fmt.Errorf("private wrapper: %w", errors.New("private cause")), nil, typedNil} {
		t.Run(fmt.Sprintf("%T/%v", err, err == nil), func(t *testing.T) {
			problem := apiresponse.FromError(err)
			require.Equal(t, apiresponse.Problem{Status: 500, Code: apiresponse.InternalError, Message: "internal server error"}, problem)
			response := httptest.NewRecorder()
			apiresponse.Write(response, httptest.NewRequest("GET", "/", nil), problem, apiresponse.Compatibility{})
			require.NotContains(t, response.Body.String(), "private")
			require.Equal(t, 500, response.Code)
		})
	}
}

func TestCodeDoesNotDependOnMessage(t *testing.T) {
	for _, test := range []struct {
		status int
		code   apiresponse.Code
	}{
		{403, apiresponse.CategoryLimitReached},
		{409, apiresponse.BreakAlreadyUsed},
	} {
		for _, message := range []string{"reworded message", "Новая формулировка", "extra break is already used; не более 3 категорий"} {
			t.Run(string(test.code)+"/"+message, func(t *testing.T) {
				response := httptest.NewRecorder()
				handler := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					apiresponse.Write(w, r, apiresponse.Problem{Status: test.status, Code: test.code, Message: message}, apiresponse.Compatibility{})
				}))
				handler.ServeHTTP(response, httptest.NewRequest("GET", "/", nil))
				var payload apiresponse.Response
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
				require.Equal(t, test.status, response.Code)
				require.Equal(t, test.code, payload.Code)
				require.Equal(t, message, payload.Message)
				require.NotEmpty(t, payload.RequestID)
			})
		}
	}
}

// This tests representation compatibility, not the mapping of storage errors or
// route wiring. The unchanged real-router baseline tests cover those separately.
func TestWriteRepresentsBaselineJSONErrors(t *testing.T) {
	for _, filename := range []string{"error_contract_baseline.json", "error_contract_postgres_baseline.json"} {
		data, err := os.ReadFile("../router/testdata/" + filename)
		require.NoError(t, err)
		var fixtures map[string]struct {
			Status int             `json:"status"`
			JSON   json.RawMessage `json:"json"`
		}
		require.NoError(t, json.Unmarshal(data, &fixtures))
		for name, fixture := range fixtures {
			if fixture.Status < 400 || len(fixture.JSON) == 0 {
				continue // Success and text/empty webhook responses are separate contracts.
			}
			t.Run(filename+"/"+name, func(t *testing.T) {
				var want apiresponse.Response
				require.NoError(t, json.Unmarshal(fixture.JSON, &want))
				compat := apiresponse.Compatibility{PreserveNullFields: want.FieldErrors == nil, ValidationErrors: want.ValidationErrors != nil}
				switch want.Status.(type) {
				case float64:
					compat.StatusStyle = apiresponse.NumericStatus
				case nil:
					compat.StatusStyle = apiresponse.OmitStatus
				}
				handler := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					apiresponse.Write(w, r, apiresponse.Problem{Status: fixture.Status, Code: want.Code, Message: want.Message, Fields: want.FieldErrors}, compat)
				}))
				r := httptest.NewRequest("GET", "/api/v1/tasks", nil)
				r.Header.Set("X-Request-Id", "error-baseline-request")
				response := httptest.NewRecorder()
				middleware.RequestID(handler).ServeHTTP(response, r)
				require.Equal(t, fixture.Status, response.Code)
				require.Equal(t, "application/json", response.Header().Get("Content-Type"))
				require.JSONEq(t, string(fixture.JSON), response.Body.String())
			})
		}
	}
}
