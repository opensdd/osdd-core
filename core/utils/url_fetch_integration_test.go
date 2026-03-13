package utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchURL_Integration_RealHTTPS(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	body, err := FetchURL(context.Background(), "https://example.com")
	require.NoError(t, err)
	assert.NotEmpty(t, body)
	assert.Contains(t, string(body), "Example Domain")
}

func TestFetchURL_Integration_RealNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	_, err := FetchURL(context.Background(), "https://httpbin.org/status/404")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "returned status 404")
}

func TestFetchURL_Integration_BinaryDownload(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	// Fetch a small known binary file (1x1 PNG pixel via httpbin).
	body, err := FetchURL(context.Background(), "https://httpbin.org/image/png")
	require.NoError(t, err)
	assert.NotEmpty(t, body)
	// PNG files start with the magic bytes 0x89 'P' 'N' 'G'.
	require.GreaterOrEqual(t, len(body), 4)
	assert.Equal(t, byte(0x89), body[0])
	assert.Equal(t, byte('P'), body[1])
	assert.Equal(t, byte('N'), body[2])
	assert.Equal(t, byte('G'), body[3])
}
