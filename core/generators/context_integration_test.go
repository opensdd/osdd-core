package generators

import (
	"bytes"
	"context"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"time"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
	"github.com/opensdd/osdd-core/core"
	"github.com/opensdd/osdd-core/core/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var updateGolden = flag.Bool("update", false, "update golden testdata files")

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

	result, err := c.Materialize(context.Background(), ctx, &core.GenerationContext{})
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

	result, err := c.Materialize(context.Background(), ctx, &core.GenerationContext{})
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

	result, err := c.Materialize(context.Background(), ctx, &core.GenerationContext{})
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

	_, err := c.Materialize(context.Background(), ctx, &core.GenerationContext{})
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

	result, err := c.Materialize(context.Background(), ctx, &core.GenerationContext{})
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

func TestContext_IntegrationTest_GitRepoSource(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	c := &Context{}

	workspace := t.TempDir()

	ctx := recipes.Context_builder{
		Entries: []*recipes.ContextEntry{
			recipes.ContextEntry_builder{
				Path: "checkout",
				From: recipes.ContextFrom_builder{
					GitRepo: osdd.GitRepository_builder{
						FullName: "opensdd/osdd-api",
						Provider: "github",
					}.Build(),
				}.Build(),
			}.Build(),
		},
	}.Build()

	genCtx := &core.GenerationContext{WorkspacePath: workspace}
	result, err := c.Materialize(context.Background(), ctx, genCtx)
	require.NoError(t, err, "unexpected error cloning git repository")
	require.Len(t, result.GetEntries(), 1)

	entry := result.GetEntries()[0]
	require.True(t, entry.HasDirectory(), "expected a Directory entry")
	assert.Equal(t, "checkout", entry.GetDirectory())

	// Verify the clone directory exists and has a .git folder.
	clonedPath := filepath.Join(workspace, "checkout")
	info, err := os.Stat(filepath.Join(clonedPath, ".git"))
	require.NoError(t, err, ".git directory should exist in cloned repo")
	assert.True(t, info.IsDir())
}

func TestContext_IntegrationTest_GitRepoWithGhAuth(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	// Skip if gh CLI is not installed.
	if _, err := exec.LookPath("gh"); err != nil {
		t.Skip("gh CLI not installed, skipping authenticated git clone test")
	}

	// Get a token from gh auth.
	var tokenBuf bytes.Buffer
	ghCmd := exec.Command("gh", "auth", "token")
	ghCmd.Stdout = &tokenBuf
	if err := ghCmd.Run(); err != nil {
		t.Skipf("gh auth token failed (not logged in?): %v", err)
	}
	token := strings.TrimSpace(tokenBuf.String())
	if token == "" {
		t.Skip("gh auth token returned empty token")
	}

	c := &Context{}
	workspace := t.TempDir()

	ctx := recipes.Context_builder{
		Entries: []*recipes.ContextEntry{
			recipes.ContextEntry_builder{
				Path: "auth-checkout",
				From: recipes.ContextFrom_builder{
					GitRepo: osdd.GitRepository_builder{
						FullName:        "opensdd/osdd-api",
						Provider:        "github",
						AuthTokenEnvVar: strPtr("OSDD_TEST_GH_TOKEN"),
					}.Build(),
				}.Build(),
			}.Build(),
		},
	}.Build()

	genCtx := &core.GenerationContext{
		WorkspacePath: workspace,
		EnvOverrides:  map[string]string{"OSDD_TEST_GH_TOKEN": token},
	}
	result, err := c.Materialize(context.Background(), ctx, genCtx)
	require.NoError(t, err, "unexpected error cloning with gh auth token")
	require.Len(t, result.GetEntries(), 1)

	entry := result.GetEntries()[0]
	require.True(t, entry.HasDirectory(), "expected a Directory entry")
	assert.Equal(t, "auth-checkout", entry.GetDirectory())

	clonedPath := filepath.Join(workspace, "auth-checkout")
	info, err := os.Stat(filepath.Join(clonedPath, ".git"))
	require.NoError(t, err, ".git directory should exist in cloned repo")
	assert.True(t, info.IsDir())
}

func TestContext_IntegrationTest_JiraIssuesSource(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	org := testutil.IntegEnv("OSDD_TEST_JIRA_ORG")
	token := testutil.IntegEnv("OSDD_TEST_JIRA_TOKEN")
	project := testutil.IntegEnv("OSDD_TEST_JIRA_PROJECT")
	if org == "" || token == "" || project == "" {
		t.Skip("OSDD_TEST_JIRA_ORG, OSDD_TEST_JIRA_TOKEN, and OSDD_TEST_JIRA_PROJECT required (env var or ~/.config/osdd/.env.integ-test)")
	}

	c := &Context{}
	ctx := recipes.Context_builder{
		Entries: []*recipes.ContextEntry{
			recipes.ContextEntry_builder{
				Path: "jira-issues",
				From: recipes.ContextFrom_builder{
					JiraIssues: recipes.JiraIssuesSource_builder{
						Organization:    org,
						Projects:        []string{project},
						AuthTokenEnvVar: strPtr("OSDD_JIRA_TOKEN"),
					}.Build(),
				}.Build(),
			}.Build(),
		},
	}.Build()

	genCtx := &core.GenerationContext{
		EnvOverrides: map[string]string{"OSDD_JIRA_TOKEN": token},
	}
	result, err := c.Materialize(context.Background(), ctx, genCtx)
	require.NoError(t, err, "unexpected error fetching Jira issues")

	entries := result.GetEntries()
	require.GreaterOrEqual(t, len(entries), 2, "expected at least summary + 1 issue file")

	// First entry is the summary
	summary := entries[0]
	require.True(t, summary.HasFile())
	assert.Equal(t, "jira-issues/all-issues.json", summary.GetFile().GetPath())
	assert.Contains(t, summary.GetFile().GetContent(), `"id"`)
	assert.Contains(t, summary.GetFile().GetContent(), `"title"`)

	// Remaining entries are per-issue files
	for _, e := range entries[1:] {
		require.True(t, e.HasFile())
		assert.True(t, strings.HasPrefix(e.GetFile().GetPath(), "jira-issues/issues/"), "expected per-issue file in jira-issues/issues/ folder")
		assert.True(t, strings.HasSuffix(e.GetFile().GetPath(), ".json"), "expected .json extension")
	}

	if issueID := testutil.IntegEnv("OSDD_TEST_JIRA_ISSUE"); issueID != "" {
		found := false
		for _, e := range entries[1:] {
			if e.GetFile().GetPath() == "jira-issues/issues/"+issueID+".json" {
				found = true
				break
			}
		}
		assert.True(t, found, "expected to find per-issue file for %s", issueID)
	}
}

func TestContext_IntegrationTest_LinearIssuesSource(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	token := testutil.IntegEnv("OSDD_TEST_LINEAR_TOKEN")
	if token == "" {
		t.Skip("OSDD_TEST_LINEAR_TOKEN required (env var or ~/.config/osdd/.env.integ-test)")
	}

	c := &Context{}
	ctx := recipes.Context_builder{
		Entries: []*recipes.ContextEntry{
			recipes.ContextEntry_builder{
				Path: "linear-issues",
				From: recipes.ContextFrom_builder{
					LinearIssues: recipes.LinearIssuesSource_builder{
						Workspace:       "test",
						AuthTokenEnvVar: strPtr("OSDD_LINEAR_TOKEN"),
					}.Build(),
				}.Build(),
			}.Build(),
		},
	}.Build()

	genCtx := &core.GenerationContext{
		EnvOverrides: map[string]string{"OSDD_LINEAR_TOKEN": token},
	}
	result, err := c.Materialize(context.Background(), ctx, genCtx)
	require.NoError(t, err, "unexpected error fetching Linear issues")

	entries := result.GetEntries()
	require.GreaterOrEqual(t, len(entries), 2, "expected at least summary + 1 issue file")

	// First entry is the summary
	summary := entries[0]
	require.True(t, summary.HasFile())
	assert.Equal(t, "linear-issues/all-issues.json", summary.GetFile().GetPath())
	assert.Contains(t, summary.GetFile().GetContent(), `"id"`)
	assert.Contains(t, summary.GetFile().GetContent(), `"title"`)

	// Remaining entries are per-issue files
	for _, e := range entries[1:] {
		require.True(t, e.HasFile())
		assert.True(t, strings.HasPrefix(e.GetFile().GetPath(), "linear-issues/issues/"), "expected per-issue file in linear-issues/issues/ folder")
		assert.True(t, strings.HasSuffix(e.GetFile().GetPath(), ".json"), "expected .json extension")
	}

	if issueID := testutil.IntegEnv("OSDD_TEST_LINEAR_ISSUE"); issueID != "" {
		found := false
		for _, e := range entries[1:] {
			if e.GetFile().GetPath() == "linear-issues/issues/"+issueID+".json" {
				found = true
				break
			}
		}
		assert.True(t, found, "expected to find per-issue file for %s", issueID)
	}
}

// TestContext_Golden_JiraDevplan fetches real Jira issues from the devplan
// organization and compares the materialized output against golden files
// stored in testdata/jira_devplan_golden/.
//
// Run with -update to regenerate the golden files:
//
//	go test ./core/generators/ -run Golden_JiraDevplan -count=1 -update -v
func TestContext_Golden_JiraDevplan(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	org := testutil.IntegEnv("OSDD_TEST_JIRA_ORG")
	if org != "devplan" {
		t.Skip("golden test only runs when OSDD_TEST_JIRA_ORG=devplan")
	}
	token := testutil.IntegEnv("OSDD_TEST_JIRA_TOKEN")
	project := testutil.IntegEnv("OSDD_TEST_JIRA_PROJECT")
	if token == "" || project == "" {
		t.Skip("OSDD_TEST_JIRA_TOKEN and OSDD_TEST_JIRA_PROJECT required")
	}

	// Use a fixed date range to produce a small, stable set of issues (idempotent).
	// Adjust the window so it captures ~5-10 issues from the devplan project.
	from := timestamppb.New(time.Date(2025, 9, 14, 0, 0, 0, 0, time.UTC))
	to := timestamppb.New(time.Date(2025, 9, 16, 0, 0, 0, 0, time.UTC))

	c := &Context{}
	ctx := recipes.Context_builder{
		Entries: []*recipes.ContextEntry{
			recipes.ContextEntry_builder{
				Path: "jira-issues",
				From: recipes.ContextFrom_builder{
					JiraIssues: recipes.JiraIssuesSource_builder{
						Organization:    org,
						Projects:        []string{project},
						AuthTokenEnvVar: strPtr("OSDD_JIRA_TOKEN"),
						Filter: recipes.IssuesFilter_builder{
							CreatedAtFilter: osdd.DatesFilter_builder{
								From: from,
								To:   to,
							}.Build(),
						}.Build(),
					}.Build(),
				}.Build(),
			}.Build(),
		},
	}.Build()

	genCtx := &core.GenerationContext{
		EnvOverrides: map[string]string{"OSDD_JIRA_TOKEN": token},
	}
	result, err := c.Materialize(context.Background(), ctx, genCtx)
	require.NoError(t, err)

	entries := result.GetEntries()
	require.GreaterOrEqual(t, len(entries), 2, "expected summary + at least 1 issue")

	goldenDir := "testdata/jira_devplan_golden"

	if *updateGolden {
		// Wipe and recreate golden directory.
		require.NoError(t, os.RemoveAll(goldenDir))
		require.NoError(t, os.MkdirAll(goldenDir, 0o755))
		for _, e := range entries {
			require.True(t, e.HasFile())
			p := filepath.Join(goldenDir, e.GetFile().GetPath())
			require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
			require.NoError(t, os.WriteFile(p, []byte(e.GetFile().GetContent()), 0o644))
		}
		t.Logf("updated %d golden files in %s", len(entries), goldenDir)
		return
	}

	// Compare each materialized entry against its golden file.
	for _, e := range entries {
		require.True(t, e.HasFile())
		relPath := e.GetFile().GetPath()
		goldenPath := filepath.Join(goldenDir, relPath)

		expected, err := os.ReadFile(goldenPath)
		require.NoError(t, err, "golden file missing for %s (run with -update to generate)", relPath)
		assert.Equal(t, string(expected), e.GetFile().GetContent(), "mismatch for %s", relPath)
	}

	// Verify no stale golden files remain that are no longer produced.
	var goldenFiles []string
	require.NoError(t, filepath.Walk(goldenDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(goldenDir, path)
		goldenFiles = append(goldenFiles, rel)
		return nil
	}))

	produced := make(map[string]bool, len(entries))
	for _, e := range entries {
		produced[e.GetFile().GetPath()] = true
	}
	for _, gf := range goldenFiles {
		assert.True(t, produced[gf], "stale golden file %s no longer produced (run with -update)", gf)
	}
}
