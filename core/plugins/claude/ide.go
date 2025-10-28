package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
	"github.com/opensdd/osdd-core/core/plugins/shared"
	"github.com/opensdd/osdd-core/core/providers"
)

func NewIDEProvider() providers.IDE {
	return &shared.IDE{
		CommandsFolder:     ".claude/commands",
		MCPServersJSONPath: ".mcp.json",
		Settings:           &settings{},
	}
}

type settings struct {
	shared.IDESettings
}

func (s *settings) Update(_ context.Context, input shared.SettingsInput) ([]*osdd.MaterializedResult_Entry, error) {
	return materializePermissions(input.Permissions, input.MCPServerNames, input.CommandNames)
}

func materializePermissions(perms *recipes.Permissions, mcpServerNames []string, commandNames []string) ([]*osdd.MaterializedResult_Entry, error) {
	var entries []*osdd.MaterializedResult_Entry

	// Read existing file content if it exists
	existingContent := ""
	settingsPath := ".claude/settings.local.json"
	if data, err := os.ReadFile(settingsPath); err == nil {
		existingContent = string(data)
	}

	settingsContent, err := buildClaudeSettingsJSON(perms, mcpServerNames, commandNames, existingContent)
	if err != nil {
		return nil, err
	}
	entries = append(entries, osdd.MaterializedResult_Entry_builder{
		File: osdd.FullFileContent_builder{Path: settingsPath, Content: settingsContent}.Build(),
	}.Build())
	return entries, nil
}

// JSON models for Claude configuration files

type claudeSettings struct {
	Permissions struct {
		Allow       []string `json:"allow,omitempty"`
		Deny        []string `json:"deny,omitempty"`
		Ask         []string `json:"ask,omitempty"`
		DefaultMode string   `json:"defaultMode,omitempty"`
	} `json:"permissions"`
	EnabledMcpjsonServers      []string `json:"enabledMcpjsonServers,omitempty"`
	EnableAllProjectMcpServers bool     `json:"enableAllProjectMcpServers,omitempty"`
}

func buildClaudeSettingsJSON(perms *recipes.Permissions, mcpServerNames []string, commandNames []string, existingContent string) (string, error) {
	var s claudeSettings

	// Parse existing content if provided
	if existingContent != "" {
		if err := json.Unmarshal([]byte(existingContent), &s); err != nil {
			// If parsing fails, start fresh but log the error
			s = claudeSettings{}
		}
	}

	// Ensure non-nil slices
	if s.Permissions.Allow == nil {
		s.Permissions.Allow = []string{}
	}
	if s.Permissions.Deny == nil {
		s.Permissions.Deny = []string{}
	}
	if s.Permissions.Ask == nil {
		s.Permissions.Ask = []string{}
	}
	s.Permissions.DefaultMode = "acceptEdits"
	if s.EnabledMcpjsonServers == nil {
		s.EnabledMcpjsonServers = []string{}
	}
	s.EnableAllProjectMcpServers = true

	// Build new permissions from input
	newAllow := make([]string, 0)
	if perms != nil {
		for _, p := range perms.GetAllow() {
			if !p.HasType() {
				continue
			}
			newAllow = append(newAllow, formatPermission(p))
		}
	}

	newDeny := make([]string, 0)
	if perms != nil {
		for _, p := range perms.GetDeny() {
			if !p.HasType() {
				continue
			}
			newDeny = append(newDeny, formatPermission(p))
		}
	}

	// Add MCP servers to allow list as mcp__<name>
	var mcpAllowPermissions []string
	for _, serverName := range mcpServerNames {
		mcpAllowPermissions = append(mcpAllowPermissions, fmt.Sprintf("mcp__%s", serverName))
	}
	newAllow = append(newAllow, mcpAllowPermissions...)

	// Add SlashCommand permissions for each command
	var cmdAllow []string
	for _, name := range commandNames {
		if name != "" {
			cmdAllow = append(cmdAllow, fmt.Sprintf("SlashCommand(/%s)", name))
		}
	}
	newAllow = append(newAllow, cmdAllow...)

	// Merge with existing permissions (deduplicate)
	s.Permissions.Allow = mergeUniqueStrings(s.Permissions.Allow, newAllow)
	s.Permissions.Deny = mergeUniqueStrings(s.Permissions.Deny, newDeny)

	// Add MCP server names to enabledMcpjsonServers
	s.EnabledMcpjsonServers = mergeUniqueStrings(s.EnabledMcpjsonServers, mcpServerNames)

	b, err := json.MarshalIndent(&s, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal settings json: %w", err)
	}
	return string(b), nil
}

// mergeUniqueStrings merges two string slices, removing duplicates
func mergeUniqueStrings(existing, new []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0)

	// Add existing items first
	for _, s := range existing {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}

	// Add new items that aren't duplicates
	for _, s := range new {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}

	return result
}

func formatPermission(p *recipes.OperationPermission) string {
	switch p.WhichType() {
	case recipes.OperationPermission_Bash_case:
		return fmt.Sprintf("Bash(%s)", p.GetBash())
	case recipes.OperationPermission_Read_case:
		return fmt.Sprintf("Read(%s)", p.GetRead())
	case recipes.OperationPermission_Write_case:
		return fmt.Sprintf("Write(%s)", p.GetWrite())
	default:
		return ""
	}
}
