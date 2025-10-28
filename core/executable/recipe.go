package executable

import (
	"context"
	"fmt"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
	"github.com/opensdd/osdd-core/core/providers"
)

func ForRecipe(recipe *recipes.ExecutableRecipe) *Recipe {
	return &Recipe{recipe}
}

type Recipe struct {
	recipe *recipes.ExecutableRecipe
}

func (r *Recipe) Materialize(ctx context.Context) (*osdd.MaterializedResult, error) {
	ideType := r.recipe.GetEntryPoint().GetIdeType()
	ide, err := getIDE(ideType)
	if err != nil {
		return nil, fmt.Errorf("failed to get IDE: %w", err)
	}
	rec := &providers.Recipe{IDE: ide}
	return rec.Materialize(ctx, r.recipe.GetRecipe())
}
