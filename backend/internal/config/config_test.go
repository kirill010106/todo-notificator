package config

import (
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConfigFileDefaultsAndEnvironmentOverrides(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(file, []byte("env: prod\napp_secret: file-secret\nhttp_server:\n  address: localhost:8082\n  timeout: 8s\n"), 0600))
	t.Setenv("CONFIG_PATH", file)
	t.Setenv("APP_SECRET", "environment-secret")
	t.Setenv("CLIENT_URL", "https://app.test")
	t.Setenv("ENV", "prod")
	t.Setenv("METRICS_ADDRESS", "127.0.0.1:9099")
	t.Setenv("WEBHOOK_SECRET", "delivery-secret")
	cfg := MustLoad()
	require.Equal(t, "environment-secret", cfg.AppSecret)
	require.Equal(t, "https://app.test", cfg.ClientURL)
	require.Equal(t, "prod", cfg.Env)
	require.Equal(t, "localhost:8082", cfg.Address)
	require.Equal(t, 8*time.Second, cfg.Timeout)
	require.Equal(t, 15*time.Minute, cfg.AccessTokenTTL)
	require.Equal(t, 168*time.Hour, cfg.RefreshTokenTTL)
	require.Equal(t, "127.0.0.1:9099", cfg.MetricsServer.Address)
	require.Equal(t, "delivery-secret", cfg.Webhook.Secret)
}
