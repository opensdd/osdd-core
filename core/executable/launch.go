package executable

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// LaunchIDE launches the specified IDE at the given repository path
func LaunchIDE(ide string, repoPath string, args []string) (bool, error) {
	ideType, err := asIDE(ide)
	if err != nil {
		return false, err
	}
	installed, err := detectInstalledIDEs()
	if err != nil {
		return false, fmt.Errorf("failed to detect installed IDEs: %w", err)
	}
	return launchWithPath(installed[ideType], repoPath, args)
}

func launchWithPath(idePath string, repoPath string, args []string) (bool, error) {
	if _, err := os.Stat(idePath); os.IsNotExist(err) {
		return false, fmt.Errorf("IDE executable not found at %s", idePath)
	}

	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		return false, fmt.Errorf("repository path not found at %s", repoPath)
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin", "linux", "windows":
		if isTerminalExecutable(idePath) {
			return launchInTerminal(idePath, repoPath, args)
		}
		cmd = exec.Command(idePath, repoPath)
	default:
		return false, fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return true, cmd.Start()
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

// launchInTerminal launches CLI IDE in a new terminal session
func launchInTerminal(idePath string, repoPath string, args []string) (bool, error) {
	switch runtime.GOOS {
	case "darwin":
		extraArgs := getExtraLaunchParams(idePath)
		allArgs := append(extraArgs, args...)
		extra := ""
		if len(allArgs) > 0 {
			extra = " " + strings.Join(allArgs, " ")
		}
		script := fmt.Sprintf(
			`tell application "Terminal" to do script "cd '%s' && '%s'%v"`, repoPath, idePath, extra)
		cmd := exec.Command("osascript", "-e", script)
		err := cmd.Start()
		return true, err
	default:
		fmt.Printf("Only MacOS is supported for direct execution of CLIs. Please start manually:\n  cd \"%s\" && %v\n", repoPath, idePath)
		return false, nil
	}
}

func getExtraLaunchParams(idePath string) []string {
	if strings.Contains(strings.ToLower(idePath), "cursor-agent") {
		return []string{"-f"}
	}
	return nil
}
