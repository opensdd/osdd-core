package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
	"github.com/opensdd/osdd-core/core/utils"
	"google.golang.org/protobuf/encoding/protojson"
	"gopkg.in/yaml.v3"
)

type GitHub struct {
	Strict bool
}

// fetchByURL fetches content from a GitHub URL using utils.FetchGithub.
// It accepts standard github.com paths and lets utils.ConvertToRawURL resolve raw URLs.
func fetchByURL(ctx context.Context, url string) (string, error) {
	ref := osdd.GitReference_builder{Path: url}.Build()
	return utils.FetchGithub(ctx, ref)
}

// buildGitHubBaseRecipeURL constructs a GitHub URL base (without extension) for the given recipe id according to the rules:
// 1) <recipe_name> -> https://github.com/opensdd/recipes/global/<recipe_name>/recipe
// 2) <repo_owner>/<repo_name>/<recipe_name> -> https://github.com/<repo_owner>/<repo_name>/opensdd_recipes/<recipe_name>/recipe
func buildGitHubBaseRecipeURL(id string) (string, error) {
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
		return fmt.Sprintf("https://github.com/opensdd/recipes/global/%s/recipe", name), nil
	case 3:
		owner, repo, name := parts[0], parts[1], parts[2]
		if owner == "" || repo == "" || name == "" {
			return "", fmt.Errorf("invalid recipe id: %s", id)
		}
		return fmt.Sprintf("https://github.com/%s/%s/opensdd_recipes/%s/recipe", owner, repo, name), nil
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
	baseURL, err := buildGitHubBaseRecipeURL(id)
	if err != nil {
		return nil, err
	}

	// Build candidate URLs, YAML first, then JSON
	candidates := []struct {
		url  string
		yaml bool
	}{
		{url: baseURL + ".yaml", yaml: true},
		{url: baseURL + ".json", yaml: false},
	}

	var (
		content  string
		usedYAML bool
		lastErr  error
	)

	ctx := context.Background()
	for _, c := range candidates {
		if s, err := fetchByURL(ctx, c.url); err == nil && s != "" {
			content = s
			usedYAML = c.yaml
			break
		} else if err != nil {
			lastErr = err
		}
	}

	if content == "" {
		if lastErr != nil {
			return nil, fmt.Errorf("failed to fetch recipe from GitHub: %w", lastErr)
		}
		return nil, fmt.Errorf("failed to fetch recipe from GitHub: unknown error")
	}

	exec := &recipes.ExecutableRecipe{}
	um := protojson.UnmarshalOptions{DiscardUnknown: !g.Strict}

	if usedYAML {
		// Convert YAML to JSON first, then unmarshal as protobuf JSON
		var y any
		if err := yaml.Unmarshal([]byte(content), &y); err != nil {
			return nil, fmt.Errorf("failed to parse YAML recipe: %w", err)
		}
		b, err := json.Marshal(y)
		if err != nil {
			return nil, fmt.Errorf("failed to convert YAML to JSON: %w", err)
		}
		return exec, um.Unmarshal(b, exec)
	}

	// JSON content, unmarshal directly
	return exec, um.Unmarshal([]byte(content), exec)
}
