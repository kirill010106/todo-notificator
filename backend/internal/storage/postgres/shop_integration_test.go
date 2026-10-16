//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	todonotificator "github.com/kirill010106/todo-notificator"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"sync"
	"testing"
	"time"
)

func TestServerShop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	pg, err := tcpostgres.Run(ctx, "postgres:16-alpine", tcpostgres.WithDatabase("shop"), tcpostgres.WithUsername("postgres"), tcpostgres.WithPassword("postgres"), testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pg.Terminate(context.Background())) })
	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	s, err := New(dsn)
	require.NoError(t, err)
	defer s.Close()
	goose.SetBaseFS(todonotificator.MigrationsFS)
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.Up(s.DB, "migrations"))
	number := 0
	newUser := func() int64 {
		number++
		id, err := s.SaveUser(ctx, fmt.Sprintf("shop%d@test.local", number), []byte("test"))
		require.NoError(t, err)
		return id
	}
	wallet := func(user int64) domain.Wallet { w, _, err := s.GetShop(ctx, user); require.NoError(t, err); return w }
	act := func(user int64, request domain.ShopAction) domain.Wallet {
		w, err := s.ShopAction(ctx, user, request)
		require.NoError(t, err)
		return w
	}
	task := func(user int64) int64 {
		id, err := s.SaveTask(ctx, domain.Task{UserID: user, Title: "Task", Status: "pending"})
		require.NoError(t, err)
		return id
	}
	t.Run("import never trusts browser money or boosters and runs once", func(t *testing.T) {
		user := newUser()
		require.EqualValues(t, 50, wallet(user).Coins)
		legacy := &domain.Wallet{Coins: 999999, Boosters: map[string]int64{"xp_boost": 100}, ActiveTheme: "ocean", ActiveSound: "bell", CustomRewards: []domain.PersonalReward{{ID: "old", Title: "Книга", Price: 20}, {ID: "old", Title: "Duplicate", Price: 20}, {ID: "cheap", Title: "Кофе", Price: 5}}}
		w := act(user, domain.ShopAction{Action: "import_legacy", Legacy: legacy})
		require.EqualValues(t, 50, w.Coins)
		require.Len(t, w.CustomRewards, 2)
		require.Zero(t, w.Boosters["xp_boost"])
		require.Equal(t, "ocean", w.ActiveTheme)
		require.Equal(t, "bell", w.ActiveSound)
		require.Len(t, act(user, domain.ShopAction{Action: "import_legacy", Legacy: legacy}).CustomRewards, 2)
		require.Empty(t, wallet(newUser()).CustomRewards)
	})
	t.Run("empty first device leaves later legacy import available", func(t *testing.T) {
		user := newUser()
		empty := &domain.Wallet{CustomRewards: []domain.PersonalReward{}, ActiveTheme: "default", ActiveSound: "default"}
		w := act(user, domain.ShopAction{Action: "import_legacy", Legacy: empty})
		require.False(t, w.Imported)
		legacy := &domain.Wallet{CustomRewards: []domain.PersonalReward{{ID: "later", Title: "Книга", Price: 5}}, ActiveTheme: "default", ActiveSound: "default"}
		w = act(user, domain.ShopAction{Action: "import_legacy", Legacy: legacy})
		require.True(t, w.Imported)
		require.Len(t, w.CustomRewards, 1)
	})
	t.Run("concurrent purchases cannot overspend", func(t *testing.T) {
		user := newUser()
		var wg sync.WaitGroup
		results := make(chan error, 2)
		for _, item := range []string{"ocean", "violet"} {
			wg.Add(1)
			go func(item string) {
				defer wg.Done()
				_, err := s.ShopAction(ctx, user, domain.ShopAction{Action: "buy", ItemID: item, Price: 1})
				results <- err
			}(item)
		}
		wg.Wait()
		close(results)
		success := 0
		for err := range results {
			if err == nil {
				success++
			} else {
				var e *domain.ContractError
				require.ErrorAs(t, err, &e)
				require.Equal(t, "INSUFFICIENT_COINS", e.Code)
			}
		}
		require.Equal(t, 1, success)
		require.EqualValues(t, 10, wallet(user).Coins)
		require.Len(t, wallet(user).OwnedItems, 2)
	})
	t.Run("personal rewards and settings are account scoped", func(t *testing.T) {
		user, other := newUser(), newUser()
		w := act(user, domain.ShopAction{Action: "create_personal", Title: "Кофе", Price: 20})
		id := w.CustomRewards[0].ID
		_, err := s.ShopAction(ctx, other, domain.ShopAction{Action: "redeem_personal", ItemID: id})
		require.Error(t, err)
		require.EqualValues(t, 30, act(user, domain.ShopAction{Action: "redeem_personal", ItemID: id, Price: 1}).Coins)
		require.Empty(t, act(user, domain.ShopAction{Action: "delete_personal", ItemID: id}).CustomRewards)
		act(user, domain.ShopAction{Action: "equip", ItemID: "default_sound"})
		_, err = s.ShopAction(ctx, user, domain.ShopAction{Action: "equip", ItemID: "bell"})
		require.Error(t, err)
	})
	t.Run("single and bulk task rewards cannot be farmed by reopening", func(t *testing.T) {
		user := newUser()
		a, b := task(user), task(user)
		done, pending := "done", "pending"
		require.NoError(t, s.UpdateTask(ctx, user, a, domain.TaskUpdate{Status: &done}))
		require.EqualValues(t, 55, wallet(user).Coins)
		require.NoError(t, s.UpdateTask(ctx, user, a, domain.TaskUpdate{Status: &pending}))
		_, err := s.BulkCompleteTasks(ctx, user, []int64{a, b})
		require.NoError(t, err)
		require.EqualValues(t, 60, wallet(user).Coins)
		_, err = s.BulkCompleteTasks(ctx, user, []int64{a, b})
		require.NoError(t, err)
		require.EqualValues(t, 60, wallet(user).Coins)
	})
	t.Run("XP booster survives cancellation and only multiplies qualified focus", func(t *testing.T) {
		user := newUser()
		require.NoError(t, s.withUserTx(ctx, user, func(tx *sql.Tx) error { return awardCoinsTx(ctx, tx, user, "test-funds", 100) }))
		act(user, domain.ShopAction{Action: "buy", ItemID: "xp_boost"})
		act(user, domain.ShopAction{Action: "activate_booster"})
		_, err := s.ShopAction(ctx, user, domain.ShopAction{Action: "activate_booster"})
		require.Error(t, err)
		id, err := s.StartPomodoroSession(ctx, user, nil)
		require.NoError(t, err)
		require.NoError(t, s.AddPomodoroBreak(ctx, user, id.ID))
		focus, err := s.GetFocusState(ctx, user)
		require.NoError(t, err)
		require.Equal(t, "extra_break", focus.Phase)
		require.Contains(t, focus.AvailableActions, "cancel")
		require.NoError(t, s.CancelPomodoroSession(ctx, user, id.ID))
		require.True(t, wallet(user).ActiveBooster)
		require.EqualValues(t, 50, wallet(user).Coins)
		id, err = s.StartPomodoroSession(ctx, user, nil)
		require.NoError(t, err)
		require.NoError(t, s.FinishPomodoroSession(ctx, user, id.ID, "completed", false))
		require.True(t, wallet(user).ActiveBooster)
		id, err = s.StartPomodoroSession(ctx, user, nil)
		require.NoError(t, err)
		_, err = s.DB.ExecContext(ctx, `UPDATE pomodoro_sessions SET started_at=CURRENT_TIMESTAMP-INTERVAL '25 minutes' WHERE id=$1`, id.ID)
		require.NoError(t, err)
		var response domain.OperationResponse
		response, err = s.ExecuteOperation(ctx, user, "boost-finish", "POST /stop", []byte(`{}`), func(txctx context.Context) domain.OperationResponse {
			if err := s.FinishPomodoroSession(txctx, user, id.ID, "completed", false); err != nil {
				t.Error(err)
				return domain.OperationResponse{Status: 500}
			}
			return domain.OperationResponse{Status: 200, Body: []byte(`{"status":"OK"}`)}
		})
		require.NoError(t, err)
		require.Equal(t, 200, response.Status)
		var payload struct{ Rewards domain.ActionRewards }
		require.NoError(t, json.Unmarshal(response.Body, &payload))
		require.Equal(t, 40, payload.Rewards.PomodoroXP)
		require.False(t, wallet(user).ActiveBooster)
		require.EqualValues(t, 60, wallet(user).Coins)
		require.Error(t, s.FinishPomodoroSession(ctx, user, id.ID, "completed", false))
		require.EqualValues(t, 60, wallet(user).Coins)
	})
	t.Run("cancel window uses server time and ownership", func(t *testing.T) {
		user, other := newUser(), newUser()
		taskID := task(user)
		id, err := s.StartPomodoroSession(ctx, user, &taskID)
		require.NoError(t, err)
		require.Error(t, s.CancelPomodoroSession(ctx, other, id.ID))
		require.NoError(t, s.CancelPomodoroSession(ctx, user, id.ID))
		value, err := s.GetTask(ctx, user, taskID)
		require.NoError(t, err)
		require.Equal(t, "pending", value.Status)
		id, err = s.StartPomodoroSession(ctx, user, &taskID)
		require.NoError(t, err)
		_, err = s.DB.ExecContext(ctx, `UPDATE pomodoro_sessions SET started_at=clock_timestamp()-INTERVAL '31 seconds' WHERE id=$1`, id.ID)
		require.NoError(t, err)
		require.Error(t, s.CancelPomodoroSession(ctx, user, id.ID))
		require.EqualValues(t, 50, wallet(user).Coins)
	})
}
