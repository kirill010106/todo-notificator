//go:build integration

package postgres

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	todonotificator "github.com/kirill010106/todo-notificator"
	"github.com/kirill010106/todo-notificator/internal/domain"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestProductivityRewards(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	dsn := os.Getenv("GAMIFICATION_TEST_DSN")
	if dsn == "" {
		pg, err := tcpostgres.Run(ctx, "postgres:16-alpine", tcpostgres.WithDatabase("gamification"), tcpostgres.WithUsername("postgres"), tcpostgres.WithPassword("postgres"), testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)))
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, pg.Terminate(context.Background())) })
		dsn, err = pg.ConnectionString(ctx, "sslmode=disable")
		require.NoError(t, err)
	}
	s, err := New(dsn)
	require.NoError(t, err)
	defer s.Close()
	var tables int
	require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema NOT IN ('pg_catalog','information_schema') AND table_schema NOT LIKE 'pg_toast%'`).Scan(&tables))
	require.Zero(t, tables, "GAMIFICATION_TEST_DSN must point to an empty disposable database")
	goose.SetBaseFS(todonotificator.MigrationsFS)
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.UpTo(s.DB, "migrations", 26))
	legacy, err := s.SaveUser(ctx, "legacy-gamification@test.local", []byte("test"))
	require.NoError(t, err)
	_, err = s.DB.ExecContext(ctx, `INSERT INTO user_stats(user_id,points,level,total_pomodoros) VALUES($1,700,5,100)`, legacy)
	require.NoError(t, err)
	_, err = s.DB.ExecContext(ctx, `INSERT INTO achievements(user_id,code) VALUES($1,'night_owl')`, legacy)
	require.NoError(t, err)
	_, err = s.DB.ExecContext(ctx, `INSERT INTO daily_reward_claims(user_id,reward_date,code,xp) VALUES($1,CURRENT_DATE,'daily_chest',50)`, legacy)
	require.NoError(t, err)
	oldWallet := newWallet()
	oldWallet.Coins = 123
	oldWallet.Boosters["xp_boost"] = 2
	oldWallet.ActiveBooster = true
	oldWallet.CustomRewards = []domain.PersonalReward{{ID: "book", Title: "Книга", Price: 150}}
	oldWallet.OwnedItems = append(oldWallet.OwnedItems, "ocean")
	require.NoError(t, writeWallet(ctx, s.DB, legacy, oldWallet))
	oldTask, err := s.SaveTask(ctx, domain.Task{UserID: legacy, Title: "Legacy", Status: "pending"})
	require.NoError(t, err)
	_, err = s.DB.ExecContext(ctx, `UPDATE tasks SET reward_claimed=true WHERE id=$1`, oldTask)
	require.NoError(t, err)

	t.Run("upgrade preserves data and refunds unused boosters exactly once", func(t *testing.T) {
		for i := 0; i < 2; i++ {
			require.NoError(t, goose.Up(s.DB, "migrations"))
			w, ledger, err := s.GetShop(ctx, legacy)
			require.NoError(t, err)
			require.EqualValues(t, 423, w.Coins)
			require.False(t, w.ActiveBooster)
			require.Zero(t, w.Boosters["xp_boost"])
			require.Equal(t, oldWallet.CustomRewards, w.CustomRewards)
			require.Equal(t, oldWallet.OwnedItems, w.OwnedItems)
			require.Len(t, ledger, 1)
			require.EqualValues(t, 300, ledger[0].Coins)
			stats, err := s.GetUserStats(ctx, legacy)
			require.NoError(t, err)
			require.EqualValues(t, 700, *stats.Points)
			require.EqualValues(t, 5, *stats.Level)
			require.EqualValues(t, 1, stats.TotalCompletedTasks)
			awards, err := s.GetAchievements(ctx, legacy)
			require.NoError(t, err)
			require.Len(t, awards, 7)
			require.True(t, awards[5].Unlocked)
			require.True(t, awards[6].Archived)
			require.True(t, awards[6].Unlocked)
			var claims int
			require.NoError(t, s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM daily_reward_claims WHERE user_id=$1`, legacy).Scan(&claims))
			require.Equal(t, 1, claims)
			if i == 0 {
				require.NoError(t, goose.Down(s.DB, "migrations"))
			}
		}
	})

	serial := 0
	newUser := func() int64 {
		serial++
		id, err := s.SaveUser(ctx, fmt.Sprintf("gamification%d@test.local", serial), []byte("test"))
		require.NoError(t, err)
		return id
	}
	execute := func(user int64, key string, action func(context.Context) error) (domain.OperationResponse, error) {
		return s.ExecuteOperation(ctx, user, key, "POST /test", []byte(`{}`), func(c context.Context) domain.OperationResponse {
			if err := action(c); err != nil {
				return domain.OperationResponse{Status: 500, Body: []byte(`{"error":"failed"}`)}
			}
			return domain.OperationResponse{Status: 200, Body: []byte(`{}`)}
		})
	}
	t.Run("personal redemption replay concurrent spending and rollback", func(t *testing.T) {
		user := newUser()
		w, err := s.ShopAction(ctx, user, domain.ShopAction{Action: "create_personal", Title: "Книга", Price: 40})
		require.NoError(t, err)
		request := domain.ShopAction{Action: "redeem_personal", ItemID: w.CustomRewards[0].ID}
		var wg sync.WaitGroup
		responses := make([]domain.OperationResponse, 6)
		failures := make([]error, 6)
		for i := range responses {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				responses[i], failures[i] = execute(user, "redeem", func(c context.Context) error { _, err := s.ShopAction(c, user, request); return err })
			}(i)
		}
		wg.Wait()
		for i := range responses {
			require.NoError(t, failures[i])
			require.Equal(t, 200, responses[i].Status)
			require.JSONEq(t, string(responses[0].Body), string(responses[i].Body))
		}
		w, ledger, err := s.GetShop(ctx, user)
		require.NoError(t, err)
		require.EqualValues(t, 10, w.Coins)
		require.Len(t, ledger, 1)
		_, err = s.ShopAction(ctx, user, request)
		require.Error(t, err)
		other := newUser()
		w, err = s.ShopAction(ctx, other, domain.ShopAction{Action: "create_personal", Title: "Еда", Price: 40})
		require.NoError(t, err)
		purchase := domain.ShopAction{Action: "redeem_personal", ItemID: w.CustomRewards[0].ID}
		results := make(chan error, 2)
		for i := 0; i < 2; i++ {
			go func() { _, err := s.ShopAction(ctx, other, purchase); results <- err }()
		}
		success := 0
		for i := 0; i < 2; i++ {
			if <-results == nil {
				success++
			}
		}
		require.Equal(t, 1, success)
		rollbackUser := newUser()
		_, err = s.DB.ExecContext(ctx, `ALTER TABLE reward_ledger ADD CONSTRAINT fail_purchase CHECK(coins>=0) NOT VALID`)
		require.NoError(t, err)
		_, err = s.ShopAction(ctx, rollbackUser, domain.ShopAction{Action: "buy", ItemID: "ocean"})
		require.Error(t, err)
		_, err = s.DB.ExecContext(ctx, `ALTER TABLE reward_ledger DROP CONSTRAINT fail_purchase`)
		require.NoError(t, err)
		w, ledger, err = s.GetShop(ctx, rollbackUser)
		require.NoError(t, err)
		require.EqualValues(t, 50, w.Coins)
		require.Len(t, w.OwnedItems, 1)
		require.Empty(t, ledger)
	})
}
