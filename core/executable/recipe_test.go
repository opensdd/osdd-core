package executable

import (
	"context"
	"testing"

	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutableRecipe_Materialize_Table(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		ideType     string
		execRecipe  recipes.ExecutableRecipe_builder
		wantErrSub  string
		wantEntries int
	}{
		{
			name:    "happy path: claude ide type with empty recipe produces no entries",
			ideType: "claude",
			execRecipe: recipes.ExecutableRecipe_builder{
				// Provide an empty recipe so that recipes.Recipe doesn't try to use IDE
				Recipe: recipes.Recipe_builder{}.Build(),
			},
			wantEntries: 0,
		},
		{
			name:    "error: unsupported ide type",
			ideType: "unknown-ide",
			execRecipe: recipes.ExecutableRecipe_builder{
				Recipe: recipes.Recipe_builder{}.Build(),
			},
			wantErrSub: "failed to get IDE",
		},
		{
			name:    "error: underlying recipes.Materialize rejects nil Recipe",
			ideType: "claude",
			// leave Recipe unset so GetRecipe() returns nil
			execRecipe: recipes.ExecutableRecipe_builder{},
			wantErrSub: "recipe cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Build executable recipe with given entry point ide type and provided recipe
			exec := tt.execRecipe
			exec.EntryPoint = recipes.EntryPoint_builder{IdeType: tt.ideType}.Build()
			re := ForRecipe(exec.Build())

			res, err := re.Materialize(context.Background())
			if tt.wantErrSub != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrSub)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, res)
			if tt.wantEntries >= 0 {
				assert.Len(t, res.GetEntries(), tt.wantEntries)
			}
		})
	}
}
