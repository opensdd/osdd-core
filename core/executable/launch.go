package executable

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

type LaunchResult struct {
	// ToExecute is the command to execute in the terminal. If provided, then actual execution was skipped.
	ToExecute string
	Skipped   bool
}

type LaunchParams struct {
	IDE           string
	RepoPath      string
	Args          []string
	OutputCMDOnly bool
}

// LaunchIDE launches the specified IDE at the given repository path
func LaunchIDE(ctx context.Context, params LaunchParams) (LaunchResult, error) {
	ideType, err := asIDE(params.IDE)
	if err != nil {
		return LaunchResult{}, err
	}
	installed, err := detectInstalledIDEs()
	if err != nil {
		return LaunchResult{}, fmt.Errorf("failed to detect installed IDEs: %w", err)
	}
	return launchWithPath(ctx, installed[ideType], params)
}

func launchWithPath(ctx context.Context, idePath string, params LaunchParams) (LaunchResult, error) {
	if _, err := os.Stat(idePath); os.IsNotExist(err) {
		return LaunchResult{}, fmt.Errorf("IDE executable not found at %s", idePath)
	}

	if _, err := os.Stat(params.RepoPath); os.IsNotExist(err) {
		return LaunchResult{}, fmt.Errorf("repository path not found at %s", params.RepoPath)
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin", "linux", "windows":
		if isTerminalExecutable(idePath) {
			return launchInTerminal(ctx, idePath, params)
		}
		if params.OutputCMDOnly {
			return LaunchResult{ToExecute: fmt.Sprintf("%v %v", idePath, params.RepoPath)}, nil
		}
		cmd = exec.CommandContext(ctx, idePath, params.RepoPath)
	default:
		return LaunchResult{}, fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return LaunchResult{}, cmd.Start()
}

var executableAgents = []string{
	"claude",
	"cursor-agent",
	"codex",
}

func isTerminalExecutable(idePath string) bool {
	execName := strings.ToLower(filepath.Base(idePath))
	return slices.Contains(executableAgents, execName)
}

func quoteSingle(s string) string {
	// Escape any single quotes in the string by replacing ' with '\''
	escaped := strings.ReplaceAll(s, "\"", "\\\"")
	return "'" + escaped + "'"
}

// launchInTerminal launches CLI IDE in a new terminal session
func launchInTerminal(ctx context.Context, idePath string, params LaunchParams) (LaunchResult, error) {
	extraArgs := getExtraLaunchParams(idePath)
	allArgs := append(extraArgs, params.Args...)
	if params.OutputCMDOnly {
		extra := ""
		if len(allArgs) > 0 {
			for _, arg := range allArgs {
				extra += " "
				extra += strconv.Quote(arg)
			}
		}
		toExecute := fmt.Sprintf("cd '%s' && '%s'%v", params.RepoPath, idePath, extra)
		return LaunchResult{ToExecute: toExecute}, nil
	}
	//extra := ""
	//if len(allArgs) > 0 {
	//	extra = " " + strings.Join(allArgs, " ")
	//}
	//toExecute := fmt.Sprintf("cd '%s' && '%s'%v", params.RepoPath, idePath, extra)

	extra := ""
	if len(allArgs) > 0 {
		for _, arg := range allArgs {
			extra += " "
			extra += quoteSingle(arg)
		}
	}
	toExecute := fmt.Sprintf("cd '%s' && '%s'%v", params.RepoPath, idePath, extra)

	switch runtime.GOOS {
	case "darwin":
		script := fmt.Sprintf(`tell application "Terminal" to do script "%v"`, toExecute)
		fmt.Printf("Launching IDE in a new terminal session:\n  %v\n", script)
		cmd := exec.CommandContext(ctx, "osascript", "-e", script)
		return LaunchResult{}, cmd.Start()
	default:
		fmt.Printf("Only MacOS is supported for direct execution of CLIs. Please start manually:\n  %v\n", toExecute)
		return LaunchResult{Skipped: true}, nil
	}
}

func getExtraLaunchParams(idePath string) []string {
	if strings.Contains(strings.ToLower(idePath), "cursor-agent") {
		return []string{"-f"}
	}
	return nil
}
