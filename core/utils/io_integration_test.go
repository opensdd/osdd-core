package utils

import (
	"context"
	"testing"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ex(cmd string, args ...string) *osdd.Exec {
	return osdd.Exec_builder{
		Cmd:  cmd,
		Args: args,
	}.Build()
}

func TestExecuteCommand_Integration_Success(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	out, err := ExecuteCommand(context.Background(), ex("echo", "integration ok"))
	require.NoError(t, err)
	assert.Equal(t, "integration ok\n", out)
}

func TestExecuteCommand_Integration_Error(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	_, err := ExecuteCommand(context.Background(), ex("exit", "1"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "command execution failed")
}
