package utils

import (
	"context"
	"fmt"
	"os/exec"
	"slices"

	"github.com/opensdd/osdd-api/clients/go/osdd"
)

var AllowedCommands []string

// ExecuteCommand runs the provided shell command and returns its combined stdout/stderr output as string.
func ExecuteCommand(ctx context.Context, execConfig *osdd.Exec) (string, error) {
	cmd := execConfig.GetCmd()
	if cmd == "" {
		return "", fmt.Errorf("command cannot be empty")
	}
	if len(AllowedCommands) > 0 {
		if !slices.Contains(AllowedCommands, cmd) {
			return "", fmt.Errorf("command [%v] is not allowed", cmd)
		}
	}

	command := exec.CommandContext(ctx, cmd, execConfig.GetArgs()...)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("command execution failed: %w (output: %s)", err, string(output))
	}

	return string(output), nil
}
