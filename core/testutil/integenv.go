// Package testutil provides shared helpers for integration tests.
package testutil

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	integEnvOnce sync.Once
	integEnvVars map[string]string
)

func loadIntegEnvFile() map[string]string {
	integEnvOnce.Do(func() {
		integEnvVars = map[string]string{}
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		f, err := os.Open(filepath.Join(home, ".config", "osdd", ".env.integ-test"))
		if err != nil {
			return
		}
		defer func() { _ = f.Close() }()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if k, v, ok := strings.Cut(line, "="); ok {
				integEnvVars[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
	})
	return integEnvVars
}

// IntegEnv returns the value of key from the environment, falling back to
// ~/.config/osdd/.env.integ-test if the env var is not set.
func IntegEnv(key string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return loadIntegEnvFile()[key]
}
