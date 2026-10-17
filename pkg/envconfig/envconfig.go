// Package envconfig loads the single repository .env before application startup.
package envconfig

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// Load preserves process variables, ignores nested .env files, and anchors config
// paths to the selected .env directory. ENV_FILE is a process-only override.
// Without ENV_FILE, go.work identifies the repository root. Outside a checkout
// (for example in a container), configuration comes only from the environment.
func Load() error {
	return loadFrom(os.Getwd)
}

func loadFrom(getwd func() (string, error)) error {
	cwd, err := getwd()
	if err != nil {
		return fmt.Errorf("determine working directory: %w", err)
	}
	root := ""
	file, explicit := os.LookupEnv("ENV_FILE")
	if explicit {
		if file == "" {
			return fmt.Errorf("ENV_FILE is empty: specify a path or unset it for environment-only startup")
		}
		if !filepath.IsAbs(file) {
			file = filepath.Join(cwd, file)
		}
		file = filepath.Clean(file)
		root = filepath.Dir(file)
	} else {
		for dir := cwd; ; dir = filepath.Dir(dir) {
			if info, statErr := os.Stat(filepath.Join(dir, "go.work")); statErr == nil && !info.IsDir() {
				root = dir
				file = filepath.Join(root, ".env")
				break
			}
			if filepath.Dir(dir) == dir {
				break
			}
		}
	}

	if file != "" {
		values, readErr := godotenv.Read(file)
		if readErr != nil {
			if explicit || !os.IsNotExist(readErr) {
				// Parser errors may contain secret values. Never include their text.
				return fmt.Errorf("cannot read or parse ENV_FILE %s", file)
			}
		} else {
			for key, value := range values {
				if _, exists := os.LookupEnv(key); !exists && key != "ENV_FILE" {
					if err := os.Setenv(key, value); err != nil {
						return fmt.Errorf("cannot set environment variable %s", key)
					}
				}
			}
		}
	}
	for key, fallback := range map[string]string{
		"CONFIG_PATH":       "backend/config/local.yaml",
		"EMAIL_CONFIG_PATH": "notifiers/email/config/local.yaml",
	} {
		value := os.Getenv(key)
		if value == "" {
			if root == "" {
				continue
			}
			value = fallback
		}
		if !filepath.IsAbs(value) {
			if root == "" {
				return fmt.Errorf("%s must be absolute outside a repository; alternatively set ENV_FILE", key)
			}
			value = filepath.Join(root, value)
		}
		if err := os.Setenv(key, filepath.Clean(value)); err != nil {
			return fmt.Errorf("cannot set %s", key)
		}
	}
	return nil
}
