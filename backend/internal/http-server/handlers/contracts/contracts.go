package contracts

import (
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/apiresponse"
	"github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
	"github.com/kirill010106/todo-notificator/internal/storage/postgres"
	"net/http"
	"strconv"
)

type Handler struct{ Store *postgres.Storage }

func reply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func problem(w http.ResponseWriter, r *http.Request, err error) {
	value := apiresponse.Problem{Status: 500, Code: apiresponse.InternalError, Message: "Не удалось выполнить запрос"}
	var contract *domain.ContractError
	if errors.As(err, &contract) {
		value.Status, value.Code, value.Message = contract.Status, apiresponse.Code(contract.Code), contract.Message
	}
	// This family historically omitted ContractError.Fields; retain its empty object.
	apiresponse.Write(w, r, value, apiresponse.Compatibility{})
}
func (h Handler) Focus(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.GetUserID(r.Context())
	value, err := h.Store.GetFocusState(r.Context(), id)
	if err != nil {
		problem(w, r, err)
		return
	}
	reply(w, 200, map[string]any{"status": "OK", "focus": value})
}
func (h Handler) Resume(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.GetUserID(r.Context())
	if err := h.Store.ResumeFocus(r.Context(), id); err != nil {
		problem(w, r, err)
		return
	}
	reply(w, 200, map[string]any{"status": "OK"})
}
func (h Handler) FinishRest(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.GetUserID(r.Context())
	if err := h.Store.FinishFocusRest(r.Context(), id); err != nil {
		problem(w, r, err)
		return
	}
	reply(w, 200, map[string]any{"status": "OK"})
}
func (h Handler) Claim(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.GetUserID(r.Context())
	code := chi.URLParam(r, "code")
	if code == "" {
		code = "daily_chest"
	}
	if err := h.Store.ClaimDailyReward(r.Context(), id, code); err != nil {
		problem(w, r, err)
		return
	}
	reply(w, 200, map[string]any{"status": "OK"})
}
func (h Handler) Settings(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.GetUserID(r.Context())
	var request struct {
		Timezone string `json:"timezone"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&request); err != nil {
		apiresponse.Write(w, r, apiresponse.Problem{Status: 400, Code: apiresponse.ValidationError, Message: "Некорректный запрос"}, apiresponse.Compatibility{})
		return
	}
	if err := h.Store.SetTimezone(r.Context(), id, request.Timezone); err != nil {
		problem(w, r, err)
		return
	}
	reply(w, 200, map[string]any{"status": "OK", "timezone": request.Timezone})
}
func (h Handler) Today(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.GetUserID(r.Context())
	values := map[string]int{"limit": 20, "today_offset": 0, "overdue_offset": 0}
	for name, defaultValue := range values {
		raw := r.URL.Query().Get(name)
		if raw == "" {
			values[name] = defaultValue
			continue
		}
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 || (name == "limit" && (value < 1 || value > 100)) {
			apiresponse.Write(w, r, apiresponse.Problem{Status: 400, Code: apiresponse.ValidationError, Message: "Некорректная пагинация"}, apiresponse.Compatibility{})
			return
		}
		values[name] = value
	}
	result, err := h.Store.GetToday(r.Context(), id, r.URL.Query().Get("timezone"), values["limit"], values["today_offset"], values["overdue_offset"])
	if err != nil {
		problem(w, r, err)
		return
	}
	reply(w, 200, result)
}

func (h Handler) Shop(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.GetUserID(r.Context())
	wallet, ledger, err := h.Store.GetShop(r.Context(), user)
	if err != nil {
		problem(w, r, err)
		return
	}
	reply(w, 200, map[string]any{"status": "OK", "wallet": wallet, "catalog": domain.ShopCatalog, "ledger": ledger})
}
func (h Handler) ShopAction(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.GetUserID(r.Context())
	var request domain.ShopAction
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		apiresponse.Write(w, r, apiresponse.Problem{Status: 400, Code: apiresponse.ValidationError, Message: "Некорректное действие магазина"}, apiresponse.Compatibility{StatusStyle: apiresponse.OmitStatus})
		return
	}
	wallet, err := h.Store.ShopAction(r.Context(), user, request)
	if err != nil {
		problem(w, r, err)
		return
	}
	reply(w, 200, map[string]any{"status": "OK", "wallet": wallet})
}
func (h Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.GetUserID(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		apiresponse.Write(w, r, apiresponse.Problem{Status: 400, Code: apiresponse.ValidationError, Message: "Некорректный номер сессии"}, apiresponse.Compatibility{StatusStyle: apiresponse.OmitStatus})
		return
	}
	if err := h.Store.CancelPomodoroSession(r.Context(), user, id); err != nil {
		problem(w, r, err)
		return
	}
	reply(w, 200, map[string]any{"status": "OK"})
}
