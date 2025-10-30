package fetcher

import (
	"context"
	"fmt"
	"strings"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
	"github.com/opensdd/osdd-core/core/utils"
	"google.golang.org/protobuf/encoding/protojson"
)

type GitHub struct {
}

// buildGitHubRecipeURL constructs a GitHub URL for the given recipe id according to the rules:
// 1) <recipe_name> -> https://github.com/opensdd/recipes/global/<recipe_name>/recipe.json
// 2) <repo_owner>/<repo_name>/<recipe_name> -> https://github.com/<repo_owner>/<repo_name>/opensdd_recipes/<recipe_name>/recipe.json
func buildGitHubRecipeURL(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("recipe id cannot be empty")
	}
	parts := strings.Split(id, "/")
	switch len(parts) {
	case 1:
		name := parts[0]
		if name == "" {
			return "", fmt.Errorf("recipe name cannot be empty")
		}
		return fmt.Sprintf("https://github.com/opensdd/recipes/global/%s/recipe.json", name), nil
	case 3:
		owner, repo, name := parts[0], parts[1], parts[2]
		if owner == "" || repo == "" || name == "" {
			return "", fmt.Errorf("invalid recipe id: %s", id)
		}
		return fmt.Sprintf("https://github.com/%s/%s/opensdd_recipes/%s/recipe.json", owner, repo, name), nil
	default:
		return "", fmt.Errorf("invalid recipe id format: %s", id)
	}
}

// FetchRecipe fetches a recipe by its id. ID can be of 2 acceptable formats:
// 1. <recipe_name>. In that case a recipe is fetched from repo https://github.com/opensdd/recipes, file path within a repo - "global/<recipe_name>/recipe.json"
// 2. <repo_owner>/<repo_name>/<recipe_name>. In that case a recipe is fetched from repo https://github.com/<repo_owner>/<repo_name>, file path within a repo - "opensdd_recipes/<recipe_name>/recipe.json"
//
// Returns an error if a recipe cannot be fetched, id is of not correct format or any other issue fetching repo.
func (g *GitHub) FetchRecipe(id string) (*recipes.ExecutableRecipe, error) {
	url, err := buildGitHubRecipeURL(id)
	if err != nil {
		return nil, err
	}

	ref := osdd.GitReference_builder{Path: url}.Build()
	content, err := utils.FetchGithub(context.Background(), ref)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch recipe from GitHub: %w", err)
	}

	exec := &recipes.ExecutableRecipe{}
	um := protojson.UnmarshalOptions{DiscardUnknown: true}
	return exec, um.Unmarshal([]byte(content), exec)
}
