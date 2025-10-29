package generators

import (
	"os"
	"testing"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
	"github.com/opensdd/osdd-core/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildUserInputSpec(params ...*osdd.UserInputParameter) *recipes.UserInputContextSource {
	return recipes.UserInputContextSource_builder{Entries: params}.Build()
}

func readTestdata(t *testing.T, path string) string {
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

func TestRenderUserInput_Success(t *testing.T) {
	spec := buildUserInputSpec(
		osdd.UserInputParameter_builder{Name: "Repo", Description: "Repository URL", Optional: false}.Build(),
		osdd.UserInputParameter_builder{Name: "Branch", Description: "Git branch", Optional: true}.Build(),
	)
	ctx := &core.GenerationContext{UserInput: map[string]string{
		"Repo":   "https://github.com/opensdd/osdd-core",
		"Branch": "main",
	}}

	out, err := renderUserInput(spec, ctx)
	require.NoError(t, err)
	expected := readTestdata(t, "testdata/user_input_success.md")
	assert.Equal(t, expected, out)
}

func TestRenderUserInput_OptionalMissing_RendersEmptyValue(t *testing.T) {
	spec := buildUserInputSpec(
		osdd.UserInputParameter_builder{Name: "Repo", Description: "Repository URL", Optional: false}.Build(),
		osdd.UserInputParameter_builder{Name: "Notes", Description: "Optional notes", Optional: true}.Build(),
	)
	ctx := &core.GenerationContext{UserInput: map[string]string{
		"Repo": "https://github.com/opensdd/osdd-core",
	}}

	out, err := renderUserInput(spec, ctx)
	require.NoError(t, err)
	expected := readTestdata(t, "testdata/user_input_optional_missing.md")
	assert.Equal(t, expected, out)
}

func TestRenderUserInput_MissingRequired(t *testing.T) {
	spec := buildUserInputSpec(
		osdd.UserInputParameter_builder{Name: "Repo", Optional: false}.Build(),
	)
	ctx := &core.GenerationContext{UserInput: map[string]string{}}

	out, err := renderUserInput(spec, ctx)
	assert.Empty(t, out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required user input parameters")
	assert.Contains(t, err.Error(), "Repo")
}

func TestRenderUserInput_EmptySpec(t *testing.T) {
	spec := buildUserInputSpec()
	out, err := renderUserInput(spec, &core.GenerationContext{})
	require.NoError(t, err)
	assert.Equal(t, "", out)
}

func TestRenderUserInput_NilSpec(t *testing.T) {
	out, err := renderUserInput(nil, &core.GenerationContext{})
	assert.Empty(t, out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user input specification cannot be nil")
}
