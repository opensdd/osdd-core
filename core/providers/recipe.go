package providers

import (
	"context"
	"fmt"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
	"github.com/opensdd/osdd-core/core"
	"github.com/opensdd/osdd-core/core/generators"
	"github.com/opensdd/osdd-core/core/prefetch"
)

type Recipe struct {
	IDE IDE
}

func (r *Recipe) Materialize(ctx context.Context, recipe *recipes.Recipe) (*osdd.MaterializedResult, error) {
	if recipe == nil {
		return nil, fmt.Errorf("recipe cannot be nil")
	}
	genCtx := &core.GenerationContext{}
	if pf := recipe.GetPrefetch(); pf != nil {
		p := prefetch.Processor{}
		entries, err := p.Process(ctx, pf)
		if err != nil {
			return nil, fmt.Errorf("failed to process prefetch: %w", err)
		}
		genCtx.Prefetched = entries
	}

	var resultEntries []*osdd.MaterializedResult_Entry

	// Materialize context entries if present
	if recipe.HasContext() {
		contextGen := &generators.Context{}
		contextResult, err := contextGen.Materialize(ctx, recipe.GetContext(), genCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to materialize context: %w", err)
		}
		resultEntries = append(resultEntries, contextResult.GetEntries()...)
	}

	// Materialize IDE configuration if present
	if recipe.HasIde() {
		ideResult, err := r.IDE.Materialize(ctx, recipe.GetIde())
		if err != nil {
			return nil, fmt.Errorf("failed to materialize IDE configuration: %w", err)
		}
		resultEntries = append(resultEntries, ideResult.GetEntries()...)
	}

	return osdd.MaterializedResult_builder{
		Entries: resultEntries,
	}.Build(), nil
}
