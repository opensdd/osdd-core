package utils

import (
	"testing"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to convert string to *string for builder pattern
func strPtr(s string) *string {
	return &s
}

func TestConvertToRawURL_AlreadyRaw(t *testing.T) {
	t.Parallel()
	url := "https://raw.githubusercontent.com/owner/repo/main/file.md"
	result, err := ConvertToRawURL(url, nil)
	require.NoError(t, err)
	assert.Equal(t, url, result)
}

func TestConvertToRawURL_GithubURL(t *testing.T) {
	t.Parallel()
	url := "https://github.com/myorg/standards/CONTRIBUTING.MD"
	result, err := ConvertToRawURL(url, nil)
	require.NoError(t, err)
	assert.Equal(t, "https://raw.githubusercontent.com/myorg/standards/main/CONTRIBUTING.MD", result)
}

func TestConvertToRawURL_WithTag(t *testing.T) {
	t.Parallel()
	url := "https://github.com/myorg/repo/file.md"
	version := osdd.GitVersion_builder{
		Tag: strPtr("v1.2.3"),
	}.Build()

	result, err := ConvertToRawURL(url, version)
	require.NoError(t, err)
	assert.Equal(t, "https://raw.githubusercontent.com/myorg/repo/v1.2.3/file.md", result)
}

func TestConvertToRawURL_WithCommit(t *testing.T) {
	t.Parallel()
	url := "https://github.com/myorg/repo/file.md"
	version := osdd.GitVersion_builder{
		Commit: strPtr("abc123"),
	}.Build()

	result, err := ConvertToRawURL(url, version)
	require.NoError(t, err)
	assert.Equal(t, "https://raw.githubusercontent.com/myorg/repo/abc123/file.md", result)
}

func TestConvertToRawURL_InvalidFormat(t *testing.T) {
	t.Parallel()
	url := "https://github.com/invalid"
	_, err := ConvertToRawURL(url, nil)
	assert.Error(t, err, "expected error for invalid github path format")
}

func TestConvertToRawURL_BlobFormat(t *testing.T) {
	t.Parallel()
	url := "https://github.com/devplaninc/devplan-cli/blob/main/README.md"
	result, err := ConvertToRawURL(url, nil)
	require.NoError(t, err)
	assert.Equal(t, "https://raw.githubusercontent.com/devplaninc/devplan-cli/main/README.md", result)
}

func TestConvertToRawURL_TreeFormat(t *testing.T) {
	t.Parallel()
	url := "https://github.com/owner/repo/tree/v1.0.0/docs/guide.md"
	result, err := ConvertToRawURL(url, nil)
	require.NoError(t, err)
	assert.Equal(t, "https://raw.githubusercontent.com/owner/repo/v1.0.0/docs/guide.md", result)
}
