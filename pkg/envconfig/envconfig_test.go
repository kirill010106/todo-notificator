package envconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func unset(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		t.Setenv(key, "") // restore the original process environment on cleanup
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
}

func write(t *testing.T, file, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestRootFileFromEveryServiceDirectory(t *testing.T) {
	for _, dir := range []string{".", "backend", "notifiers/email", "activity-logger", "notifiers/telegram-bot"} {
		t.Run(dir, func(t *testing.T) {
			unset(t, "ENV_FILE", "SMTP_FROM", "CONFIG_PATH", "EMAIL_CONFIG_PATH")
			root := t.TempDir()
			write(t, filepath.Join(root, "go.work"), "go 1.26.0\n")
			write(t, filepath.Join(root, ".env"), "SMTP_FROM=new@example.com\nCONFIG_PATH=backend/config/local.yaml\nEMAIL_CONFIG_PATH=notifiers/email/config/local.yaml\n")
			for _, nested := range []string{"backend", "notifiers/email", "activity-logger", "notifiers/telegram-bot"} {
				write(t, filepath.Join(root, nested, ".env"), "SMTP_FROM=old@example.com\n")
			}
			t.Chdir(filepath.Join(root, dir))
			if err := Load(); err != nil {
				t.Fatal(err)
			}
			if got := os.Getenv("SMTP_FROM"); got != "new@example.com" {
				t.Fatalf("nested file won: %s", got)
			}
			for key, relative := range map[string]string{"CONFIG_PATH": "backend/config/local.yaml", "EMAIL_CONFIG_PATH": "notifiers/email/config/local.yaml"} {
				if got := os.Getenv(key); got != filepath.Join(root, relative) {
					t.Fatalf("%s resolved to %s", key, got)
				}
			}
		})
	}
}

func TestExplicitFileAndProcessPriority(t *testing.T) {
	unset(t, "ENV_FILE", "SMTP_FROM", "CONFIG_PATH", "EMAIL_CONFIG_PATH")
	root := t.TempDir()
	file := filepath.Join(root, "selected", ".env")
	write(t, file, "SMTP_FROM=file@example.com\nCONFIG_PATH=backend/config/local.yaml\nENV_FILE=ignored.env\n")
	t.Chdir(root)
	t.Setenv("ENV_FILE", "selected/.env")
	t.Setenv("SMTP_FROM", "process@example.com")
	t.Setenv("EMAIL_CONFIG_PATH", filepath.Join(root, "absolute.yaml"))
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("SMTP_FROM") != "process@example.com" || os.Getenv("ENV_FILE") != "selected/.env" {
		t.Fatal("file overrode the process environment")
	}
	if os.Getenv("CONFIG_PATH") != filepath.Join(root, "selected/backend/config/local.yaml") || os.Getenv("EMAIL_CONFIG_PATH") != filepath.Join(root, "absolute.yaml") {
		t.Fatal("incorrect path base or absolute override")
	}
}

func TestEnvironmentOnlyContainer(t *testing.T) {
	unset(t, "ENV_FILE", "SMTP_FROM", "CONFIG_PATH", "EMAIL_CONFIG_PATH")
	root := t.TempDir()
	config := filepath.Join(root, "config/prod.yaml")
	t.Setenv("CONFIG_PATH", config)
	// Use a synthetic container cwd outside the checkout: the test temp directory
	// may itself live below the real repository in sandboxed environments.
	containerCwd := filepath.Join(filepath.VolumeName(root)+string(os.PathSeparator), "envconfig-container")
	if err := loadFrom(func() (string, error) { return containerCwd, nil }); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("SMTP_FROM") != "" || os.Getenv("CONFIG_PATH") != config || os.Getenv("EMAIL_CONFIG_PATH") != "" {
		t.Fatal("environment-only startup read a file or changed container paths")
	}
}

func TestMissingRootFileAllowsProcessEnvironment(t *testing.T) {
	unset(t, "ENV_FILE", "CONFIG_PATH", "EMAIL_CONFIG_PATH")
	root := t.TempDir()
	write(t, filepath.Join(root, "go.work"), "go 1.26.0\n")
	t.Chdir(root)
	if err := Load(); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("CONFIG_PATH") != filepath.Join(root, "backend/config/local.yaml") {
		t.Fatal("default path is not rooted")
	}
}

func TestExplicitMissingOrMalformedFileFailsWithoutSecrets(t *testing.T) {
	for _, content := range []string{"", "PRIVATE-VALUE=secret\n"} {
		t.Run(content, func(t *testing.T) {
			unset(t, "ENV_FILE", "CONFIG_PATH", "EMAIL_CONFIG_PATH")
			file := filepath.Join(t.TempDir(), ".env")
			if content != "" {
				write(t, file, content)
			}
			t.Setenv("ENV_FILE", file)
			err := Load()
			if err == nil || strings.Contains(err.Error(), "PRIVATE-VALUE") || strings.Contains(err.Error(), "secret") {
				t.Fatalf("expected a sanitized error, got %v", err)
			}
		})
	}
}

func TestRelativeConfigOutsideRepositoryFails(t *testing.T) {
	unset(t, "ENV_FILE", "CONFIG_PATH", "EMAIL_CONFIG_PATH")
	containerCwd := filepath.Join(filepath.VolumeName(t.TempDir())+string(os.PathSeparator), "envconfig-container")
	t.Setenv("CONFIG_PATH", "config/local.yaml")
	if err := loadFrom(func() (string, error) { return containerCwd, nil }); err == nil {
		t.Fatal("relative config outside a repository is ambiguous")
	}
}
