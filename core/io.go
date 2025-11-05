package core

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/opensdd/osdd-api/clients/go/osdd"
)

// PersistMaterializedResult writes all file entries from MaterializedResult into the filesystem under the given root directory.
// - root: base directory where files will be written.
// - result: materialized content to persist.
// Behavior:
// - Creates parent directories as needed (0755 perms).
// - Overwrites existing files (0644 perms).
// - Skips entries that do not contain a file.
// - Rejects paths that escape the provided root via path traversal.
func PersistMaterializedResult(_ context.Context, root string, result *osdd.MaterializedResult) error {
	log := slog.With("op", "PersistMaterializedResult")
	if strings.TrimSpace(root) == "" {
		return fmt.Errorf("root path cannot be empty")
	}
	if result == nil {
		return fmt.Errorf("materialized result cannot be nil")
	}

	root = filepath.Clean(root)

	entries := result.GetEntries()
	if len(entries) == 0 {
		return nil
	}

	for i, e := range entries {
		if e == nil || !e.HasFile() {
			continue
		}
		f := e.GetFile()
		if f == nil {
			continue
		}
		p := strings.TrimSpace(f.GetPath())
		if p == "" {
			return fmt.Errorf("entry %d: file path cannot be empty", i)
		}

		// Clean and resolve the path under root.
		rel := filepath.Clean(p)
		// Disallow absolute paths by making them relative.
		if filepath.IsAbs(rel) {
			// turn "/abs/path" into "abs/path"
			rel = strings.TrimPrefix(rel, string(os.PathSeparator))
		}
		full := filepath.Join(root, rel)
		full = filepath.Clean(full)

		// Ensure the target path is within root (prevent path traversal).
		if !isPathWithinRoot(root, full) {
			return fmt.Errorf("entry %d: path escapes root: %s", i, p)
		}

		// Materialize parent directories.
		dir := filepath.Dir(full)
		log.Debug("Creating directory", "dir", dir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("entry %d: failed to create directories for %s: %w", i, full, err)
		}

		// Write file (overwrite if exists).
		log.Debug("Writing file", "rel", rel, "full", full)
		if err := os.WriteFile(full, []byte(f.GetContent()), 0o644); err != nil {
			return fmt.Errorf("entry %d: failed to write file %s: %w", i, full, err)
		}
	}
	return nil
}

// isPathWithinRoot checks whether target is inside root directory.
func isPathWithinRoot(root, target string) bool {
	rootClean := filepath.Clean(root)
	targetClean := filepath.Clean(target)

	// On case-insensitive filesystems, filepath.Rel handles appropriately; use rel to check traversal.
	rel, err := filepath.Rel(rootClean, targetClean)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." {
		return false
	}
	if strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false
	}
	return true
}
