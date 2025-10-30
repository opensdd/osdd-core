package providers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
)

type WorkspaceOptions struct {
	// Create indicates whether to create the required folders or not.
	Create bool
}

type Workspace struct {
}

// Create creates a new workspace according to the config. Returns a path to the workspace.
// If workspace is not enabled, return an empty path. If a "unique" property is provided, it generates
// a new unique directory within the workspace's path. That new folder will have the format
// `<timestamp as YYYYMMDD>_<slug of the len fron unique property>`.
func (w *Workspace) Create(_ context.Context, ws *recipes.WorkspaceConfig, opts WorkspaceOptions) (string, error) {
	if ws == nil || !ws.GetEnabled() {
		return "", nil
	}

	// Get home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	// Build base workspace path
	workspacePath := filepath.Join(homeDir, ws.GetPath())

	// If unique config is provided, append unique subdirectory
	if ws.HasUnique() {
		unique := ws.GetUnique()
		slugLen := int(unique.GetLen())
		if slugLen <= 0 {
			slugLen = 8 // default slug length
		}

		// Generate timestamp as YYYYMMDD
		timestamp := time.Now().Format("20060102")

		// Generate random slug
		slug, err := generateRandomSlug(slugLen)
		if err != nil {
			return "", fmt.Errorf("failed to generate slug: %w", err)
		}

		// Append unique directory name
		workspacePath = filepath.Join(workspacePath, fmt.Sprintf("%s_%s", timestamp, slug))
	}

	// Create the directory if opts.Create is true
	if opts.Create {
		if err := os.MkdirAll(workspacePath, 0755); err != nil {
			return "", fmt.Errorf("failed to create workspace directory: %w", err)
		}
	}

	return workspacePath, nil
}

// generateRandomSlug creates a random hex string of the specified length
func generateRandomSlug(length int) (string, error) {
	// We need length/2 bytes to get length hex characters
	bytes := make([]byte, (length+1)/2)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	slug := hex.EncodeToString(bytes)
	// Trim to exact length in case of odd numbers
	if len(slug) > length {
		slug = slug[:length]
	}
	return slug, nil
}
