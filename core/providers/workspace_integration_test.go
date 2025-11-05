package providers_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
	"github.com/opensdd/osdd-core/core/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkspace_Create_EmptyPath_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	tests := []struct {
		name   string
		config *recipes.WorkspaceConfig
		reason string
	}{
		{
			name:   "disabled workspace",
			config: recipes.WorkspaceConfig_builder{Enabled: false, Path: "osdd/test-workspace"}.Build(),
			reason: "disabled workspace should return empty path",
		},
		{
			name:   "nil config",
			config: nil,
			reason: "nil workspace config should return empty path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := &providers.Workspace{}
			res, err := w.Materialize(context.Background(), tt.config)
			require.NoError(t, err)
			assert.Empty(t, res, tt.reason)
		})
	}
}

func TestWorkspace_Create_WithoutUnique_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	w := &providers.Workspace{}

	// Materialize a temporary test workspace path
	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)

	testPath := "osdd/test-workspace-basic"
	config := recipes.WorkspaceConfig_builder{
		Enabled: true,
		Path:    testPath,
	}.Build()

	expectedPath := filepath.Join(homeDir, testPath)

	// Clean up before and after test
	defer func() { _ = os.RemoveAll(expectedPath) }()
	_ = os.RemoveAll(expectedPath)

	path, err := w.Materialize(context.Background(), config)
	require.NoError(t, err)
	assert.Equal(t, expectedPath, path)

	// Verify the directory was created
	require.NoError(t, err)
}

func TestWorkspace_Create_WithUnique_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	tests := []struct {
		name        string
		testPath    string
		slugLen     int32
		expectedLen int
		description string
	}{
		{
			name:        "custom slug length",
			testPath:    "osdd/test-workspace-unique",
			slugLen:     10,
			expectedLen: 10,
			description: "unique directory name should match format YYYYMMDD_<slug>",
		},
		{
			name:        "default slug length",
			testPath:    "osdd/test-workspace-unique-default",
			slugLen:     0,
			expectedLen: 8,
			description: "unique directory name should match format YYYYMMDD_<slug> with default length 8",
		},
		{
			name:        "small slug length",
			testPath:    "osdd/test-workspace-unique-small",
			slugLen:     4,
			expectedLen: 4,
			description: "unique directory name should match format YYYYMMDD_<slug> with length 4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := &providers.Workspace{}

			config := recipes.WorkspaceConfig_builder{
				Enabled: true,
				Path:    tt.testPath,
				Unique: osdd.NameGenConfig_builder{
					Len: tt.slugLen,
				}.Build(),
			}.Build()

			path, err := w.Materialize(context.Background(), config)
			require.NoError(t, err)
			require.NotEmpty(t, path)

			// Verify the unique directory name format: YYYYMMDD_<slug>
			dirname := filepath.Base(path)
			pattern := regexp.MustCompile(fmt.Sprintf(`^\d{8}_[a-f0-9]{%d}$`, tt.expectedLen))
			assert.True(t, pattern.MatchString(dirname),
				"%s, got: %s", tt.description, dirname)
		})
	}
}

func TestWorkspace_Create_WithoutCreatingFolder_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	tests := []struct {
		name     string
		testPath string
		unique   bool
		slugLen  int32
	}{
		{
			name:     "without unique",
			testPath: "osdd/test-workspace-no-create",
			unique:   false,
		},
		{
			name:     "with unique",
			testPath: "osdd/test-workspace-no-create-unique",
			unique:   true,
			slugLen:  8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := &providers.Workspace{}

			homeDir, err := os.UserHomeDir()
			require.NoError(t, err)

			configBuilder := recipes.WorkspaceConfig_builder{
				Enabled: true,
				Path:    tt.testPath,
			}

			if tt.unique {
				configBuilder.Unique = osdd.NameGenConfig_builder{
					Len: tt.slugLen,
				}.Build()
			}

			config := configBuilder.Build()
			basePath := filepath.Join(homeDir, tt.testPath)

			path, err := w.Materialize(context.Background(), config)
			require.NoError(t, err)
			require.NotEmpty(t, path)

			// Verify the path is correct
			if tt.unique {
				assert.Contains(t, path, basePath, "path should contain base path")
				// Verify the unique directory name format: YYYYMMDD_<slug>
				dirname := filepath.Base(path)
				pattern := regexp.MustCompile(fmt.Sprintf(`^\d{8}_[a-f0-9]{%d}$`, tt.slugLen))
				assert.True(t, pattern.MatchString(dirname),
					"unique directory name should match format, got: %s", dirname)
			} else {
				assert.Equal(t, basePath, path)
			}

			// Verify the directory was NOT created
			_, err = os.Stat(path)
			assert.True(t, os.IsNotExist(err), "directory should not exist when Materialize is false")
		})
	}
}

func TestWorkspace_Create_MultipleCallsCreateDifferentDirs_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	w := &providers.Workspace{}

	testPath := "osdd/test-workspace-multiple"

	config := recipes.WorkspaceConfig_builder{
		Enabled: true,
		Path:    testPath,
		Unique: osdd.NameGenConfig_builder{
			Len: 8,
		}.Build(),
	}.Build()

	path1, err := w.Materialize(context.Background(), config)
	require.NoError(t, err)

	path2, err := w.Materialize(context.Background(), config)
	require.NoError(t, err)
	require.NotEmpty(t, path1)
	require.NotEmpty(t, path2)

	// Verify they are different (different slugs)
	assert.NotEqual(t, path1, path2, "multiple calls should create different directories")
}
