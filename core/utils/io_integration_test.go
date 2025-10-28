//go:build integration
// +build integration

package utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteCommand_Integration_Success(t *testing.T) {
	out, err := ExecuteCommand(context.Background(), "echo 'integration ok'")
	require.NoError(t, err)
	assert.Equal(t, "integration ok\n", out)
}

func TestExecuteCommand_Integration_Error(t *testing.T) {
	_, err := ExecuteCommand(context.Background(), "exit 1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "command execution failed")
}
