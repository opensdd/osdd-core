package core

import "github.com/opensdd/osdd-api/clients/go/osdd"

type GenerationContext struct {
	Prefetched map[string]*osdd.FetchedData
	// UserInput carries user-provided values keyed by parameter name.
	UserInput map[string]string
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
