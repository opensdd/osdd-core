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

type Workspace struct {
}

// Materialize creates a new workspace according to the config. Returns a path to the workspace.
// If workspace is not enabled, return an empty path. If a "unique" property is provided, it generates
// a new unique directory within the workspace's path. That new folder will have the format
// `<timestamp as YYYYMMDD>_<slug of the len fron unique property>`.
func (w *Workspace) Materialize(_ context.Context, ws *recipes.WorkspaceConfig) (string, error) {
	if ws == nil || !ws.GetEnabled() {
		return "", nil
	}
	workspacePath := ws.GetPath()

	if !ws.GetAbsolute() {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		workspacePath = filepath.Join(homeDir, ws.GetPath())
	}

	if ws.HasUnique() {
		unique := ws.GetUnique()
		slugLen := int(unique.GetLen())
		if slugLen <= 0 {
			slugLen = 8
		}

		timestamp := time.Now().Format("20060102")

		slug, err := generateRandomSlug(slugLen)
		if err != nil {
			return "", fmt.Errorf("failed to generate slug: %w", err)
		}

		workspacePath = filepath.Join(workspacePath, fmt.Sprintf("%s_%s", timestamp, slug))
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
