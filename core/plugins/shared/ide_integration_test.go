package shared

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
	"github.com/opensdd/osdd-core/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getIDEInteg() *IDE {
	return &IDE{
		MCPServersJSONPath: ".mcp.json",
		CommandsFolder:     ".claude/commands/",
	}
}

func TestIDE_Materialize_Mcp_MergeWithExisting(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	// Setup: Materialize a temporary directory and existing MCP file
	tempDir := t.TempDir()
	claudeDir := filepath.Join(tempDir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0755))

	// Change to temp directory for the test
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tempDir))
	defer func() { _ = os.Chdir(origDir) }()

	// Materialize existing MCP file
	existingMcp := `{
  "mcpServers": {
    "filesystem": {
      "command": "npx @modelcontextprotocol/server-filesystem"
    },
    "github": {
      "url": "https://old-api.github.com/mcp/"
    }
  }
}`
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, ".mcp.json"), []byte(existingMcp), 0644))

	// Define new MCP servers to merge
	g := getIDEInteg()
	ide := recipes.Ide_builder{
		Mcp: recipes.Mcp_builder{Servers: map[string]*recipes.McpServer{
			"github":  recipes.McpServer_builder{Http: recipes.HttpMcpServer_builder{Url: "https://api.githubcopilot.com/mcp/"}.Build()}.Build(), // Update existing
			"devplan": recipes.McpServer_builder{Stdio: recipes.StdioMcpServer_builder{Command: "devplan mcp"}.Build()}.Build(),                  // Add new
		}}.Build(),
	}.Build()

	// Execute
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

	// Verify existing server not in new config is preserved
	require.Contains(t, parsed.McpServers, "filesystem", "existing server not in new config should be preserved")
	assert.Equal(t, "npx @modelcontextprotocol/server-filesystem", parsed.McpServers["filesystem"].Command)

	// Verify existing server in new config is updated
	require.Contains(t, parsed.McpServers, "github", "existing server should be updated")
	assert.Equal(t, "http", parsed.McpServers["github"].Type)
	assert.Equal(t, "https://api.githubcopilot.com/mcp/", parsed.McpServers["github"].Url, "github server should be updated")

	// Verify new server is added
	require.Contains(t, parsed.McpServers, "devplan", "new server should be added")
	assert.Equal(t, "stdio", parsed.McpServers["devplan"].Type)
	assert.Equal(t, "devplan", parsed.McpServers["devplan"].Command)
	assert.Equal(t, []string{"mcp"}, parsed.McpServers["devplan"].Args)

	// Verify total count
	assert.Len(t, parsed.McpServers, 3, "should have 3 servers total")
}

func TestIDE_Materialize_Mcp_InvalidExistingJSON(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	// Setup: Materialize a temporary directory with invalid JSON
	tempDir := t.TempDir()
	claudeDir := filepath.Join(tempDir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0755))

	// Change to temp directory for the test
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tempDir))
	defer func() { _ = os.Chdir(origDir) }()

	// Materialize existing MCP file with invalid JSON
	invalidJSON := `{ "mcpServers": { "test": }`
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, ".mcp.json"), []byte(invalidJSON), 0644))

	// Define new MCP servers
	g := getIDEInteg()
	ide := recipes.Ide_builder{
		Mcp: recipes.Mcp_builder{Servers: map[string]*recipes.McpServer{
			"devplan": recipes.McpServer_builder{Stdio: recipes.StdioMcpServer_builder{Command: "devplan mcp"}.Build()}.Build(),
		}}.Build(),
	}.Build()

	// Execute - should not error, just start fresh
	res, err := g.Materialize(context.Background(), &core.GenerationContext{}, ide)
	require.Error(t, err)
	assert.Nil(t, res)
}
