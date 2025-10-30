package prefetch

import (
	"context"
	"fmt"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/opensdd/osdd-core/core/utils"
	"google.golang.org/protobuf/encoding/protojson"
)

type Processor struct{}

func (p *Processor) Process(ctx context.Context, prefetch *osdd.Prefetch) (map[string]*osdd.FetchedData, error) {
	entries := prefetch.GetEntries()
	if len(entries) == 0 {
		return nil, nil
	}

	result := make(map[string]*osdd.FetchedData)

	for i, entry := range entries {
		if entry == nil {
			return nil, fmt.Errorf("prefetch entry at index %d is nil", i)
		}

		// Process the entry based on its type
		data, err := p.processEntry(ctx, entry)
		if err != nil {
			return nil, fmt.Errorf("failed to process entry at index %d: %w", i, err)
		}
		res := &osdd.PrefetchResult{}
		u := protojson.UnmarshalOptions{DiscardUnknown: true}
		if err := u.Unmarshal([]byte(data), res); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prefetch result: %w", err)
		}
		for _, d := range res.GetData() {
			result[d.GetId()] = d
		}
	}

	return result, nil
}

func (p *Processor) processEntry(ctx context.Context, entry *osdd.PrefetchEntry) (string, error) {
	switch entry.WhichType() {
	case osdd.PrefetchEntry_Cmd_case:
		data, err := utils.ExecuteCommand(ctx, entry.GetCmd())
		if err != nil {
			return "", fmt.Errorf("command execution failed: %w", err)
		}
		return data, nil

	default:
		return "", fmt.Errorf("unknown or unset prefetch entry type [%v]", entry.WhichType())
	}
}
