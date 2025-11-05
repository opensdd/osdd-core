package shared

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
	"github.com/opensdd/osdd-core/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getIDE() *IDE {
	return &IDE{
		MCPServersJSONPath: ".mcp.json",
		CommandsFolder:     ".claude/commands/",
	}
}

func TestIDE_Materialize_NilIde(t *testing.T) {
	t.Parallel()
	g := getIDE()
	_, err := g.Materialize(context.Background(), &core.GenerationContext{}, nil)
	assert.Error(t, err)
}

func TestIDE_Materialize_Mcp(t *testing.T) {
	t.Parallel()
	g := getIDE()

	ide := recipes.Ide_builder{
		Mcp: recipes.Mcp_builder{Servers: map[string]*recipes.McpServer{
			"github":  recipes.McpServer_builder{Http: recipes.HttpMcpServer_builder{Url: "https://api.githubcopilot.com/mcp/"}.Build()}.Build(),
			"devplan": recipes.McpServer_builder{Stdio: recipes.StdioMcpServer_builder{Command: "devplan mcp"}.Build()}.Build(),
		}}.Build(),
	}.Build()

	res, err := g.Materialize(context.Background(), &core.GenerationContext{}, ide)
	require.NoError(t, err)

	var mcpContent string
	for _, e := range res.GetEntries() {
		if e.GetFile().GetPath() == ".mcp.json" {
			mcpContent = e.GetFile().GetContent()
			break
		}
	}
	require.NotEmpty(t, mcpContent)

	var parsed struct {
		McpServers map[string]struct {
			Type    string            `json:"type"`
			Command string            `json:"command,omitempty"`
			Args    []string          `json:"args,omitempty"`
			Env     map[string]string `json:"env,omitempty"`
			Url     string            `json:"url,omitempty"`
		} `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal([]byte(mcpContent), &parsed))

	require.Contains(t, parsed.McpServers, "github")
	require.Contains(t, parsed.McpServers, "devplan")
	assert.Equal(t, "http", parsed.McpServers["github"].Type)
	assert.Equal(t, "https://api.githubcopilot.com/mcp/", parsed.McpServers["github"].Url)
	assert.Equal(t, "stdio", parsed.McpServers["devplan"].Type)
	assert.Equal(t, "devplan", parsed.McpServers["devplan"].Command)
	assert.Equal(t, []string{"mcp"}, parsed.McpServers["devplan"].Args)
}
