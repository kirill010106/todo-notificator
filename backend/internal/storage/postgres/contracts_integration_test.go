//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	todonotificator "github.com/kirill010106/todo-notificator"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/storage"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestFrontendContracts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	provider, err := testcontainers.ProviderDocker.GetProvider()
	require.NoError(t, err, "Docker is required with -tags integration")
	require.NoError(t, provider.Health(ctx))
	pg, err := tcpostgres.Run(ctx, "postgres:16-alpine", tcpostgres.WithDatabase("contracts"), tcpostgres.WithUsername("postgres"), tcpostgres.WithPassword("postgres"), testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pg.Terminate(context.Background())) })
	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	s, err := New(dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	goose.SetBaseFS(todonotificator.MigrationsFS)
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.Up(s.DB, "migrations"))
	number := 0
	newUser := func() int64 {
		number++
		id, err := s.SaveUser(ctx, fmt.Sprintf("contract%d@test.local", number), []byte("test"))
		require.NoError(t, err)
		return id
	}
	newTask := func(userID int64) int64 {
		id, err := s.SaveTask(ctx, domain.Task{UserID: userID, Title: "Task", Status: "pending"})
		require.NoError(t, err)
		return id
	}
	execute := func(userID int64, key, route, body string, action func(context.Context) error) (domain.OperationResponse, error) {
		return s.ExecuteOperation(ctx, userID, key, route, []byte(body), func(txctx context.Context) domain.OperationResponse {
			if err := action(txctx); err != nil {
				t.Logf("operation rejected: %v", err)
				if problem, ok := err.(*domain.ContractError); ok {
					return domain.OperationResponse{Status: problem.Status, Body: []byte(`{"error":"rejected"}`)}
				}
				return domain.OperationResponse{Status: 500, Body: []byte(`{"error":"failed"}`)}
			}
			return domain.OperationResponse{Status: 200, Body: []byte(`{"status":"OK"}`)}
		})
	}
	t.Run("concurrent replay returns one committed reward and response", func(t *testing.T) {
		user := newUser()
		task := newTask(user)
		status := "done"
		var called atomic.Int64
		var callbacks atomic.Int64
		var wg sync.WaitGroup
		responses := make([]domain.OperationResponse, 8)
		failures := make([]error, 8)
		for i := range responses {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				responses[i], failures[i] = execute(user, "same-key", "PATCH /tasks/1", `{"status":"done"}`, func(c context.Context) error {
					called.Add(1)
					domain.AfterCommit(c, func() { callbacks.Add(1) })
					return s.UpdateTask(c, user, task, domain.TaskUpdate{Status: &status})
				})
			}(i)
		}
		wg.Wait()
		for i := range responses {
			require.NoError(t, failures[i])
			require.Equal(t, 200, responses[i].Status)
			require.JSONEq(t, string(responses[0].Body), string(responses[i].Body))
		}
		require.Equal(t, int64(1), called.Load())
		require.Equal(t, int64(1), callbacks.Load())
		var payload struct {
			Rewards domain.ActionRewards `json:"rewards"`
			Stats   domain.UserStats     `json:"stats"`
		}
		require.NoError(t, json.Unmarshal(responses[0].Body, &payload))
		require.Equal(t, 10, payload.Rewards.TaskXP)
		require.Equal(t, 25, payload.Rewards.AchievementXP)
		require.Equal(t, int64(35), *payload.Stats.Points)
		_, err := execute(user, "same-key", "PATCH /tasks/1", `{"status":"pending"}`, func(context.Context) error { return nil })
		require.Error(t, err)
		require.Equal(t, "IDEMPOTENCY_KEY_REUSED", err.(*domain.ContractError).Code)
	})
	t.Run("handler failure rolls back task and reward together", func(t *testing.T) {
		user := newUser()
		task := newTask(user)
		done := "done"
		response, err := s.ExecuteOperation(ctx, user, "failed-key", "PATCH /tasks/1", nil, func(c context.Context) domain.OperationResponse {
			domain.AfterCommit(c, func() { t.Error("rollback executed a side effect") })
			require.NoError(t, s.UpdateTask(c, user, task, domain.TaskUpdate{Status: &done}))
			return domain.OperationResponse{Status: 500, Body: []byte(`{"error":"response failed"}`)}
		})
		require.NoError(t, err)
		require.Equal(t, 500, response.Status)
		saved, err := s.GetTask(ctx, user, task)
		require.NoError(t, err)
		require.Equal(t, "pending", saved.Status)
		require.False(t, saved.RewardClaimed)
		var operations int
		require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_operations WHERE user_id=$1`, user).Scan(&operations))
		require.Zero(t, operations)
	})
	t.Run("nullable fields are actually cleared", func(t *testing.T) {
		user := newUser()
		deadline := time.Now().Add(time.Hour)
		task, err := s.SaveTask(ctx, domain.Task{UserID: user, Title: "Nullable", Status: "pending", Deadline: &deadline, ReminderAt: &deadline})
		require.NoError(t, err)
		require.NoError(t, s.UpdateTask(ctx, user, task, domain.TaskUpdate{ClearDeadline: true, ClearReminderAt: true, ClearCategoryID: true}))
		saved, err := s.GetTask(ctx, user, task)
		require.NoError(t, err)
		require.Nil(t, saved.Deadline)
		require.Nil(t, saved.ReminderAt)
	})
	t.Run("today counts all matches but pages the lists", func(t *testing.T) {
		user := newUser()
		require.NoError(t, s.SetTimezone(ctx, user, "Europe/Moscow"))
		var now time.Time
		require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT CURRENT_TIMESTAMP`).Scan(&now))
		moscow, err := time.LoadLocation("Europe/Moscow")
		require.NoError(t, err)
		// Keep this pagination test valid even during the minute before midnight.
		if now.In(moscow).Hour() == 23 {
			require.NoError(t, s.SetTimezone(ctx, user, "Pacific/Honolulu"))
		}
		for i := 0; i < 25; i++ {
			future := now.Add(time.Minute)
			_, err := s.SaveTask(ctx, domain.Task{UserID: user, Title: fmt.Sprint(i), Status: "pending", Deadline: &future})
			require.NoError(t, err)
		}
		past := now.Add(-time.Minute)
		_, err = s.SaveTask(ctx, domain.Task{UserID: user, Title: "Overdue", Status: "pending", Deadline: &past})
		require.NoError(t, err)
		newTask(user)
		first, err := s.GetToday(ctx, user, "", 20, 0, 0)
		require.NoError(t, err)
		require.Equal(t, 25, first.Today.Pagination.Total)
		require.Len(t, first.Today.Tasks, 20)
		require.Equal(t, 1, first.Overdue.Pagination.Total)
		second, err := s.GetToday(ctx, user, "", 20, 20, 0)
		require.NoError(t, err)
		require.Len(t, second.Today.Tasks, 5)
		has := false
		undated, total, err := s.GetTasks(ctx, user, 100, 0, domain.TaskFilter{HasDeadline: &has})
		require.NoError(t, err)
		require.Equal(t, 1, total)
		require.Len(t, undated, 1)
	})
	t.Run("daily claim is server verified and cannot be claimed twice", func(t *testing.T) {
		user := newUser()
		newTask(user)
		first, err := execute(user, "daily1", "POST /me/quests/create_task/claim", "", func(c context.Context) error { return s.ClaimDailyReward(c, user, "create_task") })
		require.NoError(t, err)
		require.Equal(t, 200, first.Status)
		repeated, err := execute(user, "daily1", "POST /me/quests/create_task/claim", "", func(c context.Context) error { t.Error("replay invoked action"); return nil })
		require.NoError(t, err)
		require.JSONEq(t, string(first.Body), string(repeated.Body))
		duplicate, err := execute(user, "daily2", "POST /me/quests/create_task/claim", "", func(c context.Context) error { return s.ClaimDailyReward(c, user, "create_task") })
		require.NoError(t, err)
		require.Equal(t, 409, duplicate.Status)
		unavailable, err := execute(user, "chest", "POST /me/rewards/daily-chest/claim", "", func(c context.Context) error { return s.ClaimDailyReward(c, user, "daily_chest") })
		require.NoError(t, err)
		require.Equal(t, 409, unavailable.Status)
		stats, err := s.GetUserStats(ctx, user)
		require.NoError(t, err)
		require.Equal(t, int64(10), *stats.Points)
		require.Zero(t, *stats.CurrentStreak)
	})
	t.Run("daily completion records survive deletion and ignore repeated saves", func(t *testing.T) {
		user := newUser()
		task := newTask(user)
		done, pending := "done", "pending"
		for _, status := range []string{done, done, pending, done} {
			require.NoError(t, s.UpdateTask(ctx, user, task, domain.TaskUpdate{Status: &status}))
		}
		quests, err := s.GetDailyQuests(ctx, user)
		require.NoError(t, err)
		require.Equal(t, 1, quests[2].Current)
		require.NoError(t, s.DeleteTask(ctx, user, task))
		quests, err = s.GetDailyQuests(ctx, user)
		require.NoError(t, err)
		require.Equal(t, 1, quests[0].Current)
		require.Equal(t, 1, quests[2].Current)
	})
	t.Run("bulk results identify changed and skipped tasks", func(t *testing.T) {
		user := newUser()
		first, second := newTask(user), newTask(user)
		response, err := execute(user, "bulk", "POST /tasks/bulk-complete", `{}`, func(c context.Context) error {
			_, err := s.BulkCompleteTasks(c, user, []int64{first, second, first, 999999})
			return err
		})
		require.NoError(t, err)
		var payload struct {
			Result  domain.ActionResult  `json:"result"`
			Rewards domain.ActionRewards `json:"rewards"`
		}
		require.NoError(t, json.Unmarshal(response.Body, &payload))
		require.ElementsMatch(t, []int64{first, second}, payload.Result.ChangedTaskIDs)
		require.Equal(t, []int64{999999}, payload.Result.SkippedTaskIDs)
		require.Equal(t, 20, payload.Rewards.TaskXP)
	})
	t.Run("payment status and premium are persisted atomically", func(t *testing.T) {
		user := newUser()
		_, err := s.CreatePayment(ctx, "canceled-payment", user, "49.00", "RUB", "pending", "Premium")
		require.NoError(t, err)
		pending, err := s.GetLatestPendingPayment(ctx, user)
		require.NoError(t, err)
		require.Equal(t, "canceled-payment", pending)
		owner, err := s.UpdatePaymentStatus(ctx, pending, "canceled")
		require.NoError(t, err)
		require.Equal(t, user, owner)
		pending, err = s.GetLatestPendingPayment(ctx, user)
		require.NoError(t, err)
		require.Empty(t, pending)
		account, err := s.GetUserByID(ctx, user)
		require.NoError(t, err)
		require.False(t, account.IsPremium)
		_, err = s.CreatePayment(ctx, "paid-payment", user, "49.00", "RUB", "pending", "Premium")
		require.NoError(t, err)
		owner, err = s.CompletePaymentAndGrantPremium(ctx, "paid-payment")
		require.NoError(t, err)
		require.Equal(t, user, owner)
		account, err = s.GetUserByID(ctx, user)
		require.NoError(t, err)
		require.True(t, account.IsPremium)
		_, err = s.CompletePaymentAndGrantPremium(ctx, "missing-payment")
		require.ErrorIs(t, err, storage.ErrPaymentNotFound)
	})
	t.Run("parallel refresh rotation has exactly one winner and revocation is scoped", func(t *testing.T) {
		user, other := newUser(), newUser()
		require.NoError(t, s.SaveRefreshToken(ctx, user, "old-refresh", time.Now().Add(time.Hour)))
		require.NoError(t, s.SaveRefreshToken(ctx, other, "other-refresh", time.Now().Add(time.Hour)))
		var successes atomic.Int64
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				account, err := s.RotateRefreshToken(ctx, "old-refresh", fmt.Sprintf("rotated-%d", i), time.Now().Add(time.Hour))
				if err == nil {
					successes.Add(1)
					assert.Equal(t, user, account.ID)
				} else {
					assert.ErrorIs(t, err, storage.ErrRefreshTokenInvalid)
				}
			}(i)
		}
		wg.Wait()
		require.Equal(t, int64(1), successes.Load())
		require.NoError(t, s.DeleteUserRefreshTokens(ctx, user))
		remaining, err := s.GetRefreshToken(ctx, "other-refresh")
		require.NoError(t, err)
		require.Equal(t, other, remaining.UserID)
		require.NoError(t, s.DeleteRefreshToken(ctx, "other-refresh"))
		require.ErrorIs(t, s.DeleteRefreshToken(ctx, "other-refresh"), storage.ErrRefreshTokenInvalid)
		// Exercise a non-UTC application clock regardless of the test runner timezone.
		moscow := time.FixedZone("UTC+3", 3*60*60)
		require.NoError(t, s.SaveRefreshToken(ctx, user, "expired-refresh", time.Now().In(moscow).Add(-time.Hour)))
		_, err = s.GetRefreshToken(ctx, "expired-refresh")
		require.ErrorIs(t, err, storage.ErrRefreshTokenInvalid)
		_, err = s.RotateRefreshToken(ctx, "expired-refresh", "should-not-exist", time.Now().Add(time.Hour))
		require.ErrorIs(t, err, storage.ErrRefreshTokenInvalid)
		// A second connection has a different PostgreSQL timezone as well.
		zoned, err := New(dsn + "&timezone=Europe%2FMoscow")
		require.NoError(t, err)
		defer zoned.Close()
		future := time.Now().In(moscow).Add(10 * time.Minute)
		require.NoError(t, zoned.SaveRefreshToken(ctx, other, "zone-refresh", future))
		valid, err := s.GetRefreshToken(ctx, "zone-refresh")
		require.NoError(t, err)
		require.WithinDuration(t, future, valid.ExpiresAt, time.Second)
		_, err = zoned.RotateRefreshToken(ctx, "zone-refresh", "zone-rotated", future)
		require.NoError(t, err)
		require.NoError(t, zoned.DeleteExpiredRefreshTokens(ctx))
		_, err = s.GetRefreshToken(ctx, "zone-rotated")
		require.NoError(t, err)
		var count int
		require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM refresh_tokens WHERE user_id=$1`, user).Scan(&count))
		require.Zero(t, count)
	})
	t.Run("focus persists breaks and fourth recovery across clients", func(t *testing.T) {
		user := newUser()
		other := &Storage{DB: s.DB}
		idle, err := s.GetFocusState(ctx, user)
		require.NoError(t, err)
		require.Equal(t, "idle", idle.Phase)
		session, err := s.StartPomodoroSession(ctx, user, nil)
		require.NoError(t, err)
		require.NoError(t, s.AddPomodoroBreak(ctx, user, session.ID))
		paused, err := other.GetFocusState(ctx, user)
		require.NoError(t, err)
		require.Equal(t, "extra_break", paused.Phase)
		require.Equal(t, session.ID, paused.Session.ID)
		require.NoError(t, other.ResumeFocus(ctx, user))
		resumed, err := s.GetFocusState(ctx, user)
		require.NoError(t, err)
		require.Equal(t, "focus", resumed.Phase)
		require.Equal(t, session.ID, resumed.Session.ID)
		for i := 0; i < 4; i++ {
			_, err = s.DB.ExecContext(ctx, `UPDATE pomodoro_sessions SET started_at=CURRENT_TIMESTAMP-INTERVAL '25 minutes' WHERE id=$1`, session.ID)
			require.NoError(t, err)
			require.NoError(t, s.FinishPomodoroSession(ctx, user, session.ID, "completed", false))
			recovery, err := other.GetFocusState(ctx, user)
			require.NoError(t, err)
			require.Equal(t, "recovery_break", recovery.Phase)
			expected := 5 * time.Minute
			if i == 3 {
				expected = 15 * time.Minute
			}
			require.InDelta(t, expected.Seconds(), recovery.PhaseEndsAt.Sub(recovery.ServerNow).Seconds(), 2)
			if i < 3 {
				require.NoError(t, s.ResumeFocus(ctx, user))
				session, err = s.GetActivePomodoroSession(ctx, user)
				require.NoError(t, err)
			}
		}
		require.NoError(t, s.FinishFocusRest(ctx, user))
		idle, err = other.GetFocusState(ctx, user)
		require.NoError(t, err)
		require.Equal(t, "idle", idle.Phase)
	})
}
