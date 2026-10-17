//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	todonotificator "github.com/kirill010106/todo-notificator"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/kirill010106/todo-notificator/internal/storage"
	queue "github.com/kirill010106/todo-notificator/notifiers/shared/storage/postgres"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"golang.org/x/crypto/bcrypt"
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

	t.Run("account registration credentials and password recovery", func(t *testing.T) {
		password := []byte("password123")
		hash, err := bcrypt.GenerateFromPassword(password, bcrypt.MinCost)
		require.NoError(t, err)
		u, err := s.RegisterAccount(ctx, "account@test.local", "  Кирилл  ", hash, "initial-verify", "initial-session", time.Now().Add(time.Hour))
		require.NoError(t, err)
		require.Equal(t, "Кирилл", u.DisplayName)
		loaded, err := s.GetUserByID(ctx, u.ID)
		require.NoError(t, err)
		require.False(t, loaded.IsVerified)
		require.NoError(t, s.RequestPasswordReset(ctx, u.Email, "reset-initial"))
		var digest, raw string
		require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT token_hash FROM password_reset_tokens WHERE user_id=$1`, u.ID).Scan(&digest))
		require.Equal(t, ResetTokenHash("reset-initial"), digest)
		q, err := queue.New(dsn)
		require.NoError(t, err)
		defer q.Close()
		d, err := q.ClaimPasswordResetDelivery(ctx)
		require.NoError(t, err)
		require.NotNil(t, d)
		ready, err := q.PasswordResetReady(ctx, *d)
		require.NoError(t, err)
		require.True(t, ready)
		require.NoError(t, q.FinishPasswordResetDelivery(ctx, *d, errors.New("smtp unavailable")))
		require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT token FROM password_reset_outbox WHERE id=$1`, d.ID).Scan(&raw))
		require.Equal(t, "reset-initial", raw)
		require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT token_hash FROM password_reset_tokens WHERE user_id=$1`, u.ID).Scan(&digest))
		// Requests inside one minute neither create a delivery nor retire the usable link.
		require.NoError(t, s.RequestPasswordReset(ctx, u.Email, "suppressed"))
		require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT token_hash FROM password_reset_tokens WHERE user_id=$1`, u.ID).Scan(&raw))
		require.Equal(t, digest, raw)
		require.NoError(t, s.RequestPasswordReset(ctx, "unknown@test.local", "unknown"))
		// Invalid/expired tokens leave the password and session intact.
		require.Error(t, s.ResetPassword(ctx, "missing", hash))
		_, err = s.DB.ExecContext(ctx, `UPDATE password_reset_tokens SET expires_at=CURRENT_TIMESTAMP-INTERVAL '1 second' WHERE user_id=$1`, u.ID)
		require.NoError(t, err)
		require.Error(t, s.ResetPassword(ctx, "reset-initial", hash))
		_, err = s.DB.ExecContext(ctx, `UPDATE password_reset_tokens SET expires_at=CURRENT_TIMESTAMP+INTERVAL '30 minutes' WHERE user_id=$1`, u.ID)
		require.NoError(t, err)
		// Competing uses serialize: exactly one succeeds and all sessions are revoked.
		var successes atomic.Int32
		var wg sync.WaitGroup
		newHash, err := bcrypt.GenerateFromPassword([]byte("newpassword123"), bcrypt.MinCost)
		require.NoError(t, err)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if s.ResetPassword(ctx, "reset-initial", newHash) == nil {
					successes.Add(1)
				}
			}()
		}
		wg.Wait()
		require.Equal(t, int32(1), successes.Load())
		loaded, err = s.GetUserByID(ctx, u.ID)
		require.NoError(t, err)
		require.False(t, loaded.IsVerified)
		require.Equal(t, int64(1), loaded.AuthVersion)
		require.NoError(t, bcrypt.CompareHashAndPassword(loaded.PassHash, []byte("newpassword123")))
		require.Error(t, bcrypt.CompareHashAndPassword(loaded.PassHash, password))
		_, err = s.RotateRefreshToken(ctx, "initial-session", "replacement", time.Now().Add(time.Hour))
		require.ErrorIs(t, err, storage.ErrRefreshTokenInvalid)
		var count int
		require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM password_reset_outbox WHERE user_id=$1 AND token IS NOT NULL`, u.ID).Scan(&count))
		require.Zero(t, count)
		require.NoError(t, s.ConfirmEmail(ctx, "initial-verify"))
		oldReset := "before-change"
		_, err = s.DB.ExecContext(ctx, `UPDATE password_reset_outbox SET created_at=CURRENT_TIMESTAMP-INTERVAL '2 minutes' WHERE user_id=$1`, u.ID)
		require.NoError(t, err)
		require.NoError(t, s.RequestPasswordReset(ctx, u.Email, oldReset))
		address := "changed@test.local"
		_, _, err = s.ChangeCredentials(ctx, u.ID, 1, "wrong", &address, nil, "new-verify", "new-session", time.Now().Add(time.Hour))
		require.Error(t, err)
		changed, ok, err := s.ChangeCredentials(ctx, u.ID, 1, "newpassword123", &address, nil, "new-verify", "new-session", time.Now().Add(time.Hour))
		require.NoError(t, err)
		require.True(t, ok)
		require.False(t, changed.IsVerified)
		require.Equal(t, int64(2), changed.AuthVersion)
		require.Error(t, s.ResetPassword(ctx, oldReset, hash))
		require.Error(t, s.ConfirmEmail(ctx, "initial-verify"))
		require.NoError(t, s.ConfirmEmail(ctx, "new-verify"))
		_, ok, err = s.ChangeCredentials(ctx, u.ID, 2, "newpassword123", &address, nil, "unused", "unused", time.Now().Add(time.Hour))
		require.NoError(t, err)
		require.False(t, ok)
		name := ""
		zone := "Europe/Moscow"
		require.NoError(t, s.UpdateAccountProfile(ctx, u.ID, &name, &zone))
		loaded, err = s.GetUserByID(ctx, u.ID)
		require.NoError(t, err)
		require.Empty(t, loaded.DisplayName)
		require.NoError(t, s.UpdateAccountProfile(ctx, u.ID, nil, nil))
		gotZone, err := s.GetTimezone(ctx, u.ID)
		require.NoError(t, err)
		require.Equal(t, zone, gotZone)
		// Existing address conflict rolls the whole credential operation back.
		_, err = s.SaveUser(ctx, "occupied@test.local", hash)
		require.NoError(t, err)
		occupied := "occupied@test.local"
		_, _, err = s.ChangeCredentials(ctx, u.ID, 2, "newpassword123", &occupied, nil, "unused", "unused", time.Now().Add(time.Hour))
		require.Error(t, err)
		loaded, err = s.GetUserByID(ctx, u.ID)
		require.NoError(t, err)
		require.Equal(t, address, loaded.Email)
		require.Equal(t, int64(2), loaded.AuthVersion)
		_, err = s.RotateRefreshToken(ctx, "new-session", "rotated-session", time.Now().Add(time.Hour))
		require.NoError(t, err)
		// Authenticated password changes keep just the newly created session.
		_, ok, err = s.ChangeCredentials(ctx, u.ID, 2, "newpassword123", nil, hash, "unused", "password-session", time.Now().Add(time.Hour))
		require.NoError(t, err)
		require.True(t, ok)
		_, err = s.RotateRefreshToken(ctx, "rotated-session", "bad", time.Now().Add(time.Hour))
		require.ErrorIs(t, err, storage.ErrRefreshTokenInvalid)
		_, err = s.RotateRefreshToken(ctx, "password-session", "good", time.Now().Add(time.Hour))
		require.NoError(t, err)
	})

	t.Run("reset rate limits replacement delivery and verification race", func(t *testing.T) {
		hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
		require.NoError(t, err)
		u, err := s.RegisterAccount(ctx, "rate@test.local", "", hash, "rate-verify", "rate-session", time.Now().Add(time.Hour))
		require.NoError(t, err)
		t.Cleanup(func() { _, _ = s.DB.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, u.ID) })
		require.NoError(t, s.RequestPasswordReset(ctx, u.Email, "rate-0"))
		for i := 1; i < 5; i++ {
			_, err = s.DB.ExecContext(ctx, `UPDATE password_reset_outbox SET created_at=created_at-INTERVAL '2 minutes' WHERE user_id=$1`, u.ID)
			require.NoError(t, err)
			require.NoError(t, s.RequestPasswordReset(ctx, u.Email, fmt.Sprintf("rate-%d", i)))
		}
		var count int
		require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM password_reset_outbox WHERE user_id=$1`, u.ID).Scan(&count))
		require.Equal(t, 5, count)
		require.Error(t, s.ResetPassword(ctx, "rate-0", hash))
		_, err = s.DB.ExecContext(ctx, `UPDATE password_reset_outbox SET created_at=created_at-INTERVAL '2 minutes' WHERE user_id=$1`, u.ID)
		require.NoError(t, err)
		require.NoError(t, s.RequestPasswordReset(ctx, u.Email, "rate-5"))
		require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM password_reset_outbox WHERE user_id=$1`, u.ID).Scan(&count))
		require.Equal(t, 5, count)
		var digest string
		require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT token_hash FROM password_reset_tokens WHERE user_id=$1`, u.ID).Scan(&digest))
		require.Equal(t, ResetTokenHash("rate-4"), digest)
		q, err := queue.New(dsn)
		require.NoError(t, err)
		defer q.Close()
		d, err := q.ClaimPasswordResetDelivery(ctx)
		require.NoError(t, err)
		require.NotNil(t, d)
		require.Equal(t, "rate-4", d.Token)
		require.NoError(t, q.FinishPasswordResetDelivery(ctx, *d, nil))
		var raw sql.NullString
		require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT token FROM password_reset_outbox WHERE id=$1`, d.ID).Scan(&raw))
		require.False(t, raw.Valid)
		// A stale acknowledgement cannot restore an erased token.
		require.NoError(t, q.FinishPasswordResetDelivery(ctx, *d, errors.New("stale failure")))
		require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT token FROM password_reset_outbox WHERE id=$1`, d.ID).Scan(&raw))
		require.False(t, raw.Valid)
		for i := 0; i < 6; i++ {
			oldToken := fmt.Sprintf("race-verify-%d", i)
			race, err := s.RegisterAccount(ctx, fmt.Sprintf("race%d@test.local", i), "", hash, oldToken, fmt.Sprintf("race-session-%d", i), time.Now().Add(time.Hour))
			require.NoError(t, err)
			raceID := race.ID
			t.Cleanup(func() { _, _ = s.DB.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, raceID) })
			address := fmt.Sprintf("race-new%d@test.local", i)
			var wg sync.WaitGroup
			wg.Add(2)
			var changeErr error
			go func() { defer wg.Done(); _ = s.ConfirmEmail(ctx, oldToken) }()
			go func() {
				defer wg.Done()
				_, _, changeErr = s.ChangeCredentials(ctx, race.ID, 0, "password123", &address, nil, fmt.Sprintf("race-new-verify-%d", i), fmt.Sprintf("race-new-session-%d", i), time.Now().Add(time.Hour))
			}()
			wg.Wait()
			require.NoError(t, changeErr)
			loaded, err := s.GetUserByID(ctx, race.ID)
			require.NoError(t, err)
			require.Equal(t, address, loaded.Email)
			require.False(t, loaded.IsVerified)
			require.Error(t, s.ConfirmEmail(ctx, oldToken))
		}
	})
	t.Run("verification leases respect retry budget and reject stale acknowledgements", func(t *testing.T) {
		user := newUser()
		require.NoError(t, s.SaveEmailVerificationToken(ctx, user, "lease-budget", time.Now().Add(time.Hour)))
		q, err := queue.New(dsn)
		require.NoError(t, err)
		defer q.Close()
		first, err := q.ClaimVerificationDelivery(ctx)
		require.NoError(t, err)
		require.NotNil(t, first)
		_, err = s.DB.ExecContext(ctx, `UPDATE verification_email_outbox SET lease_until=CURRENT_TIMESTAMP-INTERVAL '1 second' WHERE id=$1`, first.ID)
		require.NoError(t, err)
		next, err := q.ClaimVerificationDelivery(ctx)
		require.NoError(t, err)
		require.NotNil(t, next)
		require.Equal(t, 2, next.Attempt)
		require.NoError(t, q.FinishVerificationDelivery(ctx, *first, nil))
		var status string
		require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT status FROM verification_email_outbox WHERE id=$1`, first.ID).Scan(&status))
		require.Equal(t, "scheduled", status)
		_, err = s.DB.ExecContext(ctx, `UPDATE verification_email_outbox SET attempts=4,lease_until=CURRENT_TIMESTAMP-INTERVAL '1 second' WHERE id=$1`, first.ID)
		require.NoError(t, err)
		last, err := q.ClaimVerificationDelivery(ctx)
		require.NoError(t, err)
		require.NotNil(t, last)
		require.Equal(t, 5, last.Attempt)
		active, err := q.ClaimVerificationDelivery(ctx)
		require.NoError(t, err)
		require.Nil(t, active)
		_, err = s.DB.ExecContext(ctx, `UPDATE verification_email_outbox SET lease_until=CURRENT_TIMESTAMP-INTERVAL '1 second' WHERE id=$1`, last.ID)
		require.NoError(t, err)
		exhausted, err := q.ClaimVerificationDelivery(ctx)
		require.NoError(t, err)
		require.Nil(t, exhausted)
		require.NoError(t, q.FinishVerificationDelivery(ctx, *last, nil))
		var attempts int
		require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT status,attempts FROM verification_email_outbox WHERE id=$1`, last.ID).Scan(&status, &attempts))
		require.Equal(t, "failed", status)
		require.Equal(t, 5, attempts)
	})
	t.Run("verification token and email delivery enter the outbox together", func(t *testing.T) {
		user := newUser()
		expires := time.Now().Add(24 * time.Hour)
		require.NoError(t, s.SaveEmailVerificationToken(ctx, user, "outbox-token", expires))
		var email, queuedToken, status string
		var queuedExpiry time.Time
		err := s.DB.QueryRowContext(ctx, `SELECT email,token,status,expires_at FROM verification_email_outbox WHERE user_id=$1`, user).Scan(&email, &queuedToken, &status, &queuedExpiry)
		require.NoError(t, err)
		require.Equal(t, fmt.Sprintf("contract%d@test.local", number), email)
		require.Equal(t, "outbox-token", queuedToken)
		require.Equal(t, "scheduled", status)
		require.WithinDuration(t, expires, queuedExpiry, time.Second)
	})
	t.Run("parallel category creation cannot exceed the Free limit", func(t *testing.T) {
		user := newUser()
		var wg sync.WaitGroup
		results := make(chan error, 8)
		for i := 0; i < cap(results); i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_, err := s.CreateCategory(ctx, domain.Category{UserID: user, Name: fmt.Sprintf("Category %d", i)})
				results <- err
			}(i)
		}
		wg.Wait()
		close(results)
		success, limited := 0, 0
		for err := range results {
			if err == nil {
				success++
			} else if errors.Is(err, storage.ErrCategoryLimitReached) {
				limited++
			} else {
				require.NoError(t, err)
			}
		}
		count, err := s.CountCategories(ctx, user)
		require.NoError(t, err)
		require.Equal(t, 3, count)
		require.Equal(t, 3, success)
		require.Equal(t, 5, limited)
	})
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
		task, err := s.SaveTask(ctx, domain.Task{UserID: user, Title: "Nullable", Status: "pending", Deadline: &deadline, ReminderInputs: []domain.ReminderInput{{OffsetMinutes: new(int64)}}})
		require.NoError(t, err)
		require.NoError(t, s.UpdateTask(ctx, user, task, domain.TaskUpdate{ClearDeadline: true, Reminders: &[]domain.ReminderInput{}, ClearCategoryID: true}))
		saved, err := s.GetTask(ctx, user, task)
		require.NoError(t, err)
		require.Nil(t, saved.Deadline)
		require.Empty(t, saved.Reminders)
	})
	t.Run("today counts all matches but pages the lists", func(t *testing.T) {
		user := newUser()
		err := s.SetTimezone(ctx, user, "Local")
		require.Error(t, err, "Go's Local location is not an IANA zone supported by PostgreSQL and browsers")
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
	t.Run("stale current streak expires on read while best streak remains", func(t *testing.T) {
		user := newUser()
		_, err := s.GetUserStats(ctx, user)
		require.NoError(t, err)
		_, err = s.DB.ExecContext(ctx, `UPDATE user_stats SET current_streak=7,best_streak=12,last_activity_date=(CURRENT_TIMESTAMP AT TIME ZONE 'UTC')::date-10 WHERE user_id=$1`, user)
		require.NoError(t, err)
		stats, err := s.GetUserStats(ctx, user)
		require.NoError(t, err)
		require.Equal(t, int64(0), *stats.CurrentStreak)
		require.Equal(t, int64(12), *stats.BestStreak)
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
		paymentRow, err := s.CreatePayment(ctx, "paid-payment", user, "49.00", "RUB", "pending", "Premium")
		require.NoError(t, err)
		retriedRow, err := s.CreatePayment(ctx, "paid-payment", user, "49.00", "RUB", "pending", "Premium")
		require.NoError(t, err)
		require.Equal(t, paymentRow, retriedRow)
		_, err = s.CreatePayment(ctx, "paid-payment", newUser(), "49.00", "RUB", "pending", "Premium")
		require.Error(t, err, "a provider operation must never be attached to another account")
		_, err = s.CreatePayment(ctx, "latest-pending", user, "49.00", "RUB", "pending", "Premium")
		require.NoError(t, err)
		pendingIDs, err := s.GetPendingPayments(ctx, user)
		require.NoError(t, err)
		require.Equal(t, []string{"latest-pending", "paid-payment"}, pendingIDs)
		owner, err = s.CompletePaymentAndGrantPremium(ctx, "paid-payment")
		require.NoError(t, err)
		require.Equal(t, user, owner)
		_, err = s.CreatePayment(ctx, "paid-payment", user, "49.00", "RUB", "pending", "Premium")
		require.NoError(t, err)
		var paymentStatus string
		require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT status FROM payments WHERE yookassa_payment_id='paid-payment'`).Scan(&paymentStatus))
		require.Equal(t, "succeeded", paymentStatus)
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
