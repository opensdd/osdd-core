package generators

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
	"github.com/opensdd/osdd-core/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContext_IntegrationTest_TextSource(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	c := &Context{}

	ctx := recipes.Context_builder{
		Entries: []*recipes.ContextEntry{
			recipes.ContextEntry_builder{
				Path: "output.txt",
				From: recipes.ContextFrom_builder{
					Text: strPtr("integration test content"),
				}.Build(),
			}.Build(),
		},
	}.Build()

	result, err := c.Materialize(context.Background(), ctx, nil)
	require.NoError(t, err)
	require.Len(t, result.GetEntries(), 1)

	entry := result.GetEntries()[0]
	require.True(t, entry.HasFile())
	file := entry.GetFile()
	assert.Equal(t, "output.txt", file.GetPath())
	assert.Equal(t, "integration test content", file.GetContent())
}

func TestContext_IntegrationTest_CmdSource(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	c := &Context{}

	ctx := recipes.Context_builder{
		Entries: []*recipes.ContextEntry{
			recipes.ContextEntry_builder{
				Path: "cmd_output.txt",
				From: recipes.ContextFrom_builder{
					Cmd: ex("echo", "command result"),
				}.Build(),
			}.Build(),
		},
	}.Build()

	result, err := c.Materialize(context.Background(), ctx, nil)
	require.NoError(t, err)
	require.Len(t, result.GetEntries(), 1)

	entry := result.GetEntries()[0]
	require.True(t, entry.HasFile())
	file := entry.GetFile()
	assert.Equal(t, "cmd_output.txt", file.GetPath())
	assert.Equal(t, "command result\n", file.GetContent())
}

func TestContext_IntegrationTest_MultipleEntries(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	c := &Context{}

	ctx := recipes.Context_builder{
		Entries: []*recipes.ContextEntry{
			recipes.ContextEntry_builder{
				Path: "file1.txt",
				From: recipes.ContextFrom_builder{
					Text: strPtr("content 1"),
				}.Build(),
			}.Build(),
			recipes.ContextEntry_builder{
				Path: "file2.txt",
				From: recipes.ContextFrom_builder{
					Cmd: ex("echo", "content 2"),
				}.Build(),
			}.Build(),
		},
	}.Build()

	result, err := c.Materialize(context.Background(), ctx, nil)
	require.NoError(t, err)
	require.Len(t, result.GetEntries(), 2)

	// Verify first entry
	entry1 := result.GetEntries()[0]
	require.True(t, entry1.HasFile())
	file1 := entry1.GetFile()
	assert.Equal(t, "file1.txt", file1.GetPath())
	assert.Equal(t, "content 1", file1.GetContent())

	// Verify second entry
	entry2 := result.GetEntries()[1]
	require.True(t, entry2.HasFile())
	file2 := entry2.GetFile()
	assert.Equal(t, "file2.txt", file2.GetPath())
	assert.Equal(t, "content 2\n", file2.GetContent())
}

func TestContext_IntegrationTest_FailFast(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	c := &Context{}

	ctx := recipes.Context_builder{
		Entries: []*recipes.ContextEntry{
			recipes.ContextEntry_builder{
				Path: "file1.txt",
				From: recipes.ContextFrom_builder{
					Cmd: ex("exit", "1"), // This will fail
				}.Build(),
			}.Build(),
			recipes.ContextEntry_builder{
				Path: "file2.txt",
				From: recipes.ContextFrom_builder{
					Text: strPtr("should not be materialized"),
				}.Build(),
			}.Build(),
		},
	}.Build()

	_, err := c.Materialize(context.Background(), ctx, nil)
	assert.Error(t, err, "expected error due to failed command")
}

func TestContext_IntegrationTest_RealGithubFetch(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	c := &Context{}

	ctx := recipes.Context_builder{
		Entries: []*recipes.ContextEntry{
			recipes.ContextEntry_builder{
				Path: "README.md",
				From: recipes.ContextFrom_builder{
					Github: osdd.GitReference_builder{
						Path: "https://github.com/devplaninc/devplan-cli/blob/main/README.md",
					}.Build(),
				}.Build(),
			}.Build(),
		},
	}.Build()

	result, err := c.Materialize(context.Background(), ctx, nil)
	require.NoError(t, err, "unexpected error fetching from GitHub")
	require.Len(t, result.GetEntries(), 1)

	entry := result.GetEntries()[0]
	require.True(t, entry.HasFile())
	file := entry.GetFile()
	assert.Equal(t, "README.md", file.GetPath())

	// Basic validation - README should contain the word "devplan"
	content := file.GetContent()
	assert.NotEmpty(t, content, "fetched content is empty")
	assert.Contains(t, strings.ToLower(content), "devplan", "fetched content doesn't appear to be the devplan README")
}

func TestContext_IntegrationTest_LocalFileSource(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	c := &Context{}

	tmp := t.TempDir()
	p := filepath.Join(tmp, "local.txt")
	want := "local file integration content"
	require.NoError(t, os.WriteFile(p, []byte(want), 0o644))

	ctx := recipes.Context_builder{
		Entries: []*recipes.ContextEntry{
			recipes.ContextEntry_builder{
				Path: "from_local.txt",
				From: recipes.ContextFrom_builder{
					LocalFile: &p,
				}.Build(),
			}.Build(),
		},
	}.Build()

	result, err := c.Materialize(context.Background(), ctx, &core.GenerationContext{})
	require.NoError(t, err)
	require.Len(t, result.GetEntries(), 1)

	entry := result.GetEntries()[0]
	require.True(t, entry.HasFile())
	file := entry.GetFile()
	assert.Equal(t, "from_local.txt", file.GetPath())
	assert.Equal(t, want, file.GetContent())
}

func TestContext_IntegrationTest_Combined_LocalFileItem(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	c := &Context{}

	tmp := t.TempDir()
	p := filepath.Join(tmp, "data.txt")
	fileContent := "DATA_FROM_FILE"
	require.NoError(t, os.WriteFile(p, []byte(fileContent), 0o644))

	ctx := recipes.Context_builder{
		Entries: []*recipes.ContextEntry{
			recipes.ContextEntry_builder{
				Path: "combined.txt",
				From: recipes.ContextFrom_builder{
					Combined: recipes.CombinedContextSource_builder{
						Items: []*recipes.CombinedContextSource_Item{
							recipes.CombinedContextSource_Item_builder{Text: strPtr("prefix:")}.Build(),
							recipes.CombinedContextSource_Item_builder{LocalFile: &p}.Build(),
							recipes.CombinedContextSource_Item_builder{Text: strPtr(":suffix")}.Build(),
						},
					}.Build(),
				}.Build(),
			}.Build(),
		},
	}.Build()

	result, err := c.Materialize(context.Background(), ctx, &core.GenerationContext{})
	require.NoError(t, err)
	require.Len(t, result.GetEntries(), 1)

	entry := result.GetEntries()[0]
	require.True(t, entry.HasFile())
	file := entry.GetFile()
	assert.Equal(t, "combined.txt", file.GetPath())
	assert.Equal(t, "prefix:"+fileContent+":suffix", file.GetContent())
}
