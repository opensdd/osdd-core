package fetcher

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_buildGitHubRecipeURL_SingleName(t *testing.T) {
	t.Parallel()
	url, err := buildGitHubBaseRecipeURL("agents_md")
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/opensdd/recipes/global/agents_md/recipe", url)
}

func Test_buildGitHubRecipeURL_OwnerRepoName(t *testing.T) {
	t.Parallel()
	url, err := buildGitHubBaseRecipeURL("owner/repo/my_recipe")
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/owner/repo/opensdd_recipes/my_recipe/recipe", url)
}

func Test_FetchRecipe_InvalidID(t *testing.T) {
	t.Parallel()
	g := &GitHub{}
	_, err := g.FetchRecipe("")
	assert.Error(t, err)
}
