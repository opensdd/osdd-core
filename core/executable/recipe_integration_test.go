package executable_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
	"github.com/opensdd/osdd-core/core"
	"github.com/opensdd/osdd-core/core/executable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestExecutableRecipe_Materialize_FromJSON_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	// Load the sample recipe JSON from testdata
	jsonPath := filepath.Join("testdata", "recipe1.json")
	data, err := os.ReadFile(jsonPath)
	require.NoError(t, err, "failed to read %s", jsonPath)

	// Unmarshal JSON into recipes.Recipe using protojson with unknown fields discarded
	recipe := &recipes.Recipe{}
	require.NoError(t, protojson.Unmarshal(data, recipe), "failed to unmarshal recipe JSON")

	// Wrap it into an ExecutableRecipe specifying the IDE type
	exec := recipes.ExecutableRecipe_builder{
		EntryPoint: recipes.EntryPoint_builder{IdeType: "claude"}.Build(),
		Recipe:     recipe,
	}.Build()

	// Materialize through the executable layer
	r := executable.ForRecipe(exec)
	res, err := r.Materialize(context.Background(), &core.GenerationContext{})
	require.NoError(t, err)
	require.NotNil(t, res)

	// Build a map of path -> content
	got := map[string]string{}
	for _, e := range res.GetEntries() {
		if e != nil && e.GetFile() != nil {
			got[e.GetFile().GetPath()] = e.GetFile().GetContent()
		}
	}

	// Expect files from context and IDE commands, plus Claude settings
	expectedPaths := []string{
		"specs/goal.md",
		"specs/parameters.md",
		".claude/commands/agents_md_plan.md",
		".claude/commands/agents_md.md",
		".claude/commands/agents_md_generate.md",
		".claude/settings.local.json",
	}

	for _, p := range expectedPaths {
		if !assert.Contains(t, got, p, "missing materialized entry for %s", p) {
			// If the path is missing, continue to show all missing paths
			continue
		}
		// All files should have some content
		assert.NotEmpty(t, got[p], "file %s should have non-empty content", p)
	}

	// parameters.md should be a Markdown with header even without values provided
	assert.Contains(t, got["specs/parameters.md"], "# User Input")

	// Sanity: we expect at least these entries
	assert.GreaterOrEqual(t, len(got), len(expectedPaths))
}
