package utils

import (
	"testing"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gitHistorySource(fullName, provider string, maxTokens *int32, dateFilter *osdd.DatesFilter) *recipes.GitHistorySource {
	b := recipes.GitHistorySource_builder{
		Repo: osdd.GitRepository_builder{
			FullName: fullName,
			Provider: provider,
		}.Build(),
		DateFilter:    dateFilter,
		MaxFileTokens: maxTokens,
	}
	return b.Build()
}

func TestFetchGitHistory_NilSource(t *testing.T) {
	t.Parallel()
	_, err := FetchGitHistory(t.Context(), nil, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git history source cannot be nil")
}

func TestFetchGitHistory_NilRepo(t *testing.T) {
	t.Parallel()
	src := recipes.GitHistorySource_builder{}.Build()
	_, err := FetchGitHistory(t.Context(), src, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git history source repo cannot be nil")
}

func TestFetchGitHistory_EmptyFullName(t *testing.T) {
	t.Parallel()
	src := gitHistorySource("", "github", nil, nil)
	_, err := FetchGitHistory(t.Context(), src, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "git history source repo full_name cannot be empty")
}

func TestParseGitLog(t *testing.T) {
	t.Parallel()

	input := `COMMIT_BOUNDARY
commit abc123def456
Author: Alice <alice@example.com>
Date: 2025-06-15T10:00:00+00:00

    Fix the widget

    Longer description of the fix.

diff --git a/widget.go b/widget.go
index 1234567..abcdefg 100644
--- a/widget.go
+++ b/widget.go
@@ -1,3 +1,4 @@
 package widget
+func Fix() {}
COMMIT_BOUNDARY
commit 789abcdef012
Author: Bob <bob@example.com>
Date: 2025-06-14T09:00:00+00:00

    Initial commit
`

	commits := parseGitLog(input)
	require.Len(t, commits, 2)

	assert.Equal(t, "abc123def456", commits[0].Hash)
	assert.Equal(t, "Alice <alice@example.com>", commits[0].Author)
	assert.Equal(t, "2025-06-15T10:00:00+00:00", commits[0].Date)
	assert.Contains(t, commits[0].Message, "Fix the widget")
	assert.Contains(t, commits[0].Message, "Longer description of the fix.")
	assert.Contains(t, commits[0].Diff, "+func Fix() {}")

	assert.Equal(t, "789abcdef012", commits[1].Hash)
	assert.Equal(t, "Bob <bob@example.com>", commits[1].Author)
	assert.Contains(t, commits[1].Message, "Initial commit")
	assert.Empty(t, commits[1].Diff)
}

func TestParseGitLog_Empty(t *testing.T) {
	t.Parallel()
	assert.Empty(t, parseGitLog(""))
	assert.Empty(t, parseGitLog("   \n  \n  "))
}

func TestSplitByTokenLimit(t *testing.T) {
	t.Parallel()

	items := []formattedItem{
		{Label: "a", Content: "short content a\n"},
		{Label: "b", Content: "short content b\n"},
		{Label: "c", Content: "short content c\n"},
	}

	// Large limit: everything fits in one file.
	files := splitByTokenLimit(items, 100000, "test")
	require.Len(t, files, 1)
	assert.Equal(t, "test-001.md", files[0].Name)
	assert.Contains(t, files[0].Content, "short content a")
	assert.Contains(t, files[0].Content, "short content b")
	assert.Contains(t, files[0].Content, "short content c")
}

func TestSplitByTokenLimit_MultipleFiles(t *testing.T) {
	t.Parallel()

	items := []formattedItem{
		{Label: "a", Content: "content a\n"},
		{Label: "b", Content: "content b\n"},
		{Label: "c", Content: "content c\n"},
	}

	// Very small limit: each item gets its own file.
	files := splitByTokenLimit(items, 3, "commits")
	require.GreaterOrEqual(t, len(files), 2)
	assert.Equal(t, "commits-001.md", files[0].Name)
}

func TestSplitByTokenLimit_SingleLargeItem(t *testing.T) {
	t.Parallel()

	items := []formattedItem{
		{Label: "large", Content: "This is a very large item that exceeds any token limit we set because it is very very long and detailed with lots of content.\n"},
	}

	// Item exceeds limit but still gets its own file.
	files := splitByTokenLimit(items, 1, "big")
	require.Len(t, files, 1)
	assert.Equal(t, "big-001.md", files[0].Name)
	assert.Contains(t, files[0].Content, "very large item")
}

func TestSplitByTokenLimit_Empty(t *testing.T) {
	t.Parallel()
	files := splitByTokenLimit(nil, 1000, "empty")
	assert.Empty(t, files)
}

func TestFormatCommit(t *testing.T) {
	t.Parallel()

	commits := []parsedCommit{
		{
			Hash:    "abc123",
			Author:  "Alice <alice@example.com>",
			Date:    "2025-06-15T10:00:00+00:00",
			Message: "Fix the widget",
			Diff:    "+func Fix() {}",
		},
	}

	items := formatCommits(commits)
	require.Len(t, items, 1)
	assert.Contains(t, items[0].Content, "## Commit abc123")
	assert.Contains(t, items[0].Content, "**Author:** Alice <alice@example.com>")
	assert.Contains(t, items[0].Content, "**Date:** 2025-06-15T10:00:00+00:00")
	assert.Contains(t, items[0].Content, "### Message")
	assert.Contains(t, items[0].Content, "Fix the widget")
	assert.Contains(t, items[0].Content, "### Diff")
	assert.Contains(t, items[0].Content, "+func Fix() {}")
}

func TestFormatOnePR(t *testing.T) {
	t.Parallel()

	pr := pullRequest{
		Number: 42,
		Title:  "Add feature X",
		Author: "alice",
		State:  "closed",
		Body:   "This adds feature X.",
		Diff:   "+feature code",
		Reviews: []prReview{
			{Author: "bob", State: "APPROVED", Body: "Looks good!"},
		},
	}

	content := formatOnePR(pr)
	assert.Contains(t, content, "## PR #42: Add feature X")
	assert.Contains(t, content, "**Author:** alice")
	assert.Contains(t, content, "**State:** closed")
	assert.Contains(t, content, "### Description")
	assert.Contains(t, content, "This adds feature X.")
	assert.Contains(t, content, "### Reviews")
	assert.Contains(t, content, "**bob** (APPROVED): Looks good!")
	assert.Contains(t, content, "### Diff")
	assert.Contains(t, content, "+feature code")
}

func TestCountTokens(t *testing.T) {
	t.Parallel()

	// Basic sanity check: a short string should have a reasonable token count.
	tokens := countTokens("Hello, world!")
	assert.Greater(t, tokens, 0)
	assert.Less(t, tokens, 20)

	// Empty string should have zero tokens.
	assert.Equal(t, 0, countTokens(""))
}
