package executable

import (
	"context"
	"fmt"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
	"github.com/opensdd/osdd-core/core"
	"github.com/opensdd/osdd-core/core/providers"
)

type RecipeExecutionResult struct {
	// LaunchResult is the result of launching the IDE.
	LaunchResult LaunchResult
}

func ForRecipe(recipe *recipes.ExecutableRecipe) *Recipe {
	return &Recipe{recipe: recipe}
}

type Recipe struct {
	recipe       *recipes.ExecutableRecipe
	materialized *osdd.MaterializedResult
	ide          providers.IDE
}

func (r *Recipe) Materialize(ctx context.Context, genCtx *core.GenerationContext) (*osdd.MaterializedResult, error) {
	if r.materialized != nil {
		return r.materialized, nil
	}
	ideType := r.recipe.GetEntryPoint().GetIdeType()
	ide, err := getIDE(ideType)
	if err != nil {
		return nil, fmt.Errorf("failed to get IDE: %w", err)
	}
	r.ide = ide
	rec := &providers.Recipe{IDE: ide}
	ws := &providers.Workspace{}
	wsPath, err := ws.Materialize(ctx, r.recipe.GetEntryPoint().GetWorkspace())
	if err != nil {
		return nil, fmt.Errorf("failed to materialize workspace: %w", err)
	}
	recipeResult, err := rec.Materialize(ctx, genCtx, r.recipe.GetRecipe())
	if err != nil {
		return nil, fmt.Errorf("failed to materialize recipe: %w", err)
	}
	if wsPath != "" {
		recipeResult.SetWorkspacePath(wsPath)
	}

	// Cache materialized result for Execute()
	r.materialized = recipeResult
	return recipeResult, nil
}

// Execute start a recipe in the specified AI-IDE with a specified start command.
func (r *Recipe) Execute(ctx context.Context, genCtx *core.GenerationContext) (RecipeExecutionResult, error) {
	if r.materialized == nil {
		return RecipeExecutionResult{}, fmt.Errorf("recipe must be materialized first")
	}
	root := r.materialized.GetWorkspacePath()
	if root == "" {
		root = "."
	}
	// Persist materialized files into the workspace so the IDE can use them.
	if err := core.PersistMaterializedResult(context.Background(), root, r.materialized); err != nil {
		return RecipeExecutionResult{}, fmt.Errorf("failed to persist materialized result: %w", err)
	}
	ideType := r.recipe.GetEntryPoint().GetIdeType()
	execProps, err := r.ide.PrepareStart(ctx, genCtx)
	if err != nil {
		return RecipeExecutionResult{}, fmt.Errorf("failed to prepare start: %w", err)
	}
	prompt := execProps.PromptPrefix
	if !execProps.OmitDefaultPrompt {
		prompt += getPrompt(r.recipe.GetEntryPoint().GetStart())
	}
	args := execProps.ExtraArgs
	if prompt != "" {
		args = append(args, prompt)
	}

	launchResult, err := LaunchIDE(ctx, LaunchParams{
		IDE:           ideType,
		RepoPath:      root,
		Args:          args,
		OutputCMDOnly: genCtx.OutputCMDOnly,
	})
	if err != nil {
		return RecipeExecutionResult{}, fmt.Errorf("failed to launch IDE: %w", err)
	}
	return RecipeExecutionResult{LaunchResult: launchResult}, nil
}

func getPrompt(st *recipes.StartConfig) string {
	if st == nil {
		return ""
	}
	switch st.WhichType() {
	case recipes.StartConfig_Command_case:
		return fmt.Sprintf("/%v", st.GetCommand())
	case recipes.StartConfig_Prompt_case:
		return st.GetPrompt()
	}
	return ""
}
