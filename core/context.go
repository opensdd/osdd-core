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

	IDEPaths map[string]string

	ExecRecipe *recipes.ExecutableRecipe

	// OutputCMDOnly indicates whether the generator should only output the command to be executed but not execute it.
	OutputCMDOnly bool

	// WorkspacePath is the resolved workspace root directory for materialization.
	WorkspacePath string
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
