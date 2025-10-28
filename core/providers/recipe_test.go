package providers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
	"github.com/opensdd/osdd-core/core/plugins/shared"
	"github.com/opensdd/osdd-core/core/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getIDE() providers.IDE {
	return &shared.IDE{
		CommandsFolder:     ".claude/commands",
		MCPServersJSONPath: ".mcp.json",
	}
}

func strPtr(s string) *string {
	return &s
}

func TestRecipe_Materialize_NilRecipe(t *testing.T) {
	r := &providers.Recipe{IDE: getIDE()}
	_, err := r.Materialize(context.Background(), nil)
	assert.Error(t, err, "expected error for nil recipe")
}

func TestRecipe_Materialize_EmptyRecipe(t *testing.T) {
	r := &providers.Recipe{IDE: getIDE()}
	recipe := recipes.Recipe_builder{}.Build()
	result, err := r.Materialize(context.Background(), recipe)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result.GetEntries())
}

func TestRecipe_Materialize_ContextOnly(t *testing.T) {
	r := &providers.Recipe{IDE: getIDE()}

	recipe := recipes.Recipe_builder{
		Context: recipes.Context_builder{
			Entries: []*recipes.ContextEntry{
				recipes.ContextEntry_builder{
					Path: "README.md",
					From: recipes.ContextFrom_builder{
						Text: strPtr("# Project README"),
					}.Build(),
				}.Build(),
			},
		}.Build(),
	}.Build()

	result, err := r.Materialize(context.Background(), recipe)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.GetEntries(), 1)

	entry := result.GetEntries()[0]
	assert.Equal(t, "README.md", entry.GetFile().GetPath())
	assert.Equal(t, "# Project README", entry.GetFile().GetContent())
}

func TestRecipe_Materialize_IdeOnly(t *testing.T) {
	r := &providers.Recipe{IDE: getIDE()}

	recipe := recipes.Recipe_builder{
		Ide: recipes.Ide_builder{
			Commands: recipes.Commands_builder{
				Entries: []*recipes.Command{
					recipes.Command_builder{
						Name: "test",
						From: recipes.CommandFrom_builder{
							Text: strPtr("Run all tests"),
						}.Build(),
					}.Build(),
				},
			}.Build(),
		}.Build(),
	}.Build()

	result, err := r.Materialize(context.Background(), recipe)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Build map for easier verification
	entries := make(map[string]string)
	for _, e := range result.GetEntries() {
		entries[e.GetFile().GetPath()] = e.GetFile().GetContent()
	}
	assert.Equal(t, "Run all tests", entries[".claude/commands/test.md"])
}

func TestRecipe_Materialize_ContextAndIde(t *testing.T) {
	r := &providers.Recipe{IDE: getIDE()}

	recipe := recipes.Recipe_builder{
		Context: recipes.Context_builder{
			Entries: []*recipes.ContextEntry{
				recipes.ContextEntry_builder{
					Path: "docs/arch.md",
					From: recipes.ContextFrom_builder{
						Text: strPtr("# Architecture"),
					}.Build(),
				}.Build(),
			},
		}.Build(),
		Ide: recipes.Ide_builder{
			Commands: recipes.Commands_builder{
				Entries: []*recipes.Command{
					recipes.Command_builder{
						Name: "deploy",
						From: recipes.CommandFrom_builder{
							Text: strPtr("Deploy to production"),
						}.Build(),
					}.Build(),
				},
			}.Build(),
		}.Build(),
	}.Build()

	result, err := r.Materialize(context.Background(), recipe)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Build map for easier verification
	entries := make(map[string]string)
	for _, e := range result.GetEntries() {
		entries[e.GetFile().GetPath()] = e.GetFile().GetContent()
	}

	assert.Equal(t, "# Architecture", entries["docs/arch.md"])
	assert.Equal(t, "Deploy to production", entries[".claude/commands/deploy.md"])
}

func TestRecipe_Materialize_ComplexRecipe(t *testing.T) {
	r := &providers.Recipe{IDE: getIDE()}

	// Mock HTTP server for GitHub content
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("GitHub content"))
	}))
	defer server.Close()

	recipe := recipes.Recipe_builder{
		Context: recipes.Context_builder{
			Entries: []*recipes.ContextEntry{
				recipes.ContextEntry_builder{
					Path: "context/intro.md",
					From: recipes.ContextFrom_builder{
						Text: strPtr("# Introduction"),
					}.Build(),
				}.Build(),
				recipes.ContextEntry_builder{
					Path: "context/from-cmd.txt",
					From: recipes.ContextFrom_builder{
						Cmd: strPtr("echo 'command output'"),
					}.Build(),
				}.Build(),
				recipes.ContextEntry_builder{
					Path: "context/from-github.md",
					From: recipes.ContextFrom_builder{
						Github: osdd.GitReference_builder{
							Path: server.URL,
						}.Build(),
					}.Build(),
				}.Build(),
			},
		}.Build(),
		Ide: recipes.Ide_builder{
			Commands: recipes.Commands_builder{
				Entries: []*recipes.Command{
					recipes.Command_builder{
						Name: "lint",
						From: recipes.CommandFrom_builder{
							Text: strPtr("Run linting"),
						}.Build(),
					}.Build(),
					recipes.Command_builder{
						Name: "format",
						From: recipes.CommandFrom_builder{
							Cmd: strPtr("echo 'format code'"),
						}.Build(),
					}.Build(),
				},
			}.Build(),
			Permissions: recipes.Permissions_builder{
				Allow: []*recipes.OperationPermission{
					recipes.OperationPermission_builder{
						Bash: strPtr("go test:*"),
					}.Build(),
				},
				Deny: []*recipes.OperationPermission{
					recipes.OperationPermission_builder{
						Write: strPtr("**/secrets/**"),
					}.Build(),
				},
			}.Build(),
			Mcp: recipes.Mcp_builder{
				Servers: map[string]*recipes.McpServer{
					"test-server": recipes.McpServer_builder{
						Stdio: recipes.StdioMcpServer_builder{
							Command: "test-mcp-server",
						}.Build(),
					}.Build(),
				},
			}.Build(),
		}.Build(),
	}.Build()

	result, err := r.Materialize(context.Background(), recipe)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Build map for easier verification
	entries := make(map[string]string)
	for _, e := range result.GetEntries() {
		entries[e.GetFile().GetPath()] = e.GetFile().GetContent()
	}

	// Verify context entries
	assert.Equal(t, "# Introduction", entries["context/intro.md"])
	assert.Equal(t, "command output\n", entries["context/from-cmd.txt"])
	assert.Equal(t, "GitHub content", entries["context/from-github.md"])

	// Verify command entries
	assert.Equal(t, "Run linting", entries[".claude/commands/lint.md"])
	assert.Equal(t, "format code\n", entries[".claude/commands/format.md"])

	// Verify MCP
	mcpContent := entries[".mcp.json"]
	require.NotEmpty(t, mcpContent)

	var mcp struct {
		McpServers map[string]map[string]string `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal([]byte(mcpContent), &mcp))
	assert.Contains(t, mcp.McpServers, "test-server")
	assert.Equal(t, "test-mcp-server", mcp.McpServers["test-server"]["command"])
}

func TestRecipe_Materialize_InvalidContext(t *testing.T) {
	r := &providers.Recipe{IDE: getIDE()}

	recipe := recipes.Recipe_builder{
		Context: recipes.Context_builder{
			Entries: []*recipes.ContextEntry{
				recipes.ContextEntry_builder{
					Path: "test.md",
					// Missing From source
				}.Build(),
			},
		}.Build(),
	}.Build()

	_, err := r.Materialize(context.Background(), recipe)
	assert.Error(t, err, "expected error for invalid context entry")
	assert.Contains(t, err.Error(), "failed to materialize context")
}

func TestRecipe_Materialize_InvalidIde(t *testing.T) {
	r := &providers.Recipe{IDE: getIDE()}

	recipe := recipes.Recipe_builder{
		Ide: recipes.Ide_builder{
			Commands: recipes.Commands_builder{
				Entries: []*recipes.Command{
					recipes.Command_builder{
						Name: "invalid",
						// Missing From source
					}.Build(),
				},
			}.Build(),
		}.Build(),
	}.Build()

	_, err := r.Materialize(context.Background(), recipe)
	assert.Error(t, err, "expected error for invalid IDE command")
	assert.Contains(t, err.Error(), "failed to materialize IDE configuration")
}

func TestRecipe_Materialize_ContextWithCombinedSource(t *testing.T) {
	r := &providers.Recipe{IDE: getIDE()}

	recipe := recipes.Recipe_builder{
		Context: recipes.Context_builder{
			Entries: []*recipes.ContextEntry{
				recipes.ContextEntry_builder{
					Path: "combined.md",
					From: recipes.ContextFrom_builder{
						Combined: recipes.CombinedContextSource_builder{
							Items: []*recipes.CombinedContextSource_Item{
								recipes.CombinedContextSource_Item_builder{
									Text: strPtr("# Header\n"),
								}.Build(),
								recipes.CombinedContextSource_Item_builder{
									Cmd: strPtr("echo 'Body content'"),
								}.Build(),
								recipes.CombinedContextSource_Item_builder{
									Text: strPtr("\n# Footer"),
								}.Build(),
							},
						}.Build(),
					}.Build(),
				}.Build(),
			},
		}.Build(),
	}.Build()

	result, err := r.Materialize(context.Background(), recipe)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.GetEntries(), 1)

	entry := result.GetEntries()[0]
	assert.Equal(t, "combined.md", entry.GetFile().GetPath())
	assert.Equal(t, "# Header\nBody content\n\n# Footer", entry.GetFile().GetContent())
}

func TestRecipe_Materialize_MultiplePermissions(t *testing.T) {
	r := &providers.Recipe{IDE: getIDE()}

	recipe := recipes.Recipe_builder{
		Ide: recipes.Ide_builder{
			Permissions: recipes.Permissions_builder{
				Allow: []*recipes.OperationPermission{
					recipes.OperationPermission_builder{Bash: strPtr("make:*")}.Build(),
					recipes.OperationPermission_builder{Read: strPtr("**/*.go")}.Build(),
					recipes.OperationPermission_builder{Write: strPtr("**/*.md")}.Build(),
				},
				Deny: []*recipes.OperationPermission{
					recipes.OperationPermission_builder{Bash: strPtr("rm -rf:*")}.Build(),
					recipes.OperationPermission_builder{Write: strPtr("/etc/**")}.Build(),
				},
			}.Build(),
		}.Build(),
	}.Build()

	result, err := r.Materialize(context.Background(), recipe)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.GetEntries(), 0)
}

func TestRecipe_Materialize_MultipleMcpServers(t *testing.T) {
	r := &providers.Recipe{IDE: getIDE()}

	recipe := recipes.Recipe_builder{
		Ide: recipes.Ide_builder{
			Mcp: recipes.Mcp_builder{
				Servers: map[string]*recipes.McpServer{
					"http-server": recipes.McpServer_builder{
						Http: recipes.HttpMcpServer_builder{
							Url: "https://example.com/mcp",
						}.Build(),
					}.Build(),
					"stdio-server": recipes.McpServer_builder{
						Stdio: recipes.StdioMcpServer_builder{
							Command: "stdio-mcp",
						}.Build(),
					}.Build(),
					"another-stdio": recipes.McpServer_builder{
						Stdio: recipes.StdioMcpServer_builder{
							Command: "another-mcp-server",
						}.Build(),
					}.Build(),
				},
			}.Build(),
		}.Build(),
	}.Build()

	result, err := r.Materialize(context.Background(), recipe)
	require.NoError(t, err)
	require.NotNil(t, result)

	var mcpContent string
	for _, e := range result.GetEntries() {
		if e.GetFile().GetPath() == ".mcp.json" {
			mcpContent = e.GetFile().GetContent()
			break
		}
	}
	require.NotEmpty(t, mcpContent)

	var mcp struct {
		McpServers map[string]map[string]string `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal([]byte(mcpContent), &mcp))
	assert.Len(t, mcp.McpServers, 3)
	assert.Equal(t, "https://example.com/mcp", mcp.McpServers["http-server"]["url"])
	assert.Equal(t, "stdio-mcp", mcp.McpServers["stdio-server"]["command"])
	assert.Equal(t, "another-mcp-server", mcp.McpServers["another-stdio"]["command"])
}
