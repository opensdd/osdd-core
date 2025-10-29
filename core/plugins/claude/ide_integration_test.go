//go:build integration

package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration Tests for Merge Functionality

func TestIDE_Materialize_Permissions_MergeWithExisting(t *testing.T) {
	// Setup: Create a temporary directory and existing settings file
	tempDir := t.TempDir()
	claudeDir := filepath.Join(tempDir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0755))

	// Change to temp directory for the test
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tempDir))
	defer func() { _ = os.Chdir(origDir) }()

	// Create existing settings file with some permissions
	existingSettings := `{
  "permissions": {
    "allow": [
      "Bash(git status:*)",
      "Read(/etc/hosts)"
    ],
    "deny": [
      "Write(/etc/**)"
    ],
    "ask": [
      "Bash(rm:*)"
    ]
  }
}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.local.json"), []byte(existingSettings), 0644))

	allowBash := recipes.OperationPermission_builder{Bash: strPtr("go test:*")}.Build()
	allowRead := recipes.OperationPermission_builder{Read: strPtr("~/.zshrc")}.Build()
	denyWrite := recipes.OperationPermission_builder{Write: strPtr("**/secrets/**")}.Build()

	ide := recipes.Ide_builder{
		Permissions: recipes.Permissions_builder{
			Allow: []*recipes.OperationPermission{allowBash, allowRead},
			Deny:  []*recipes.OperationPermission{denyWrite},
		}.Build(),
	}.Build()

	// Execute
	res, err := materializePermissions(ide.GetPermissions(), nil, nil)
	require.NoError(t, err)
	require.NotNil(t, res)

	// Verify the merged result
	var settingsContent string
	for _, e := range res {
		if e.GetFile().GetPath() == ".claude/settings.local.json" {
			settingsContent = e.GetFile().GetContent()
			break
		}
	}
	require.NotEmpty(t, settingsContent)

	var parsed struct {
		Permissions struct {
			Allow []string `json:"allow"`
			Deny  []string `json:"deny"`
			Ask   []string `json:"ask"`
		} `json:"permissions"`
	}
	require.NoError(t, json.Unmarshal([]byte(settingsContent), &parsed))

	// Verify existing permissions are preserved
	assert.Contains(t, parsed.Permissions.Allow, "Bash(git status:*)", "existing allow should be preserved")
	assert.Contains(t, parsed.Permissions.Allow, "Read(/etc/hosts)", "existing allow should be preserved")
	assert.Contains(t, parsed.Permissions.Deny, "Write(/etc/**)", "existing deny should be preserved")
	assert.Contains(t, parsed.Permissions.Ask, "Bash(rm:*)", "existing ask should be preserved")

	// Verify new permissions are added
	assert.Contains(t, parsed.Permissions.Allow, "Bash(go test:*)", "new allow should be added")
	assert.Contains(t, parsed.Permissions.Allow, "Read(~/.zshrc)", "new allow should be added")
	assert.Contains(t, parsed.Permissions.Deny, "Write(**/secrets/**)", "new deny should be added")

	// Verify total counts
	assert.Len(t, parsed.Permissions.Allow, 4, "should have 2 existing + 2 new allows")
	assert.Len(t, parsed.Permissions.Deny, 2, "should have 1 existing + 1 new deny")
	assert.Len(t, parsed.Permissions.Ask, 1, "should have 1 existing ask")
}

func TestIDE_Materialize_McpServers_AutoAddPermissions(t *testing.T) {
	// Setup: Create a temporary directory
	tempDir := t.TempDir()
	claudeDir := filepath.Join(tempDir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0755))

	// Change to temp directory for the test
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tempDir))
	defer func() { _ = os.Chdir(origDir) }()

	ide := recipes.Ide_builder{
		Mcp: recipes.Mcp_builder{Servers: map[string]*recipes.McpServer{
			"github":     recipes.McpServer_builder{Http: recipes.HttpMcpServer_builder{Url: "https://api.github.com/mcp/"}.Build()}.Build(),
			"devplan":    recipes.McpServer_builder{Stdio: recipes.StdioMcpServer_builder{Command: "devplan mcp"}.Build()}.Build(),
			"filesystem": recipes.McpServer_builder{Stdio: recipes.StdioMcpServer_builder{Command: "npx @mcp/server-filesystem"}.Build()}.Build(),
		}}.Build(),
	}.Build()

	// Execute
	res, err := materializePermissions(ide.GetPermissions(), []string{"github", "devplan", "filesystem"}, nil)
	require.NoError(t, err)
	require.NotNil(t, res)

	// Verify settings.local.json was created with MCP permissions
	var settingsContent string
	for _, e := range res {
		if e.GetFile().GetPath() == ".claude/settings.local.json" {
			settingsContent = e.GetFile().GetContent()
			break
		}
	}
	require.NotEmpty(t, settingsContent, "settings.local.json should be created even without explicit permissions")

	var parsed struct {
		Permissions struct {
			Allow []string `json:"allow"`
			Deny  []string `json:"deny"`
			Ask   []string `json:"ask"`
		} `json:"permissions"`
		EnabledMcpjsonServers []string `json:"enabledMcpjsonServers"`
	}
	require.NoError(t, json.Unmarshal([]byte(settingsContent), &parsed))

	// Verify MCP server names were automatically added to enabledMcpjsonServers
	assert.Len(t, parsed.EnabledMcpjsonServers, 3, "should have all 3 MCP servers enabled")
	assert.Contains(t, parsed.EnabledMcpjsonServers, "github", "github MCP server should be enabled")
	assert.Contains(t, parsed.EnabledMcpjsonServers, "devplan", "devplan MCP server should be enabled")
	assert.Contains(t, parsed.EnabledMcpjsonServers, "filesystem", "filesystem MCP server should be enabled")

	// Verify MCP server permissions were also added to allow list with mcp__ prefix
	assert.Len(t, parsed.Permissions.Allow, 3, "should have mcp__ permissions for all 3 MCP servers")
	assert.Contains(t, parsed.Permissions.Allow, "mcp__github", "mcp__github permission should be in allow list")
	assert.Contains(t, parsed.Permissions.Allow, "mcp__devplan", "mcp__devplan permission should be in allow list")
	assert.Contains(t, parsed.Permissions.Allow, "mcp__filesystem", "mcp__filesystem permission should be in allow list")
}

func TestIDE_Materialize_Permissions_Deduplication(t *testing.T) {
	// Setup: Create a temporary directory and existing settings file
	tempDir := t.TempDir()
	claudeDir := filepath.Join(tempDir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0755))

	// Change to temp directory for the test
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tempDir))
	defer func() { _ = os.Chdir(origDir) }()

	// Create existing settings file with duplicate permission
	existingSettings := `{
  "permissions": {
    "allow": [
      "Bash(go test:*)",
      "Read(/etc/hosts)"
    ],
    "deny": [],
    "ask": []
  }
}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.local.json"), []byte(existingSettings), 0644))

	// Define new permissions including a duplicate
	allowBash := recipes.OperationPermission_builder{Bash: strPtr("go test:*")}.Build() // Duplicate!
	allowRead := recipes.OperationPermission_builder{Read: strPtr("~/.zshrc")}.Build()

	ide := recipes.Ide_builder{
		Permissions: recipes.Permissions_builder{
			Allow: []*recipes.OperationPermission{allowBash, allowRead},
		}.Build(),
	}.Build()

	// Execute
	res, err := materializePermissions(ide.GetPermissions(), nil, nil)
	require.NoError(t, err)
	require.NotNil(t, res)

	// Verify the merged result
	var settingsContent string
	for _, e := range res {
		if e.GetFile().GetPath() == ".claude/settings.local.json" {
			settingsContent = e.GetFile().GetContent()
			break
		}
	}
	require.NotEmpty(t, settingsContent)

	var parsed struct {
		Permissions struct {
			Allow []string `json:"allow"`
			Deny  []string `json:"deny"`
			Ask   []string `json:"ask"`
		} `json:"permissions"`
	}
	require.NoError(t, json.Unmarshal([]byte(settingsContent), &parsed))

	// Verify no duplicates
	assert.Len(t, parsed.Permissions.Allow, 3, "should have 3 unique allows (2 existing + 1 new, with 1 duplicate removed)")
	assert.Contains(t, parsed.Permissions.Allow, "Bash(go test:*)")
	assert.Contains(t, parsed.Permissions.Allow, "Read(/etc/hosts)")
	assert.Contains(t, parsed.Permissions.Allow, "Read(~/.zshrc)")

	// Count occurrences to ensure no duplicates
	count := 0
	for _, p := range parsed.Permissions.Allow {
		if p == "Bash(go test:*)" {
			count++
		}
	}
	assert.Equal(t, 1, count, "duplicate permission should appear only once")
}

func TestIDE_Materialize_Permissions_InvalidExistingJSON(t *testing.T) {
	// Setup: Create a temporary directory with invalid JSON
	tempDir := t.TempDir()
	claudeDir := filepath.Join(tempDir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0755))

	// Change to temp directory for the test
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tempDir))
	defer func() { _ = os.Chdir(origDir) }()

	// Create existing settings file with invalid JSON
	invalidJSON := `{ "permissions": { "allow": ["test" }`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.local.json"), []byte(invalidJSON), 0644))

	// Define new permissions
	allowBash := recipes.OperationPermission_builder{Bash: strPtr("go test:*")}.Build()

	ide := recipes.Ide_builder{
		Permissions: recipes.Permissions_builder{
			Allow: []*recipes.OperationPermission{allowBash},
		}.Build(),
	}.Build()

	// Execute - should not error, just start fresh
	res, err := materializePermissions(ide.GetPermissions(), nil, nil)
	require.NoError(t, err)
	require.NotNil(t, res)

	// Verify the result contains only new permissions (old invalid JSON was ignored)
	var settingsContent string
	for _, e := range res {
		if e.GetFile().GetPath() == ".claude/settings.local.json" {
			settingsContent = e.GetFile().GetContent()
			break
		}
	}
	require.NotEmpty(t, settingsContent)

	var parsed struct {
		Permissions struct {
			Allow []string `json:"allow"`
			Deny  []string `json:"deny"`
			Ask   []string `json:"ask"`
		} `json:"permissions"`
	}
	require.NoError(t, json.Unmarshal([]byte(settingsContent), &parsed))

	// Should only have new permission
	assert.Len(t, parsed.Permissions.Allow, 1)
	assert.Contains(t, parsed.Permissions.Allow, "Bash(go test:*)")
}

func TestIDE_Materialize_Permissions_NoExistingFile(t *testing.T) {
	// Setup: Create a temporary directory without existing settings
	tempDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, ".claude"), 0755))

	// Change to temp directory for the test
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tempDir))
	defer func() { _ = os.Chdir(origDir) }()

	// Define new permissions
	allowBash := recipes.OperationPermission_builder{Bash: strPtr("go test:*")}.Build()

	ide := recipes.Ide_builder{
		Permissions: recipes.Permissions_builder{
			Allow: []*recipes.OperationPermission{allowBash},
		}.Build(),
	}.Build()

	// Execute
	res, err := materializePermissions(ide.GetPermissions(), nil, nil)
	require.NoError(t, err)
	require.NotNil(t, res)

	// Verify the result contains only new permissions
	var settingsContent string
	for _, e := range res {
		if e.GetFile().GetPath() == ".claude/settings.local.json" {
			settingsContent = e.GetFile().GetContent()
			break
		}
	}
	require.NotEmpty(t, settingsContent)

	var parsed struct {
		Permissions struct {
			Allow []string `json:"allow"`
			Deny  []string `json:"deny"`
			Ask   []string `json:"ask"`
		} `json:"permissions"`
	}
	require.NoError(t, json.Unmarshal([]byte(settingsContent), &parsed))

	// Should only have new permission
	assert.Len(t, parsed.Permissions.Allow, 1)
	assert.Contains(t, parsed.Permissions.Allow, "Bash(go test:*)")
	assert.Empty(t, parsed.Permissions.Deny)
	assert.Empty(t, parsed.Permissions.Ask)
}

func TestIDE_Materialize_McpServers_PreserveExistingMcpPermissions(t *testing.T) {
	// Setup: Create a temporary directory with existing MCP permissions
	tempDir := t.TempDir()
	claudeDir := filepath.Join(tempDir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0755))

	// Change to temp directory for the test
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tempDir))
	defer func() { _ = os.Chdir(origDir) }()

	// Create existing settings with MCP server already enabled and in allow list
	existingSettings := `{
  "permissions": {
    "allow": [
      "Bash(git status:*)",
      "mcp__github"
    ],
    "deny": [],
    "ask": []
  },
  "enabledMcpjsonServers": ["github"]
}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.local.json"), []byte(existingSettings), 0644))

	// Define the same MCP server again (should not duplicate)
	ide := recipes.Ide_builder{
		Mcp: recipes.Mcp_builder{Servers: map[string]*recipes.McpServer{
			"github": recipes.McpServer_builder{Http: recipes.HttpMcpServer_builder{Url: "https://api.github.com/mcp/"}.Build()}.Build(),
		}}.Build(),
	}.Build()

	// Execute
	res, err := materializePermissions(ide.GetPermissions(), []string{"github"}, nil)
	require.NoError(t, err)
	require.NotNil(t, res)

	// Verify no duplicate MCP server
	var settingsContent string
	for _, e := range res {
		if e.GetFile().GetPath() == ".claude/settings.local.json" {
			settingsContent = e.GetFile().GetContent()
			break
		}
	}
	require.NotEmpty(t, settingsContent)

	var parsed struct {
		Permissions struct {
			Allow []string `json:"allow"`
			Deny  []string `json:"deny"`
			Ask   []string `json:"ask"`
		} `json:"permissions"`
		EnabledMcpjsonServers []string `json:"enabledMcpjsonServers"`
	}
	require.NoError(t, json.Unmarshal([]byte(settingsContent), &parsed))

	// Verify permissions remain with no duplicates
	assert.Len(t, parsed.Permissions.Allow, 2, "should have 2 permissions (no duplicate mcp__github)")
	assert.Contains(t, parsed.Permissions.Allow, "Bash(git status:*)")
	assert.Contains(t, parsed.Permissions.Allow, "mcp__github")

	// Count occurrences in allow list to ensure no duplicates
	allowCount := 0
	for _, p := range parsed.Permissions.Allow {
		if p == "mcp__github" {
			allowCount++
		}
	}
	assert.Equal(t, 1, allowCount, "mcp__github permission should appear only once in allow list")

	// Verify no duplicate MCP server in enabledMcpjsonServers
	assert.Len(t, parsed.EnabledMcpjsonServers, 1, "should have 1 enabled MCP server (no duplicate)")
	assert.Contains(t, parsed.EnabledMcpjsonServers, "github")

	// Count occurrences in enabledMcpjsonServers to ensure no duplicates
	enabledCount := 0
	for _, s := range parsed.EnabledMcpjsonServers {
		if s == "github" {
			enabledCount++
		}
	}
	assert.Equal(t, 1, enabledCount, "github server should appear only once in enabledMcpjsonServers")
}

func TestIDE_Materialize_McpServers_MergeWithExistingPermissions(t *testing.T) {
	// Setup: Create a temporary directory with existing permissions
	tempDir := t.TempDir()
	claudeDir := filepath.Join(tempDir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0755))

	// Change to temp directory for the test
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tempDir))
	defer func() { _ = os.Chdir(origDir) }()

	// Create existing settings with some permissions
	existingSettings := `{
  "permissions": {
    "allow": [
      "Bash(git status:*)",
      "Read(/etc/hosts)"
    ],
    "deny": [],
    "ask": []
  }
}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.local.json"), []byte(existingSettings), 0644))

	// Define MCP servers and additional permissions
	allowBash := recipes.OperationPermission_builder{Bash: strPtr("go test:*")}.Build()

	ide := recipes.Ide_builder{
		Permissions: recipes.Permissions_builder{
			Allow: []*recipes.OperationPermission{allowBash},
		}.Build(),
		Mcp: recipes.Mcp_builder{Servers: map[string]*recipes.McpServer{
			"github":  recipes.McpServer_builder{Http: recipes.HttpMcpServer_builder{Url: "https://api.github.com/mcp/"}.Build()}.Build(),
			"devplan": recipes.McpServer_builder{Stdio: recipes.StdioMcpServer_builder{Command: "devplan mcp"}.Build()}.Build(),
		}}.Build(),
	}.Build()

	// Execute
	res, err := materializePermissions(ide.GetPermissions(), []string{"github", "devplan"}, nil)
	require.NoError(t, err)
	require.NotNil(t, res)

	// Verify settings were merged
	var settingsContent string
	for _, e := range res {
		if e.GetFile().GetPath() == ".claude/settings.local.json" {
			settingsContent = e.GetFile().GetContent()
			break
		}
	}
	require.NotEmpty(t, settingsContent)

	var parsed struct {
		Permissions struct {
			Allow []string `json:"allow"`
			Deny  []string `json:"deny"`
			Ask   []string `json:"ask"`
		} `json:"permissions"`
		EnabledMcpjsonServers []string `json:"enabledMcpjsonServers"`
	}
	require.NoError(t, json.Unmarshal([]byte(settingsContent), &parsed))

	// Verify existing permissions are preserved
	assert.Contains(t, parsed.Permissions.Allow, "Bash(git status:*)", "existing permission should be preserved")
	assert.Contains(t, parsed.Permissions.Allow, "Read(/etc/hosts)", "existing permission should be preserved")

	// Verify new explicit permission was added
	assert.Contains(t, parsed.Permissions.Allow, "Bash(go test:*)", "new permission should be added")

	// Verify MCP permissions were added to allow list
	assert.Contains(t, parsed.Permissions.Allow, "mcp__github", "mcp__github permission should be in allow list")
	assert.Contains(t, parsed.Permissions.Allow, "mcp__devplan", "mcp__devplan permission should be in allow list")

	// Verify total permission count (existing + new + MCP)
	assert.Len(t, parsed.Permissions.Allow, 5, "should have 2 existing + 1 new + 2 MCP permissions")

	// Verify MCP servers were added to enabledMcpjsonServers
	assert.Len(t, parsed.EnabledMcpjsonServers, 2, "should have 2 MCP servers enabled")
	assert.Contains(t, parsed.EnabledMcpjsonServers, "github", "github MCP server should be enabled")
	assert.Contains(t, parsed.EnabledMcpjsonServers, "devplan", "devplan MCP server should be enabled")
}
