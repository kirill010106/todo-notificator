package bootstrap

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/render"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/http-server/helpers"
	resp "github.com/kirill010106/todo-notificator/internal/lib/api/response"
	"github.com/kirill010106/todo-notificator/internal/lib/sl"
)

type DataProvider interface {
	GetUserByID(ctx context.Context, userID int64) (*domain.User, error)
	GetCategories(ctx context.Context, userID int64) ([]domain.Category, error)
	GetTasks(ctx context.Context, userID int64, limit, offset int, filter domain.TaskFilter) ([]domain.Task, int, error)
	GetUserStats(ctx context.Context, userID int64) (domain.UserStats, error)
	GetGamificationProfile(ctx context.Context, userID int64) (domain.GamificationProfile, error)
	GetActivePomodoroSession(ctx context.Context, userID int64) (*domain.PomodoroSession, error)
	CountCategories(ctx context.Context, userID int64) (int, error)
	CountActiveReminders(ctx context.Context, userID int64) (int, error)
}

type UserInfo struct {
	ID                   int64  `json:"id"`
	Email                string `json:"email"`
	IsVerified           bool   `json:"is_verified"`
	IsPremium            bool   `json:"is_premium"`
	CategoriesCount      int    `json:"categories_count"`
	ActiveRemindersCount int    `json:"active_reminders_count"`
	CategoriesLimit      int    `json:"categories_limit"`
	RemindersLimit       int    `json:"reminders_limit"`
}

type Pagination struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}

type Response struct {
	resp.Response
	User           UserInfo                    `json:"user"`
	Categories     []domain.Category           `json:"categories"`
	Tasks          []domain.Task               `json:"tasks"`
	Pagination     Pagination                  `json:"pagination"`
	Stats          *domain.UserStats           `json:"stats,omitempty"`
	Gamification   *domain.GamificationProfile `json:"gamification,omitempty"`
	ActivePomodoro *domain.PomodoroSession     `json:"active_pomodoro,omitempty"`
}

func New(log *slog.Logger, provider DataProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.profile.bootstrap.New"

		log, userID, ok := helpers.LoggerWithAuth(w, r, log, op)
		if !ok {
			return
		}

		ctx := r.Context()

		user, err := provider.GetUserByID(ctx, userID)
		if err != nil {
			log.Error("failed to get user", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("failed to load user data"))
			return
		}

		categories, err := provider.GetCategories(ctx, userID)
		if err != nil {
			log.Warn("failed to get categories, returning empty list", sl.Err(err))
			categories = []domain.Category{}
		}

		const defaultLimit = 20
		tasks, total, err := provider.GetTasks(ctx, userID, defaultLimit, 0, domain.TaskFilter{})
		if err != nil {
			log.Warn("failed to get tasks, returning empty list", sl.Err(err))
			tasks = []domain.Task{}
			total = 0
		}

		var statsPtr *domain.UserStats
		if stats, err := provider.GetUserStats(ctx, userID); err == nil {
			statsPtr = &stats
		}

		var gamificationPtr *domain.GamificationProfile
		if gamification, err := provider.GetGamificationProfile(ctx, userID); err == nil {
			gamificationPtr = &gamification
		} else {
			log.Warn("failed to get gamification profile", sl.Err(err))
		}

		activePomodoro, _ := provider.GetActivePomodoroSession(ctx, userID)

		catCount, _ := provider.CountCategories(ctx, userID)
		remCount, _ := provider.CountActiveReminders(ctx, userID)

		catLimit := 3
		remLimit := 3
		if user.IsPremium {
			catLimit = -1
			remLimit = -1
		}

		render.Status(r, http.StatusOK)
		render.JSON(w, r, Response{
			Response: resp.OK(),
			User: UserInfo{
				ID:                   user.ID,
				Email:                user.Email,
				IsVerified:           user.IsVerified,
				IsPremium:            user.IsPremium,
				CategoriesCount:      catCount,
				ActiveRemindersCount: remCount,
				CategoriesLimit:      catLimit,
				RemindersLimit:       remLimit,
			},
			Categories: categories,
			Tasks:      tasks,
			Pagination: Pagination{
				Limit:  defaultLimit,
				Offset: 0,
				Total:  total,
			},
			Stats:          statsPtr,
			Gamification:   gamificationPtr,
			ActivePomodoro: activePomodoro,
		})
	}
}
