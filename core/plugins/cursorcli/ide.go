package cursorcli

import (
	"context"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/opensdd/osdd-core/core/plugins/shared"
	"github.com/opensdd/osdd-core/core/providers"
)

func NewIDEProvider() providers.IDEProvider {
	return &shared.IDE{
		CommandsFolder:     ".cursor/commands",
		MCPServersJSONPath: ".cursor/mcp.json",
		Settings:           &settings{},
	}
}

type settings struct {
	shared.IDESettings
}

func (s *settings) Update(_ context.Context, _ shared.SettingsInput) ([]*osdd.MaterializedResult_Entry, error) {
	return nil, nil
}
