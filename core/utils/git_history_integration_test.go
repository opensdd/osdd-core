package utils

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
	"github.com/opensdd/osdd-core/core/testutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ghTokenOrSkip returns a GitHub token from `gh auth token`, skipping the
// test if gh is not installed or not logged in.
func ghTokenOrSkip(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if _, err := exec.LookPath("gh"); err != nil {
		t.Skip("gh CLI not installed, skipping")
	}
	var buf bytes.Buffer
	cmd := exec.Command("gh", "auth", "token")
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		t.Skipf("gh auth token failed (not logged in?): %v", err)
	}
	token := strings.TrimSpace(buf.String())
	if token == "" {
		t.Skip("gh auth token returned empty token")
	}
	return token
}

func TestFetchGitHistory_Integration(t *testing.T) {
	token := ghTokenOrSkip(t)

	repo := testutil.IntegEnv("OSDD_TEST_GIT_HISTORY_REPO")
	if repo == "" {
		repo = "opensdd/osdd-api" // default to a known public repo
	}

	// Use a narrow date range for deterministic results.
	from := timestamppb.New(time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC))
	to := timestamppb.New(time.Date(2026, 2, 25, 0, 0, 0, 0, time.UTC))

	src := recipes.GitHistorySource_builder{
		Repo: osdd.GitRepository_builder{
			FullName: repo,
			Provider: "github",
		}.Build(),
		DateFilter: osdd.DatesFilter_builder{
			From: from,
			To:   to,
		}.Build(),
	}.Build()

	result, err := FetchGitHistory(context.Background(), src, token)
	require.NoError(t, err)
	require.NotNil(t, result)
	// We may or may not have commits/PRs in the date range, but the function should succeed.
	t.Logf("Result files: %d", len(result.Files))
	for _, f := range result.Files {
		t.Logf("  %s (%d bytes)", f.Name, len(f.Content))
	}
}

func TestFetchGitHubPRs_Integration(t *testing.T) {
	token := ghTokenOrSkip(t)

	from := timestamppb.New(time.Date(2026, 2, 23, 0, 0, 0, 0, time.UTC))
	to := timestamppb.New(time.Date(2026, 2, 26, 0, 0, 0, 0, time.UTC))
	df := osdd.DatesFilter_builder{
		From: from,
		To:   to,
	}.Build()

	prs, err := fetchGitHubPRs(context.Background(), "opensdd", "osdd-core", token, df, false)
	require.NoError(t, err)
	t.Logf("GitHub PRs found: %d", len(prs))
	for _, pr := range prs {
		t.Logf("  PR #%d: %s (%s)", pr.Number, pr.Title, pr.State)
	}
}

func TestFetchBitbucketPRs_Integration(t *testing.T) {
	env := integEnvOrSkip(t,
		"OSDD_TEST_BITBUCKET_WORKSPACE",
		"OSDD_TEST_BITBUCKET_REPO",
		"OSDD_TEST_BITBUCKET_TOKEN",
	)

	from := timestamppb.New(time.Date(2025, 9, 14, 0, 0, 0, 0, time.UTC))
	to := timestamppb.New(time.Date(2025, 9, 16, 0, 0, 0, 0, time.UTC))
	df := osdd.DatesFilter_builder{
		From: from,
		To:   to,
	}.Build()

	prs, err := fetchBitbucketPRs(
		context.Background(),
		env["OSDD_TEST_BITBUCKET_WORKSPACE"],
		env["OSDD_TEST_BITBUCKET_REPO"],
		env["OSDD_TEST_BITBUCKET_TOKEN"],
		df,
		false,
	)
	require.NoError(t, err)
	t.Logf("Bitbucket PRs found: %d", len(prs))
	for _, pr := range prs {
		t.Logf("  PR #%d: %s (%s)", pr.Number, pr.Title, pr.State)
	}
}

func TestFetchGitHistory_Integration_BitbucketProvider(t *testing.T) {
	env := integEnvOrSkip(t,
		"OSDD_TEST_BITBUCKET_WORKSPACE",
		"OSDD_TEST_BITBUCKET_REPO",
		"OSDD_TEST_BITBUCKET_TOKEN",
	)

	fullName := env["OSDD_TEST_BITBUCKET_WORKSPACE"] + "/" + env["OSDD_TEST_BITBUCKET_REPO"]

	from := timestamppb.New(time.Date(2025, 9, 14, 0, 0, 0, 0, time.UTC))
	to := timestamppb.New(time.Date(2025, 9, 16, 0, 0, 0, 0, time.UTC))

	src := recipes.GitHistorySource_builder{
		Repo: osdd.GitRepository_builder{
			FullName: fullName,
			Provider: "bitbucket",
		}.Build(),
		DateFilter: osdd.DatesFilter_builder{
			From: from,
			To:   to,
		}.Build(),
	}.Build()

	result, err := FetchGitHistory(context.Background(), src, env["OSDD_TEST_BITBUCKET_TOKEN"])
	require.NoError(t, err)
	require.NotNil(t, result)
	t.Logf("Result files: %d", len(result.Files))
	for _, f := range result.Files {
		t.Logf("  %s (%d bytes)", f.Name, len(f.Content))
	}
}
