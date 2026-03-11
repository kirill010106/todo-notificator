package config

import (
	"os"
	"testing"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

func setRequiredEnvVars(t *testing.T) {
	t.Helper()
	t.Setenv("SMTP_USERNAME", "test@example.com")
	t.Setenv("SMTP_PASSWORD", "password")
	t.Setenv("WEBHOOK_SECRET", "secret")
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
}

func TestIntervals_ParsedFromYAML(t *testing.T) {
	setRequiredEnvVars(t)
	const content = `
notification_intervals:
  - "24h"
  - "3h"
  - "30m"
  - "5m"
`
	f := writeTempConfig(t, content)

	var cfg Config
	if err := cleanenv.ReadConfig(f, &cfg); err != nil {
		t.Fatalf("cleanenv.ReadConfig: %v", err)
	}

	want := []time.Duration{
		24 * time.Hour,
		3 * time.Hour,
		30 * time.Minute,
		5 * time.Minute,
	}

	if len(cfg.Intervals) != len(want) {
		t.Fatalf("got %d intervals, want %d: %v", len(cfg.Intervals), len(want), cfg.Intervals)
	}
	for i, d := range want {
		if cfg.Intervals[i] != d {
			t.Errorf("Intervals[%d] = %v, want %v", i, cfg.Intervals[i], d)
		}
	}
}

func TestIntervals_EmptyList(t *testing.T) {
	setRequiredEnvVars(t)
	f := writeTempConfig(t, "smtp:\n  host: localhost\n")

	var cfg Config
	if err := cleanenv.ReadConfig(f, &cfg); err != nil {
		t.Fatalf("cleanenv.ReadConfig: %v", err)
	}

	if len(cfg.Intervals) != 0 {
		t.Errorf("expected empty intervals, got %v", cfg.Intervals)
	}
}

func TestIntervals_InvalidDuration(t *testing.T) {
	setRequiredEnvVars(t)
	const content = `
notification_intervals:
  - "not-a-duration"
`
	f := writeTempConfig(t, content)

	var cfg Config
	err := cleanenv.ReadConfig(f, &cfg)
	if err == nil {
		t.Errorf("expected error for invalid duration, got nil; parsed: %v", cfg.Intervals)
	}
}

func TestIntervals_SingleEntry(t *testing.T) {
	setRequiredEnvVars(t)
	const content = `
notification_intervals:
  - "1h30m"
`
	f := writeTempConfig(t, content)

	var cfg Config
	if err := cleanenv.ReadConfig(f, &cfg); err != nil {
		t.Fatalf("cleanenv.ReadConfig: %v", err)
	}

	if len(cfg.Intervals) != 1 {
		t.Fatalf("got %d intervals, want 1", len(cfg.Intervals))
	}
	if cfg.Intervals[0] != 90*time.Minute {
		t.Errorf("Intervals[0] = %v, want %v", cfg.Intervals[0], 90*time.Minute)
	}
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
