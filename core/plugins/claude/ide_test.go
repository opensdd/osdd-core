package claude

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIDE_Materialize_Permissions(t *testing.T) {
	allowBash := recipes.OperationPermission_builder{Bash: strPtr("go test:*")}.Build()
	allowRead := recipes.OperationPermission_builder{Read: strPtr("~/.zshrc")}.Build()
	denyWrite := recipes.OperationPermission_builder{Write: strPtr("**/secrets/**")}.Build()

	ide := recipes.Ide_builder{
		Permissions: recipes.Permissions_builder{
			Allow: []*recipes.OperationPermission{allowBash, allowRead},
			Deny:  []*recipes.OperationPermission{denyWrite},
		}.Build(),
	}.Build()

	res, err := materializePermissions(ide.GetPermissions(), nil, nil)
	require.NoError(t, err)
	require.NotNil(t, res)

	// Find settings file
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
	assert.Contains(t, parsed.Permissions.Allow, "Bash(go test:*)")
	assert.Contains(t, parsed.Permissions.Allow, "Read(~/.zshrc)")
	assert.Contains(t, parsed.Permissions.Deny, "Write(**/secrets/**)")
}

func TestIDE_Materialize_Commands_AddedToAllow(t *testing.T) {
	g := NewIDEProvider()

	ide := recipes.Ide_builder{
		Commands: recipes.Commands_builder{Entries: []*recipes.Command{
			recipes.Command_builder{Name: "refine", From: recipes.CommandFrom_builder{Text: strPtr("content1")}.Build()}.Build(),
			recipes.Command_builder{Name: "run", From: recipes.CommandFrom_builder{Text: strPtr("content2")}.Build()}.Build(),
		}}.Build(),
	}.Build()

	res, err := g.Materialize(context.Background(), ide)
	require.NoError(t, err)
	require.NotNil(t, res)

	// Find settings file
	var settingsContent string
	for _, e := range res.GetEntries() {
		if e.GetFile().GetPath() == ".claude/settings.local.json" {
			settingsContent = e.GetFile().GetContent()
			break
		}
	}
	require.NotEmpty(t, settingsContent)

	var parsed struct {
		Permissions struct {
			Allow []string `json:"allow"`
		} `json:"permissions"`
	}
	require.NoError(t, json.Unmarshal([]byte(settingsContent), &parsed))
	assert.Contains(t, parsed.Permissions.Allow, "SlashCommand(/refine)")
	assert.Contains(t, parsed.Permissions.Allow, "SlashCommand(/run)")
}

func TestIDE_Materialize_Commands_TextAndCmd(t *testing.T) {
	g := NewIDEProvider()

	cmdOutput := "hello from cmd\n"
	ide := recipes.Ide_builder{
		Commands: recipes.Commands_builder{
			Entries: []*recipes.Command{
				recipes.Command_builder{
					Name: "refine",
					From: recipes.CommandFrom_builder{Text: strPtr("some text content")}.Build(),
				}.Build(),
				recipes.Command_builder{
					Name: "run",
					From: recipes.CommandFrom_builder{Cmd: strPtr("echo 'hello from cmd'")}.Build(),
				}.Build(),
			},
		}.Build(),
	}.Build()

	res, err := g.Materialize(context.Background(), ide)
	require.NoError(t, err)
	require.NotNil(t, res)

	// Verify contents
	m := map[string]string{}
	for _, e := range res.GetEntries() {
		m[e.GetFile().GetPath()] = e.GetFile().GetContent()
	}
	assert.Equal(t, "some text content", m[".claude/commands/refine.md"])
	assert.Equal(t, cmdOutput, m[".claude/commands/run.md"])
}

func TestIDE_Materialize_Command_Github(t *testing.T) {
	g := NewIDEProvider()

	// Mock HTTP server to serve content
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("from github"))
	}))
	defer ts.Close()

	ide := recipes.Ide_builder{
		Commands: recipes.Commands_builder{
			Entries: []*recipes.Command{
				recipes.Command_builder{
					Name: "gh",
					From: recipes.CommandFrom_builder{Github: osdd.GitReference_builder{Path: ts.URL}.Build()}.Build(),
				}.Build(),
			},
		}.Build(),
	}.Build()

	res, err := g.Materialize(context.Background(), ide)
	require.NoError(t, err)
	require.NotNil(t, res)
	// Find the command entry for gh
	var foundPath string
	var foundContent string
	for _, e := range res.GetEntries() {
		if e.GetFile().GetPath() == ".claude/commands/gh.md" {
			foundPath = e.GetFile().GetPath()
			foundContent = e.GetFile().GetContent()
			break
		}
	}
	require.Equal(t, ".claude/commands/gh.md", foundPath)
	assert.Equal(t, "from github", foundContent)
}

func strPtr(s string) *string {
	return &s
}
