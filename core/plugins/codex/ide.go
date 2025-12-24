package codex

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
	"github.com/opensdd/osdd-core/core"
	"github.com/opensdd/osdd-core/core/plugins/shared"
	"github.com/opensdd/osdd-core/core/providers"
)

//go:embed commands_rules.md
var rules string

var SettingsFolder = ".codex"

func NewIDEProvider() providers.IDE {
	sh := &shared.IDE{
		CommandsFolder: fmt.Sprintf("%v/commands", SettingsFolder),
		Settings:       &settings{},
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

func (p *provider) PrepareStart(_ context.Context, genCtx *core.GenerationContext) (core.ExecProps, error) {
	promptPref, omitDefault := p.getCustomPrompt(genCtx)
	return core.ExecProps{
		PromptPrefix:      promptPref,
		OmitDefaultPrompt: omitDefault,
		ExtraArgs:         p.getExtraArgs(genCtx),
	}, nil
}

func (p *provider) getExtraArgs(genCtx *core.GenerationContext) []string {
	var result []string
	if genCtx.ExecRecipe.GetEntryPoint().GetWorkspace().GetEnabled() {
		result = append(result,
			"--full-auto",
			"--config", "sandbox_mode=\"danger-full-access\"",
		)
	}
	networkAllowed := false
	for _, a := range genCtx.ExecRecipe.GetRecipe().GetIde().GetPermissions().GetAllow() {
		if a.WhichType() == recipes.OperationPermission_Network_case && a.GetNetwork() {
			networkAllowed = true
			break
		}
	}
	for _, d := range genCtx.ExecRecipe.GetRecipe().GetIde().GetPermissions().GetDeny() {
		if d.WhichType() == recipes.OperationPermission_Network_case && d.GetNetwork() {
			networkAllowed = false
			break
		}
	}
	if networkAllowed {
		result = append(result, "--config", "sandbox_workspace_write.network_access='true'")
	}
	for name, srv := range genCtx.ExecRecipe.GetRecipe().GetIde().GetMcp().GetServers() {
		switch srv.WhichType() {
		case recipes.McpServer_Http_case:
			result = append(result, "--config", fmt.Sprintf("mcp_servers.%v.url='%v'", name, srv.GetHttp().GetUrl()))
		case recipes.McpServer_Stdio_case:
			stdio := srv.GetStdio()
			result = append(result, "--config", fmt.Sprintf("mcp_servers.%v.command=\"%v\"", name, stdio.GetCommand()))
			if args := stdio.GetArgs(); len(args) > 0 {
				out, _ := json.Marshal(args)
				if len(out) > 0 {
					result = append(result, "--config", fmt.Sprintf("mcp_servers.%v.args=%v", name, string(out)))
				}

			}
		}
	}
	return result
}

func (p *provider) getCustomPrompt(genCtx *core.GenerationContext) (string, bool) {
	if len(genCtx.ExecRecipe.GetRecipe().GetIde().GetCommands().GetEntries()) == 0 {
		return "", false
	}
	omitDefaultPrompt := false
	promptPref := fmt.Sprintf("Read and remember %v/__commands_rules__.md.", SettingsFolder)
	st := genCtx.ExecRecipe.GetEntryPoint().GetStart()
	switch st.WhichType() {
	case recipes.StartConfig_Command_case:
		promptPref = fmt.Sprintf("%v Then execute command /%v", promptPref, st.GetCommand())
		omitDefaultPrompt = true
	}

	return promptPref, omitDefaultPrompt
}

type settings struct {
	shared.IDESettings
}

func (s *settings) Update(_ context.Context, _ shared.SettingsInput) ([]*osdd.MaterializedResult_Entry, error) {
	return nil, nil
}
