package junie

import (
	"context"
	"fmt"

	_ "embed"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
	"github.com/opensdd/osdd-core/core"
	"github.com/opensdd/osdd-core/core/plugins/shared"
	"github.com/opensdd/osdd-core/core/providers"
)

//go:embed commands_rules.md
var rules string

var SettingsFolder = ".junie"

func NewIDEProvider() providers.IDE {
	sh := &shared.IDE{
		MCPServersJSONPath: ".junie/mcp/mcp.json",
		CommandsFolder:     fmt.Sprintf("%v/commands", SettingsFolder),
		Settings:           &settings{},
	}
	return &provider{shared: sh}
}

type provider struct {
	shared *shared.IDE
}

func (p *provider) Materialize(ctx context.Context, genCtx *core.GenerationContext, ide *recipes.Ide) (*osdd.MaterializedResult, error) {
	result, err := p.shared.Materialize(ctx, genCtx, ide)
	if err != nil {
		return nil, err
	}
	if len(ide.GetCommands().GetEntries()) > 0 {
		result.SetEntries(append(result.GetEntries(), osdd.MaterializedResult_Entry_builder{
			File: osdd.FullFileContent_builder{Path: fmt.Sprintf("%v/__commands_rules__.md", SettingsFolder), Content: rules}.Build(),
		}.Build()))
	}
	return result, nil
}

func (p *provider) PrepareStart(_ context.Context, _ *core.GenerationContext) (core.ExecProps, error) {
	return core.ExecProps{
		PromptPrefix:      "",
		OmitDefaultPrompt: true,
	}, nil
}

type settings struct {
	shared.IDESettings
}

func (s *settings) Update(_ context.Context, _ shared.SettingsInput) ([]*osdd.MaterializedResult_Entry, error) {
	return nil, nil
}
