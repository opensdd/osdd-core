package fetcher

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitHub_FetchRecipe_Integration_AgentsMD(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	g := &GitHub{Strict: true}
	exec, err := g.FetchRecipe("docs_update")
	require.NoError(t, err, "unexpected error fetching agents_md recipe from GitHub")
	require.NotNil(t, exec)
	require.NotNil(t, exec.GetRecipe())
	rec := exec.GetRecipe()
	assert.NotNil(t, rec.GetIde())
	assert.NotNil(t, rec.GetContext())
	assert.Nil(t, rec.GetPrefetch())
	assert.Equal(t, "docs_update_run", exec.GetEntryPoint().GetStart().GetCommand())
}
