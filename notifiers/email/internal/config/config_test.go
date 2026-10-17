package config

import (
	"os"
	"testing"

	"github.com/ilyakaznacheev/cleanenv"
)

func setRequiredEnvVars(t *testing.T) {
	t.Helper()
	t.Setenv("SMTP_USERNAME", "test@example.com")
	t.Setenv("SMTP_PASSWORD", "password")
	t.Setenv("WEBHOOK_SECRET", "secret")
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "config*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

func TestSMTP_EnvOverridesAndDefaults(t *testing.T) {
	setRequiredEnvVars(t)
	t.Setenv("SMTP_HOST", "smtp-relay.brevo.com")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMTP_USERNAME", "bb77b3001@smtp-brevo.com")
	t.Setenv("SMTP_PASSWORD", "secretkey")

	f := writeTempConfig(t, "env: local\n")

	var cfg Config
	if err := cleanenv.ReadConfig(f, &cfg); err != nil {
		t.Fatalf("cleanenv.ReadConfig: %v", err)
	}

	if cfg.SMTP.Host != "smtp-relay.brevo.com" {
		t.Errorf("expected host smtp-relay.brevo.com, got %s", cfg.SMTP.Host)
	}
	if cfg.SMTP.Port != 587 {
		t.Errorf("expected port 587, got %d", cfg.SMTP.Port)
	}
	if cfg.SMTP.Username != "bb77b3001@smtp-brevo.com" {
		t.Errorf("expected username bb77b3001@smtp-brevo.com, got %s", cfg.SMTP.Username)
	}
	if cfg.SMTP.SenderName != "ToDoNotificator" {
		t.Errorf("expected default sender name ToDoNotificator, got %s", cfg.SMTP.SenderName)
	}
}

func TestSMTP_CustomFromAndSenderName(t *testing.T) {
	setRequiredEnvVars(t)
	t.Setenv("SMTP_FROM", "verified@example.com")
	t.Setenv("SMTP_SENDER_NAME", "My Task App")

	f := writeTempConfig(t, "env: local\n")

	var cfg Config
	if err := cleanenv.ReadConfig(f, &cfg); err != nil {
		t.Fatalf("cleanenv.ReadConfig: %v", err)
	}

	if cfg.SMTP.From != "verified@example.com" {
		t.Errorf("expected from verified@example.com, got %s", cfg.SMTP.From)
	}
	if cfg.SMTP.SenderName != "My Task App" {
		t.Errorf("expected sender name 'My Task App', got %s", cfg.SMTP.SenderName)
	}
}
