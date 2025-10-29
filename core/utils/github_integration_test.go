//go:build integration

package utils

import (
	"context"
	"strings"
	"testing"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchGithub_Integration_RealGithubFetch(t *testing.T) {
	ref := osdd.GitReference_builder{
		Path: "https://github.com/devplaninc/devplan-cli/blob/main/README.md",
	}.Build()

	content, err := FetchGithub(context.Background(), ref)
	require.NoError(t, err, "unexpected error fetching from GitHub")
	assert.NotEmpty(t, content, "fetched content is empty")
	assert.Contains(t, strings.ToLower(content), "devplan", "fetched content doesn't appear to be the devplan README")
}
