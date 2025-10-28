package utils

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/opensdd/osdd-api/clients/go/osdd"
)

// ConvertToRawURL converts a github.com URL to raw.githubusercontent.com format.
// It handles various GitHub URL formats including /blob/ and /tree/ patterns.
// If a version is provided, it will be used; otherwise defaults to "main" branch.
func ConvertToRawURL(githubPath string, version *osdd.GitVersion) (string, error) {
	// If it's already a raw.githubusercontent.com URL or doesn't contain github.com, return as-is
	if strings.Contains(githubPath, "raw.githubusercontent.com") || !strings.Contains(githubPath, "github.com") {
		return githubPath, nil
	}

	// Convert github.com URL to raw.githubusercontent.com
	// Example: https://github.com/myorg/repo/blob/main/README.MD
	// To: https://raw.githubusercontent.com/myorg/repo/main/README.MD

	githubPath = strings.TrimPrefix(githubPath, "https://")
	githubPath = strings.TrimPrefix(githubPath, "http://")
	githubPath = strings.TrimPrefix(githubPath, "github.com/")

	// Handle both formats:
	// 1. owner/repo/file.md (no ref specified)
	// 2. owner/repo/blob/ref/file.md or owner/repo/tree/ref/file.md

	parts := strings.SplitN(githubPath, "/", 5)

	var owner, repo, ref, filePath string

	if len(parts) >= 4 && (parts[2] == "blob" || parts[2] == "tree") {
		// Format: owner/repo/blob|tree/ref/file.md
		if len(parts) < 5 {
			return "", fmt.Errorf("invalid github path format: %s", githubPath)
		}
		owner = parts[0]
		repo = parts[1]
		ref = parts[3]
		filePath = parts[4]
	} else if len(parts) >= 3 {
		// Format: owner/repo/file.md
		owner = parts[0]
		repo = parts[1]
		filePath = strings.Join(parts[2:], "/")

		// Use version from parameter if provided
		ref = "main"
		if version != nil && version.HasType() {
			switch version.WhichType() {
			case osdd.GitVersion_Tag_case:
				ref = version.GetTag()
			case osdd.GitVersion_Commit_case:
				ref = version.GetCommit()
			}
		}
	} else {
		return "", fmt.Errorf("invalid github path format: %s", githubPath)
	}

	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", owner, repo, ref, filePath), nil
}

// FetchGithub fetches the content of a GitHub file reference using a raw content URL.
// If the provided ref.Path is not a github.com URL, it is used as-is.
func FetchGithub(ctx context.Context, ref *osdd.GitReference) (string, error) {
	if ref == nil {
		return "", fmt.Errorf("github reference cannot be nil")
	}

	githubPath := ref.GetPath()
	if githubPath == "" {
		return "", fmt.Errorf("github path cannot be empty")
	}

	url, err := ConvertToRawURL(githubPath, ref.GetVersion())
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch from github: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github fetch returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	return string(body), nil
}
