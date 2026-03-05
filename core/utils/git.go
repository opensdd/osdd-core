package utils

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/opensdd/osdd-api/clients/go/osdd"
)

// BuildGitCloneURL constructs an HTTPS clone URL for the given GitRepository.
// It maps Provider to the appropriate host, and optionally embeds the given
// auth token. If token is empty the URL is returned without authentication.
func BuildGitCloneURL(repo *osdd.GitRepository, token string) (string, error) {
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

	if token != "" {
		return fmt.Sprintf("https://%s:%s@%s/%s.git", tokenUser, token, host, fullName), nil
	}

	return fmt.Sprintf("https://%s/%s.git", host, fullName), nil
}

// CloneOptions configures the behaviour of CloneGitRepo.
type CloneOptions struct {
	ShallowSince string // e.g. "2006-01-02"
}

// CloneGitRepo clones the repository described by repo into destPath using the git CLI.
// The token is embedded in the clone URL when non-empty.
// When opts is nil, a full (non-bare) clone is performed.
func CloneGitRepo(ctx context.Context, repo *osdd.GitRepository, destPath string, token string, opts *CloneOptions) error {
	if repo == nil {
		return fmt.Errorf("git repository cannot be nil")
	}

	url, err := BuildGitCloneURL(repo, token)
	if err != nil {
		return fmt.Errorf("failed to build clone URL: %w", err)
	}

	slog.Debug("Cloning git repository", "fullName", repo.GetFullName(), "provider", repo.GetProvider(), "dest", destPath)

	args := buildCloneArgs(opts, url, destPath)
	cmd := exec.CommandContext(ctx, "git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %w (output: %s)", err, string(output))
	}

	slog.Debug("Git clone successful", "dest", destPath)
	return nil
}

// buildCloneArgs constructs the argument list for `git clone`.
func buildCloneArgs(opts *CloneOptions, cloneURL, destPath string) []string {
	args := []string{"clone"}
	if opts != nil {
		if opts.ShallowSince != "" {
			args = append(args, "--shallow-since="+opts.ShallowSince)
		}
	}
	args = append(args, cloneURL, destPath)
	return args
}
