package core

import (
	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
)

type GenerationContext struct {
	Prefetched map[string]*osdd.FetchedData
	// UserInput carries user-provided values keyed by a parameter name.
	UserInput map[string]string
	// IDE for which the recipe is being generated.
	IDE string

	ExecRecipe *recipes.ExecutableRecipe
}

func (g *GenerationContext) GetPrefetched() map[string]*osdd.FetchedData {
	if g == nil {
		return nil
	}
	return g.Prefetched
}

func (g *GenerationContext) GetUserInput() map[string]string {
	if g == nil {
		return nil
	}
	return g.UserInput
}
