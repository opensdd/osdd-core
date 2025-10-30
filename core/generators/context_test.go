package generators

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
	"github.com/opensdd/osdd-core/core"
	"github.com/opensdd/osdd-core/core/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func strPtr(s string) *string {
	return &s
}

func contextEntry(path string, from *recipes.ContextFrom) *recipes.ContextEntry {
	return recipes.ContextEntry_builder{Path: path, From: from}.Build()
}

func textFrom(text string) *recipes.ContextFrom {
	return recipes.ContextFrom_builder{Text: strPtr(text)}.Build()
}

func cmdFrom(cmd string, args ...string) *recipes.ContextFrom {
	return recipes.ContextFrom_builder{Cmd: ex(cmd, args...)}.Build()
}

func githubFrom(path string) *recipes.ContextFrom {
	return recipes.ContextFrom_builder{
		Github: osdd.GitReference_builder{Path: path}.Build(),
	}.Build()
}

func combinedFrom(items ...*recipes.CombinedContextSource_Item) *recipes.ContextFrom {
	return recipes.ContextFrom_builder{
		Combined: recipes.CombinedContextSource_builder{Items: items}.Build(),
	}.Build()
}

func combinedTextItem(text string) *recipes.CombinedContextSource_Item {
	return recipes.CombinedContextSource_Item_builder{Text: strPtr(text)}.Build()
}

func combinedCmdItem(cmd string, args ...string) *recipes.CombinedContextSource_Item {
	return recipes.CombinedContextSource_Item_builder{Cmd: ex(cmd, args...)}.Build()
}

func combinedGithubItem(path string) *recipes.CombinedContextSource_Item {
	return recipes.CombinedContextSource_Item_builder{
		Github: osdd.GitReference_builder{Path: path}.Build(),
	}.Build()
}

func userInputParam(name string, optional bool) *osdd.UserInputParameter {
	return osdd.UserInputParameter_builder{Name: name, Optional: optional}.Build()
}

func userInputFromParams(params ...*osdd.UserInputParameter) *recipes.ContextFrom {
	return recipes.ContextFrom_builder{
		UserInput: recipes.UserInputContextSource_builder{Entries: params}.Build(),
	}.Build()
}

func combinedUserInputItem(params ...*osdd.UserInputParameter) *recipes.CombinedContextSource_Item {
	return recipes.CombinedContextSource_Item_builder{
		UserInput: recipes.UserInputContextSource_builder{Entries: params}.Build(),
	}.Build()
}

func TestContext_Materialize(t *testing.T) {
	tests := []struct {
		name     string
		context  *recipes.Context
		genCtx   *core.GenerationContext
		wantErr  string
		validate func(*testing.T, *osdd.MaterializedResult)
	}{
		{
			name:    "nil context",
			wantErr: "context cannot be nil",
		},
		{
			name:    "nil entries",
			context: recipes.Context_builder{}.Build(),
			validate: func(t *testing.T, result *osdd.MaterializedResult) {
				assert.NotNil(t, result)
				assert.Empty(t, result.GetEntries())
			},
		},
		{
			name:    "empty entries",
			context: recipes.Context_builder{Entries: []*recipes.ContextEntry{}}.Build(),
			validate: func(t *testing.T, result *osdd.MaterializedResult) {
				assert.NotNil(t, result)
				assert.Empty(t, result.GetEntries())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Context{}
			result, err := c.Materialize(context.Background(), tt.context, tt.genCtx)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestContext_MaterializeEntry(t *testing.T) {
	tests := []struct {
		name    string
		entry   *recipes.ContextEntry
		genCtx  *core.GenerationContext
		wantErr string
	}{
		{
			name:    "empty path",
			entry:   recipes.ContextEntry_builder{}.Build(),
			wantErr: "entry path cannot be empty",
		},
		{
			name:    "no from source",
			entry:   recipes.ContextEntry_builder{Path: "test.txt"}.Build(),
			wantErr: "entry must have a 'from' source",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Context{}
			_, err := c.materializeEntry(context.Background(), tt.entry, tt.genCtx)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestContext_FetchContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("github content"))
	}))
	defer server.Close()

	tests := []struct {
		name    string
		from    *recipes.ContextFrom
		genCtx  *core.GenerationContext
		want    string
		wantErr string
	}{
		{
			name: "text source",
			from: textFrom("hello world"),
			want: "hello world",
		},
		{
			name: "command source",
			from: cmdFrom("echo", "test output"),
			want: "test output\n",
		},
		{
			name: "github source",
			from: githubFrom(server.URL),
			want: "github content",
		},
		{
			name: "combined source",
			from: combinedFrom(
				combinedTextItem("# Overview: "),
				combinedCmdItem("echo", "from command"),
				combinedTextItem("\n# End"),
			),
			want: "# Overview: from command\n\n# End",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Context{}
			content, err := c.fetchContent(context.Background(), tt.from, tt.genCtx)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, content)
		})
	}
}

func ex(cmd string, args ...string) *osdd.Exec {
	return osdd.Exec_builder{
		Cmd:  cmd,
		Args: args,
	}.Build()
}

func TestUtils_ExecuteCommand(t *testing.T) {
	tests := []struct {
		name    string
		cmd     *osdd.Exec
		want    string
		wantErr bool
	}{
		{
			name: "success",
			cmd:  ex("echo", "test output"),
			want: "test output\n",
		},
		{
			name:    "empty command",
			cmd:     ex(""),
			wantErr: true,
		},
		{
			name:    "failed command",
			cmd:     ex("exit", "1"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := utils.ExecuteCommand(context.Background(), tt.cmd)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, content)
		})
	}
}

func TestUtils_FetchGithub(t *testing.T) {
	successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("file content from github"))
	}))
	defer successServer.Close()

	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer errorServer.Close()

	tests := []struct {
		name    string
		ref     *osdd.GitReference
		want    string
		wantErr bool
	}{
		{
			name: "success",
			ref:  osdd.GitReference_builder{Path: successServer.URL}.Build(),
			want: "file content from github",
		},
		{
			name:    "nil reference",
			ref:     nil,
			wantErr: true,
		},
		{
			name:    "empty path",
			ref:     osdd.GitReference_builder{}.Build(),
			wantErr: true,
		},
		{
			name:    "HTTP error",
			ref:     osdd.GitReference_builder{Path: errorServer.URL + "/notfound"}.Build(),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := utils.FetchGithub(context.Background(), tt.ref)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, content)
		})
	}
}

func TestContext_FetchCombined(t *testing.T) {
	tests := []struct {
		name     string
		combined *recipes.CombinedContextSource
		genCtx   *core.GenerationContext
		want     string
		wantErr  string
	}{
		{
			name: "success",
			combined: recipes.CombinedContextSource_builder{
				Items: []*recipes.CombinedContextSource_Item{
					combinedTextItem("# Overview: "),
					combinedCmdItem("echo", "from command"),
					combinedTextItem("\n# End"),
				},
			}.Build(),
			want: "# Overview: from command\n\n# End",
		},
		{
			name:    "nil combined",
			wantErr: "combined source cannot be nil",
		},
		{
			name:     "empty items",
			combined: recipes.CombinedContextSource_builder{}.Build(),
			want:     "",
		},
		{
			name: "failed item",
			combined: recipes.CombinedContextSource_builder{
				Items: []*recipes.CombinedContextSource_Item{
					combinedTextItem("text1"),
					combinedCmdItem("exit", "1"),
				},
			}.Build(),
			wantErr: "failed to fetch combined item",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Context{}
			content, err := c.fetchCombined(context.Background(), tt.combined, tt.genCtx)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, content)
		})
	}
}

func TestContext_FetchCombinedItem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("github content"))
	}))
	defer server.Close()

	tests := []struct {
		name    string
		item    *recipes.CombinedContextSource_Item
		genCtx  *core.GenerationContext
		want    string
		wantErr string
	}{
		{
			name: "text",
			item: combinedTextItem("test text"),
			want: "test text",
		},
		{
			name: "cmd",
			item: combinedCmdItem("echo", "cmd output"),
			want: "cmd output\n",
		},
		{
			name: "github",
			item: combinedGithubItem(server.URL),
			want: "github content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Context{}
			content, err := c.fetchCombinedItem(context.Background(), tt.item, tt.genCtx)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, content)
		})
	}
}

func ExampleContext_Materialize() {
	c := &Context{}

	ctx := recipes.Context_builder{
		Entries: []*recipes.ContextEntry{
			contextEntry("example.txt", textFrom("This is example content")),
		},
	}.Build()

	result, err := c.Materialize(context.Background(), ctx, nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	for _, entry := range result.GetEntries() {
		if entry.HasFile() {
			file := entry.GetFile()
			fmt.Printf("Path: %s, Content Length: %d\n", file.GetPath(), len(file.GetContent()))
		}
	}
	// Output: Path: example.txt, Content Length: 23
}

// --- New tests for UserInput materialization ---
func TestContext_FetchContent_UserInput_Success(t *testing.T) {
	c := &Context{}
	from := userInputFromParams(
		userInputParam("A", false),
		userInputParam("B", true),
		userInputParam("C", false),
	)
	genCtx := &core.GenerationContext{UserInput: map[string]string{
		"A": "first",
		"C": "third",
	}}

	content, err := c.fetchContent(context.Background(), from, genCtx)
	require.NoError(t, err)
	// Validate markdown structure and values
	assert.Contains(t, content, "# User Input")
	// Ensure sections for A, B, C exist and values are placed correctly
	assert.Contains(t, content, "## A")
	assert.Contains(t, content, "**Value**: first")
	assert.Contains(t, content, "## B")
	assert.Contains(t, content, "**Value**: ") // optional missing -> empty value
	assert.Contains(t, content, "## C")
	assert.Contains(t, content, "**Value**: third")
	// Ensure ordering A before C
	assert.Less(t, strings.Index(content, "## A"), strings.Index(content, "## C"))
}

func TestContext_FetchContent_UserInput_MissingRequired(t *testing.T) {
	c := &Context{}
	from := userInputFromParams(
		userInputParam("A", false),
		userInputParam("B", false),
	)
	genCtx := &core.GenerationContext{UserInput: map[string]string{
		"A": "value-a",
	}}

	_, err := c.fetchContent(context.Background(), from, genCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required user input parameters")
	assert.Contains(t, err.Error(), "B")
}

func TestContext_FetchCombined_UserInput_Mixed(t *testing.T) {
	c := &Context{}
	combined := recipes.CombinedContextSource_builder{
		Items: []*recipes.CombinedContextSource_Item{
			combinedTextItem("prefix-"),
			combinedUserInputItem(userInputParam("X", false)),
			combinedTextItem("-suffix"),
		},
	}.Build()

	genCtx := &core.GenerationContext{UserInput: map[string]string{
		"X": "middle",
	}}

	content, err := c.fetchCombined(context.Background(), combined, genCtx)
	require.NoError(t, err)
	// Should include our prefix, the markdown header and value, and the suffix
	assert.Contains(t, content, "prefix-")
	assert.Contains(t, content, "# User Input")
	assert.Contains(t, content, "## X")
	assert.Contains(t, content, "**Value**: middle")
	assert.Contains(t, content, "-suffix")
}

func TestContext_Materialize_UserInput_MissingRequired(t *testing.T) {
	t.Parallel()
	c := &Context{}

	ctx := recipes.Context_builder{
		Entries: []*recipes.ContextEntry{
			recipes.ContextEntry_builder{
				Path: "out.txt",
				From: userInputFromParams(
					userInputParam("REQ", false),
				),
			}.Build(),
		},
	}.Build()

	_, err := c.Materialize(context.Background(), ctx, &core.GenerationContext{UserInput: map[string]string{}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to materialize entry for path out.txt")
	assert.Contains(t, err.Error(), "missing required user input parameters")
	assert.Contains(t, err.Error(), "REQ")
}

func TestContext_Materialize_Combined_UserInput_MissingRequired(t *testing.T) {
	t.Parallel()
	c := &Context{}

	from := recipes.ContextFrom_builder{
		Combined: recipes.CombinedContextSource_builder{
			Items: []*recipes.CombinedContextSource_Item{
				combinedTextItem("prefix-"),
				combinedUserInputItem(userInputParam("REQ2", false)),
			},
		}.Build(),
	}.Build()

	ctx := recipes.Context_builder{
		Entries: []*recipes.ContextEntry{
			recipes.ContextEntry_builder{
				Path: "combined.txt",
				From: from,
			}.Build(),
		},
	}.Build()

	_, err := c.Materialize(context.Background(), ctx, &core.GenerationContext{UserInput: map[string]string{}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to materialize entry for path combined.txt")
	assert.Contains(t, err.Error(), "failed to fetch combined item")
	assert.Contains(t, err.Error(), "missing required user input parameters")
	assert.Contains(t, err.Error(), "REQ2")
}
