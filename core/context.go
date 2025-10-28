package core

import "github.com/opensdd/osdd-api/clients/go/osdd"

type GenerationContext struct {
	Prefetched map[string]*osdd.FetchedData
}

func (g *GenerationContext) GetPrefetched() map[string]*osdd.FetchedData {
	if g == nil {
		return nil
	}
	return g.Prefetched
}
