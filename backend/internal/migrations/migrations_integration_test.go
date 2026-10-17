//go:build integration

package migrations

import (
	"context"
	app "github.com/kirill010106/todo-notificator"
	"github.com/kirill010106/todo-notificator/internal/storage/postgres"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	pg "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"testing"
	"time"
)

func TestReleaseMigrationAndStartupGate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	container, err := pg.Run(ctx, "postgres:16-alpine", pg.WithDatabase("release"), pg.WithUsername("postgres"), pg.WithPassword("postgres"), testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2)))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Terminate(context.Background())) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	store, err := postgres.New(dsn)
	require.NoError(t, err)
	defer store.Close()
	goose.SetBaseFS(app.MigrationsFS)
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.UpTo(store.DB, "migrations", 20))
	id, err := store.SaveUser(ctx, "release@example.invalid", []byte("test"))
	require.NoError(t, err)
	_, err = store.DB.ExecContext(ctx, `INSERT INTO tasks(user_id,title,description,deadline,reminder_at,status,is_notified) VALUES
 ($1,'relative','',TIMESTAMP '2099-01-02 12:00:00',TIMESTAMPTZ '2099-01-02 11:00:00Z','pending',false),
 ($1,'sent','',NULL,TIMESTAMPTZ '2099-01-02 11:00:00Z','pending',true),
 ($1,'closed','',NULL,TIMESTAMPTZ '2099-01-02 11:00:00Z','done',false)`, id)
	require.NoError(t, err)
	_, err = store.DB.ExecContext(ctx, `UPDATE users SET timezone='Local' WHERE id=$1`, id)
	require.NoError(t, err)
	require.ErrorContains(t, Run(store.DB, false), "run the backed-up release migration")
	var exists bool
	require.NoError(t, store.DB.QueryRow(`SELECT to_regclass('task_reminders') IS NOT NULL`).Scan(&exists))
	require.False(t, exists)
	require.NoError(t, Run(store.DB, true))
	require.NoError(t, Run(store.DB, false))
	zone, err := store.GetTimezone(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "UTC", zone)
	_, err = store.GetUserStats(ctx, id)
	require.NoError(t, err)
	var total, sent, cancelled int
	require.NoError(t, store.DB.QueryRow(`SELECT COUNT(*),COUNT(*) FILTER(WHERE status='sent' AND sent_at IS NULL),COUNT(*) FILTER(WHERE status='cancelled') FROM task_reminders`).Scan(&total, &sent, &cancelled))
	require.Equal(t, 3, total)
	require.Equal(t, 1, sent)
	require.Equal(t, 1, cancelled)
	require.NoError(t, Run(store.DB, true))
	require.NoError(t, store.DB.QueryRow(`SELECT COUNT(*) FROM task_reminders`).Scan(&total))
	require.Equal(t, 3, total)
	_, err = store.DB.Exec(`INSERT INTO goose_db_version(version_id,is_applied) VALUES(999,true)`)
	require.NoError(t, err)
	require.ErrorContains(t, Run(store.DB, false), "differs from required")
}
