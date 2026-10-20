package account

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/kirill010106/todo-notificator/internal/config"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/middleware/auth"
	"github.com/kirill010106/todo-notificator/internal/lib/jwt"
	"github.com/kirill010106/todo-notificator/internal/storage"
	"github.com/kirill010106/todo-notificator/internal/storage/postgres"
	"golang.org/x/crypto/bcrypt"
)

type Handler struct {
	Store  *postgres.Storage
	Config *config.Config
	Events interface {
		LogEvent(int64, string, int64, map[string]any)
	}
}

func reply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, err error) {
	status, code, message := 500, "INTERNAL_ERROR", "Не удалось выполнить запрос"
	var c *domain.ContractError
	if errors.As(err, &c) {
		status, code, message = c.Status, c.Code, c.Message
	}
	if errors.Is(err, storage.ErrUserExists) {
		status, code, message = 409, "EMAIL_EXISTS", "Этот email уже используется"
	}
	reply(w, status, map[string]any{"status": "Error", "code": code, "message": message, "error": message})
}
func decode(w http.ResponseWriter, r *http.Request, value any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		fail(w, &domain.ContractError{Status: 400, Code: "VALIDATION_ERROR", Message: "Некорректный запрос"})
		return false
	}
	return true
}
func validEmail(w http.ResponseWriter, email string) bool {
	if validator.New().Var(email, "required,email,max=254") != nil {
		fail(w, &domain.ContractError{Status: 400, Code: "VALIDATION_ERROR", Message: "Введите корректный email"})
		return false
	}
	return true
}
func (h Handler) tokens(w http.ResponseWriter, u domain.User, refresh string, extra map[string]any) {
	access, err := jwt.NewAccessToken(u, h.Config.AppSecret, h.Config.AccessTokenTTL)
	if err != nil {
		fail(w, err)
		return
	}
	if extra == nil {
		extra = map[string]any{}
	}
	extra["status"] = "OK"
	extra["access_token"] = access
	extra["refresh_token"] = refresh
	extra["expires_in"] = int64(h.Config.AccessTokenTTL.Seconds())
	reply(w, 200, extra)
}
func (h Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
	}
	if !decode(w, r, &req) {
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	if !validEmail(w, req.Email) {
		return
	}
	if err := postgres.ValidatePassword(req.Password); err != nil {
		fail(w, err)
		return
	}
	if err := postgres.ValidateDisplayName(req.DisplayName); err != nil {
		fail(w, err)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		fail(w, err)
		return
	}
	verification, err := jwt.NewRefreshToken()
	if err != nil {
		fail(w, err)
		return
	}
	refresh, err := jwt.NewRefreshToken()
	if err != nil {
		fail(w, err)
		return
	}
	u, err := h.Store.RegisterAccount(r.Context(), req.Email, req.DisplayName, hash, verification, refresh, time.Now().Add(h.Config.RefreshTokenTTL))
	if err != nil {
		fail(w, err)
		return
	}
	if h.Events != nil {
		h.Events.LogEvent(u.ID, "USER_REGISTERED", u.ID, map[string]any{"email": u.Email})
	}
	h.tokens(w, u, refresh, map[string]any{"user_id": u.ID})
}
func (h Handler) Verify(w http.ResponseWriter, r *http.Request) {
	if err := h.Store.ConfirmEmail(r.Context(), r.URL.Query().Get("token")); err != nil {
		fail(w, err)
		return
	}
	reply(w, 200, map[string]any{"status": "OK"})
}
func (h Handler) Profile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DisplayName *string `json:"display_name"`
		Timezone    *string `json:"timezone"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.DisplayName == nil && req.Timezone == nil {
		fail(w, &domain.ContractError{Status: 400, Code: "VALIDATION_ERROR", Message: "Укажите имя или часовой пояс"})
		return
	}
	id, _ := auth.GetUserID(r.Context())
	if err := h.Store.UpdateAccountProfile(r.Context(), id, req.DisplayName, req.Timezone); err != nil {
		fail(w, err)
		return
	}
	result := map[string]any{"status": "OK"}
	if req.Timezone != nil {
		result["timezone"] = *req.Timezone
	}
	if req.DisplayName != nil {
		result["display_name"] = strings.TrimSpace(*req.DisplayName)
	}
	reply(w, 200, result)
}
func (h Handler) Change(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email           *string `json:"email"`
		CurrentPassword string  `json:"current_password"`
		NewPassword     string  `json:"new_password"`
	}
	if !decode(w, r, &req) {
		return
	}
	var hash []byte
	if strings.HasSuffix(r.URL.Path, "/email") {
		if req.Email == nil {
			validEmail(w, "")
			return
		}
		v := strings.TrimSpace(*req.Email)
		req.Email = &v
		if !validEmail(w, v) {
			return
		}
	} else {
		req.Email = nil
		if err := postgres.ValidatePassword(req.NewPassword); err != nil {
			fail(w, err)
			return
		}
		var err error
		hash, err = bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			fail(w, err)
			return
		}
	}
	verification, err := jwt.NewRefreshToken()
	if err != nil {
		fail(w, err)
		return
	}
	refresh, err := jwt.NewRefreshToken()
	if err != nil {
		fail(w, err)
		return
	}
	id, _ := auth.GetUserID(r.Context())
	u, changed, err := h.Store.ChangeCredentials(r.Context(), id, auth.GetAuthVersion(r.Context()), req.CurrentPassword, req.Email, hash, verification, refresh, time.Now().Add(h.Config.RefreshTokenTTL))
	if err != nil {
		fail(w, err)
		return
	}
	if !changed {
		reply(w, 200, map[string]any{"status": "OK", "changed": false})
		return
	}
	h.tokens(w, u, refresh, map[string]any{"changed": true})
}
func (h Handler) Forgot(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if !decode(w, r, &req) {
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	if !validEmail(w, req.Email) {
		return
	}
	token, err := jwt.NewRefreshToken()
	if err != nil {
		fail(w, err)
		return
	}
	if err = h.Store.RequestPasswordReset(r.Context(), req.Email, token); err != nil {
		fail(w, err)
		return
	}
	reply(w, 202, map[string]any{"status": "OK", "message": "Если аккаунт с такой почтой существует, мы отправим ссылку для восстановления"})
}
func (h Handler) Reset(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := postgres.ValidatePassword(req.NewPassword); err != nil {
		fail(w, err)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		fail(w, err)
		return
	}
	if err = h.Store.ResetPassword(r.Context(), req.Token, hash); err != nil {
		fail(w, err)
		return
	}
	reply(w, 200, map[string]any{"status": "OK"})
}
