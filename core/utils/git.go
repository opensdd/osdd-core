package utils

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/opensdd/osdd-api/clients/go/osdd"
)

// BuildGitCloneURL constructs an HTTPS clone URL for the given GitRepository.
// It maps Provider to the appropriate host, and optionally embeds an auth token
// read from the environment variable named by AuthTokenEnvVar.
// If the env var is not set or empty the URL is returned without authentication.
func BuildGitCloneURL(repo *osdd.GitRepository) (string, error) {
	if repo == nil {
		return "", fmt.Errorf("git repository cannot be nil")
	}
	fullName := strings.TrimSpace(repo.GetFullName())
	if fullName == "" {
		return "", fmt.Errorf("git repository full name cannot be empty")
	}

	provider := strings.ToLower(strings.TrimSpace(repo.GetProvider()))

	var host, tokenUser string
	switch provider {
	case "", "github":
		host = "github.com"
		tokenUser = "x-access-token"
	case "bitbucket":
		host = "bitbucket.org"
		tokenUser = "x-token-auth"
	default:
		return "", fmt.Errorf("unsupported git provider: %s", provider)
	}

	// Check for auth token from environment variable.
	if repo.HasAuthTokenEnvVar() {
		envVar := repo.GetAuthTokenEnvVar()
		if envVar != "" {
			token := os.Getenv(envVar)
			if token != "" {
				return fmt.Sprintf("https://%s:%s@%s/%s.git", tokenUser, token, host, fullName), nil
			}
		}
	}

	return fmt.Sprintf("https://%s/%s.git", host, fullName), nil
}

// CloneGitRepo clones the repository described by repo into destPath using the git CLI.
// It always attempts the clone even when no auth token is available.
func CloneGitRepo(ctx context.Context, repo *osdd.GitRepository, destPath string) error {
	if repo == nil {
		return fmt.Errorf("git repository cannot be nil")
	}

	url, err := BuildGitCloneURL(repo)
	if err != nil {
		return fmt.Errorf("failed to build clone URL: %w", err)
	}

	slog.Debug("Cloning git repository", "fullName", repo.GetFullName(), "provider", repo.GetProvider(), "dest", destPath)

	cmd := exec.CommandContext(ctx, "git", "clone", url, destPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %w (output: %s)", err, string(output))
	}

	slog.Debug("Git clone successful", "dest", destPath)
	return nil
}
