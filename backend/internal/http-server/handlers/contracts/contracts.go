package contracts

import (
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/kirill010106/todo-notificator/internal/domain"
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
func problem(w http.ResponseWriter, err error) {
	var value *domain.ContractError
	if errors.As(err, &value) {
		reply(w, value.Status, map[string]any{"status": "Error", "code": value.Code, "message": value.Message})
	} else {
		reply(w, 500, map[string]any{"status": "Error", "code": "INTERNAL_ERROR", "message": "Не удалось выполнить запрос"})
	}
}
func (h Handler) Focus(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.GetUserID(r.Context())
	value, err := h.Store.GetFocusState(r.Context(), id)
	if err != nil {
		problem(w, err)
		return
	}
	reply(w, 200, map[string]any{"status": "OK", "focus": value})
}
func (h Handler) Resume(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.GetUserID(r.Context())
	if err := h.Store.ResumeFocus(r.Context(), id); err != nil {
		problem(w, err)
		return
	}
	reply(w, 200, map[string]any{"status": "OK"})
}
func (h Handler) FinishRest(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.GetUserID(r.Context())
	if err := h.Store.FinishFocusRest(r.Context(), id); err != nil {
		problem(w, err)
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
		problem(w, err)
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
		reply(w, 400, map[string]any{"status": "Error", "code": "VALIDATION_ERROR", "message": "Некорректный запрос"})
		return
	}
	if err := h.Store.SetTimezone(r.Context(), id, request.Timezone); err != nil {
		problem(w, err)
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
			reply(w, 400, map[string]any{"status": "Error", "code": "VALIDATION_ERROR", "message": "Некорректная пагинация"})
			return
		}
		values[name] = value
	}
	result, err := h.Store.GetToday(r.Context(), id, r.URL.Query().Get("timezone"), values["limit"], values["today_offset"], values["overdue_offset"])
	if err != nil {
		problem(w, err)
		return
	}
	reply(w, 200, result)
}
