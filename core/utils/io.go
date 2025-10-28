package utils

import (
	"context"
	"fmt"
	"os/exec"
)

// ExecuteCommand runs the provided shell command and returns its combined stdout/stderr output as string.
func ExecuteCommand(ctx context.Context, cmd string) (string, error) {
	if cmd == "" {
		return "", fmt.Errorf("command cannot be empty")
	}

	command := exec.CommandContext(ctx, "sh", "-c", cmd)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("command execution failed: %w (output: %s)", err, string(output))
	}

	return string(output), nil
}
