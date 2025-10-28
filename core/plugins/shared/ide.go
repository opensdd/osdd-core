package shared

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
	"github.com/opensdd/osdd-core/core/utils"
)

// IDE is responsible for materializing Claude Code specific IDE configuration files.
type IDE struct {
	CommandsFolder     string
	MCPServersJSONPath string
	Settings           IDESettings
}

type SettingsInput struct {
	Permissions    *recipes.Permissions
	MCPServerNames []string
	CommandNames   []string
}

type IDESettings interface {
	Update(ctx context.Context, input SettingsInput) ([]*osdd.MaterializedResult_Entry, error)
}

type noOpSettings struct {
	IDESettings
}

func (n *noOpSettings) Update(context.Context, SettingsInput) ([]*osdd.MaterializedResult_Entry, error) {
	return nil, nil
}

// Materialize converts an Ide configuration into a set of materialized files for Claude Code.
// It produces:
// - <CommandsFolder>/<name>.md files for each command
// - <MCPServersJSONPath> for MCP server definitions
// - settings updated/created by IDESettings
func (i *IDE) Materialize(ctx context.Context, ide *recipes.Ide) (*osdd.MaterializedResult, error) {
	if ide == nil {
		return nil, fmt.Errorf("ide cannot be nil")
	}

	var entries []*osdd.MaterializedResult_Entry

	// Commands -> <CommandsFolder>/commands/<name>.md
	if ide.HasCommands() {
		cmdEntries, err := i.materializeCommands(ctx, ide.GetCommands())
		if err != nil {
			return nil, err
		}
		entries = append(entries, cmdEntries...)
	}

	// Extract MCP server names for permissions
	var mcpServerNames []string
	if ide.HasMcp() {
		for name := range ide.GetMcp().GetServers() {
			mcpServerNames = append(mcpServerNames, name)
		}
	}
	// Extract command names for permissions
	var commandNames []string
	if ide.HasCommands() {
		for _, c := range ide.GetCommands().GetEntries() {
			if c != nil && c.GetName() != "" {
				commandNames = append(commandNames, c.GetName())
			}
		}
	}
	ideSett := i.Settings
	if ideSett == nil {
		ideSett = &noOpSettings{}
	}
	settingEntries, err := ideSett.Update(ctx, SettingsInput{
		Permissions:    ide.GetPermissions(),
		MCPServerNames: mcpServerNames,
		CommandNames:   commandNames,
	})
	if err != nil {
		return nil, err
	}
	entries = append(entries, settingEntries...)

	mcpEntries, err := i.materializeMcp(ide.GetMcp())
	if err != nil {
		return nil, err
	}
	entries = append(entries, mcpEntries...)

	return osdd.MaterializedResult_builder{Entries: entries}.Build(), nil
}

func (i *IDE) materializeCommands(ctx context.Context, commands *recipes.Commands) ([]*osdd.MaterializedResult_Entry, error) {
	var entries []*osdd.MaterializedResult_Entry
	if commands == nil {
		return entries, nil
	}
	cmds := commands.GetEntries()
	for _, c := range cmds {
		name := c.GetName()
		if name == "" {
			return nil, fmt.Errorf("command name cannot be empty")
		}
		if !c.HasFrom() {
			return nil, fmt.Errorf("command %s must have a 'from' source", name)
		}

		content, err := i.fetchCommandContent(ctx, c.GetFrom())
		if err != nil {
			return nil, fmt.Errorf("failed to materialize command %s: %w", name, err)
		}

		path := fmt.Sprintf("%v/%s.md", i.CommandsFolder, name)
		entries = append(entries, osdd.MaterializedResult_Entry_builder{
			File: osdd.FullFileContent_builder{Path: path, Content: content}.Build(),
		}.Build())
	}
	return entries, nil
}

func (i *IDE) materializeMcp(mcp *recipes.Mcp) ([]*osdd.MaterializedResult_Entry, error) {
	if mcp == nil || i.MCPServersJSONPath == "" {
		return nil, nil
	}
	var entries []*osdd.MaterializedResult_Entry
	// Read existing file content if it exists
	existingContent := ""
	if data, err := os.ReadFile(i.MCPServersJSONPath); err == nil {
		existingContent = string(data)
	}

	mcpContent, err := buildMcpJSON(mcp, existingContent)
	if err != nil {
		return nil, err
	}
	entries = append(entries, osdd.MaterializedResult_Entry_builder{
		File: osdd.FullFileContent_builder{Path: i.MCPServersJSONPath, Content: mcpContent}.Build(),
	}.Build())
	return entries, nil
}

func (i *IDE) fetchCommandContent(ctx context.Context, from *recipes.CommandFrom) (string, error) {
	if from == nil || !from.HasType() {
		return "", fmt.Errorf("command 'from' source cannot be nil")
	}

	switch from.WhichType() {
	case recipes.CommandFrom_Text_case:
		return from.GetText(), nil
	case recipes.CommandFrom_Cmd_case:
		return utils.ExecuteCommand(ctx, from.GetCmd())
	case recipes.CommandFrom_Github_case:
		return utils.FetchGithub(ctx, from.GetGithub())
	default:
		return "", fmt.Errorf("unknown or unset command source type")
	}
}

type mcpServerConfig struct {
	Type    string            `json:"type,omitempty"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Url     string            `json:"url,omitempty"`
}

type mcpJson struct {
	McpServers map[string]mcpServerConfig `json:"mcpServers"`
}

func buildMcpJSON(mcp *recipes.Mcp, existingContent string) (string, error) {
	if mcp == nil {
		return "", fmt.Errorf("mcp cannot be nil")
	}

	var cm mcpJson

	// Parse existing content if provided
	if existingContent != "" {
		if err := json.Unmarshal([]byte(existingContent), &cm); err != nil {
			// If parsing fails, start fresh
			cm = mcpJson{}
		}
	}

	// Ensure the map is initialized
	if cm.McpServers == nil {
		cm.McpServers = map[string]mcpServerConfig{}
	}

	// Add or update servers from the new configuration
	for name, s := range mcp.GetServers() {
		if s == nil || !s.HasType() {
			continue
		}
		var srv mcpServerConfig
		switch s.WhichType() {
		case recipes.McpServer_Http_case:
			if s.GetHttp() != nil {
				srv.Type = "http"
				srv.Url = s.GetHttp().GetUrl()
			}
		case recipes.McpServer_Stdio_case:
			if s.GetStdio() != nil {
				srv.Type = "stdio"
				cmd := s.GetStdio().GetCommand()
				// Split command into the executable and args by whitespace
				if cmd != "" {
					parts := strings.Fields(cmd)
					if len(parts) > 0 {
						srv.Command = parts[0]
						if len(parts) > 1 {
							srv.Args = parts[1:]
						}
					}
				}
				// Always include an env object for stdio servers
				srv.Env = map[string]string{}
			}
		}
		// If we set at least a type, keep the server
		if srv.Type != "" || srv.Url != "" || srv.Command != "" {
			cm.McpServers[name] = srv
		}
	}

	b, err := json.MarshalIndent(&cm, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal mcp json: %w", err)
	}
	return string(b), nil
}
