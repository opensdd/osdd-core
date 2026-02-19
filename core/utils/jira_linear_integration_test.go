package utils

import (
	"context"
	"testing"

	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
	"github.com/opensdd/osdd-core/core/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func integEnvOrSkip(t *testing.T, keys ...string) map[string]string {
	t.Helper()
	if testing.Short() {
		t.Skip()
	}
	vals := make(map[string]string, len(keys))
	for _, k := range keys {
		v := testutil.IntegEnv(k)
		if v == "" {
			t.Skipf("%s required (env var or ~/.config/osdd/.env.integ-test)", k)
		}
		vals[k] = v
	}
	return vals
}

func TestFetchJiraIssues_Integration(t *testing.T) {
	env := integEnvOrSkip(t, "OSDD_TEST_JIRA_ORG", "OSDD_TEST_JIRA_TOKEN", "OSDD_TEST_JIRA_PROJECT")

	src := recipes.JiraIssuesSource_builder{
		Organization: env["OSDD_TEST_JIRA_ORG"],
		Projects:     []string{env["OSDD_TEST_JIRA_PROJECT"]},
	}.Build()

	result, err := FetchJiraIssues(context.Background(), src, env["OSDD_TEST_JIRA_TOKEN"])
	require.NoError(t, err)
	assert.NotEmpty(t, result.Summary)
	assert.NotEmpty(t, result.Issues)
	if issueID := testutil.IntegEnv("OSDD_TEST_JIRA_ISSUE"); issueID != "" {
		assert.Contains(t, result.Issues, issueID)
	}
}

func TestFetchLinearIssues_Integration(t *testing.T) {
	env := integEnvOrSkip(t, "OSDD_TEST_LINEAR_TOKEN")

	src := recipes.LinearIssuesSource_builder{}.Build()

	result, err := FetchLinearIssues(context.Background(), src, env["OSDD_TEST_LINEAR_TOKEN"])
	require.NoError(t, err)
	assert.NotEmpty(t, result.Summary)
	assert.NotEmpty(t, result.Issues)
	if issueID := testutil.IntegEnv("OSDD_TEST_LINEAR_ISSUE"); issueID != "" {
		assert.Contains(t, result.Issues, issueID)
	}
}

func TestFetchLinearIssues_Integration_WithTeams(t *testing.T) {
	env := integEnvOrSkip(t, "OSDD_TEST_LINEAR_TOKEN", "OSDD_TEST_LINEAR_TEAM")

	src := recipes.LinearIssuesSource_builder{
		Teams: []string{env["OSDD_TEST_LINEAR_TEAM"]},
	}.Build()

	result, err := FetchLinearIssues(context.Background(), src, env["OSDD_TEST_LINEAR_TOKEN"])
	require.NoError(t, err)
	require.NotNil(t, result)
}
