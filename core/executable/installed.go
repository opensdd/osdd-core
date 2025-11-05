package executable

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// detectInstalledIDEs returns a map of installed IDEs and their executable paths
func detectInstalledIDEs() (map[IDE]string, error) {
	detected, err := detectIDEs()
	if err != nil {
		return nil, err
	}
	for _, ideName := range []string{"codex", "claude"} {
		if p, err := exec.LookPath(ideName); err == nil {
			detected[IDE(ideName)] = p
		}
	}
	if cursorCLIPath, err := exec.LookPath("cursor-agent"); err == nil {
		detected[CursorCLI] = cursorCLIPath
	}

	return detected, nil
}

func detectIDEs() (map[IDE]string, error) {
	switch runtime.GOOS {
	case "darwin":
		return detectMacOSIDEs()
	case "linux":
		return detectLinuxIDEs()
	case "windows":
		return detectWindowsIDEs()
	default:
		return nil, nil
	}
}

// detectMacOSIDEs detects installed IDEs on macOS
func detectMacOSIDEs() (map[IDE]string, error) {
	result := make(map[IDE]string)

	detectMacJetbrains(result)
	detectMacCursor(result)
	detectMacWinsurf(result)

	return result, nil
}

func detectMacWinsurf(result map[IDE]string) {
	// Check for Windsurf
	windsurfPaths := []string{
		filepath.Join("/Applications", "Windsurf.app", "Contents", "MacOS", "Windsurf"),
		filepath.Join("/Applications", "Windsurf.app", "Contents", "MacOS", "Electron"),
		filepath.Join(os.Getenv("HOME"), "Applications", "Windsurf.app", "Contents", "MacOS", "Windsurf"),
		filepath.Join(os.Getenv("HOME"), "Applications", "Windsurf.app", "Contents", "MacOS", "Electron"),
	}
	for _, path := range windsurfPaths {
		if _, err := os.Stat(path); err == nil {
			result[Windsurf] = path
			break
		}
	}
}

func detectMacCursor(result map[IDE]string) {
	// Check for Cursor
	cursorPaths := []string{
		filepath.Join("/Applications", "Cursor.app", "Contents", "MacOS", "Cursor"),
		filepath.Join(os.Getenv("HOME"), "Applications", "Cursor.app", "Contents", "MacOS", "Cursor"),
	}
	for _, path := range cursorPaths {
		if _, err := os.Stat(path); err == nil {
			result[Cursor] = path
			break
		}
	}
}

func detectMacJetbrains(result map[IDE]string) {
	// Common installation directories on macOS
	directories := []string{
		"/Applications",
		filepath.Join(os.Getenv("HOME"), "Applications"),
	}

	// JetBrains IDEs
	jetbrainsApps := map[IDE][]string{
		PyCharm:  {"PyCharm.app", "PyCharm CE.app", "PyCharm Professional.app"},
		IntelliJ: {"IntelliJ IDEA.app", "IntelliJ IDEA CE.app", "IntelliJ IDEA Ultimate.app"},
		GoLand:   {"GoLand.app"},
		WebStorm: {"WebStorm.app"},
		PhpStorm: {"PhpStorm.app"},
		RubyMine: {"RubyMine.app"},
		CLion:    {"CLion.app"},
		Rider:    {"Rider.app"},
		DataGrip: {"DataGrip.app"},
		AppCode:  {"AppCode.app"},
	}

	// Check for JetBrains IDEs
	for ide, appNames := range jetbrainsApps {
		for _, dir := range directories {
			for _, appName := range appNames {
				appPath := filepath.Join(dir, appName)
				if _, err := os.Stat(appPath); err == nil {
					// For macOS, the executable is typically in Contents/MacOS
					exePath := filepath.Join(appPath, "Contents", "MacOS", appName[:len(appName)-4])
					if _, err := os.Stat(exePath); err == nil {
						result[ide] = exePath
						break
					}
				}
			}
		}
	}
}

// detectLinuxIDEs detects installed IDEs on Linux
func detectLinuxIDEs() (map[IDE]string, error) {
	result := make(map[IDE]string)

	// Common installation directories on Linux
	directories := []string{
		"/usr/bin",
		"/usr/local/bin",
		"/opt",
		"/snap/bin",
		filepath.Join(os.Getenv("HOME"), ".local", "bin"),
		filepath.Join(os.Getenv("HOME"), ".local", "share"),
	}
	detectLinuxJetbrains(directories, result)
	detectLinuxCursor(result)
	detectLinuxWinsurf(result)

	return result, nil
}

func detectLinuxWinsurf(result map[IDE]string) {
	// Check for Windsurf
	windsurfPaths := []string{
		"/usr/bin/windsurf",
		"/usr/local/bin/windsurf",
		"/opt/windsurf/windsurf",
		filepath.Join(os.Getenv("HOME"), ".local", "bin", "windsurf"),
	}
	for _, path := range windsurfPaths {
		if _, err := os.Stat(path); err == nil {
			result[Windsurf] = path
			break
		}
	}
}

func detectLinuxCursor(result map[IDE]string) {
	// Check for Cursor
	cursorPaths := []string{
		"/usr/bin/cursor",
		"/usr/local/bin/cursor",
		"/opt/cursor/cursor",
		filepath.Join(os.Getenv("HOME"), ".local", "bin", "cursor"),
	}
	for _, path := range cursorPaths {
		if _, err := os.Stat(path); err == nil {
			result[Cursor] = path
			break
		}
	}
}

func detectLinuxJetbrains(directories []string, result map[IDE]string) {
	// JetBrains IDEs
	jetbrainsApps := map[IDE][]string{
		PyCharm:  {"pycharm", "pycharm-community", "pycharm-professional"},
		IntelliJ: {"idea", "idea.sh", "intellij-idea-community", "intellij-idea-ultimate"},
		GoLand:   {"goland", "goland.sh"},
		WebStorm: {"webstorm", "webstorm.sh"},
		PhpStorm: {"phpstorm", "phpstorm.sh"},
		RubyMine: {"rubymine", "rubymine.sh"},
		CLion:    {"clion", "clion.sh"},
		Rider:    {"rider", "rider.sh"},
		DataGrip: {"datagrip", "datagrip.sh"},
		AppCode:  {"appcode", "appcode.sh"},
	}

	// Check for JetBrains IDEs in common directories
	for ide, exeNames := range jetbrainsApps {
		for _, dir := range directories {
			for _, exeName := range exeNames {
				exePath := filepath.Join(dir, exeName)
				if _, err := os.Stat(exePath); err == nil {
					result[ide] = exePath
					break
				}
			}
		}
	}

	// Check for JetBrains IDEs in /opt/jetbrains
	jetbrainsDir := "/opt/jetbrains"
	if _, err := os.Stat(jetbrainsDir); err == nil {
		entries, err := os.ReadDir(jetbrainsDir)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					dirName := entry.Name()
					binDir := filepath.Join(jetbrainsDir, dirName, "bin")
					if _, err := os.Stat(binDir); err == nil {
						binEntries, err := os.ReadDir(binDir)
						if err == nil {
							for _, binEntry := range binEntries {
								if !binEntry.IsDir() {
									name := binEntry.Name()
									if filepath.Ext(name) == ".sh" {
										exePath := filepath.Join(binDir, name)
										// Map to IDE based on name
										for ide, exeNames := range jetbrainsApps {
											for _, exeName := range exeNames {
												if name == exeName {
													result[ide] = exePath
													break
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
}

// detectWindowsIDEs detects installed IDEs on Windows
func detectWindowsIDEs() (map[IDE]string, error) {
	result := make(map[IDE]string)

	// Common installation directories on Windows
	programFiles := os.Getenv("ProgramFiles")
	programFilesX86 := os.Getenv("ProgramFiles(x86)")
	localAppData := os.Getenv("LOCALAPPDATA")
	appData := os.Getenv("APPDATA")

	directories := []string{
		programFiles,
		programFilesX86,
		filepath.Join(localAppData, "Programs"),
		filepath.Join(appData, "Programs"),
	}

	// JetBrains IDEs
	jetbrainsApps := map[IDE][]string{
		PyCharm:  {"JetBrains\\PyCharm*\\bin\\pycharm64.exe", "PyCharm*\\bin\\pycharm64.exe"},
		IntelliJ: {"JetBrains\\IntelliJ IDEA*\\bin\\idea64.exe", "IntelliJ IDEA*\\bin\\idea64.exe"},
		GoLand:   {"JetBrains\\GoLand*\\bin\\goland64.exe", "GoLand*\\bin\\goland64.exe"},
		WebStorm: {"JetBrains\\WebStorm*\\bin\\webstorm64.exe", "WebStorm*\\bin\\webstorm64.exe"},
		PhpStorm: {"JetBrains\\PhpStorm*\\bin\\phpstorm64.exe", "PhpStorm*\\bin\\phpstorm64.exe"},
		RubyMine: {"JetBrains\\RubyMine*\\bin\\rubymine64.exe", "RubyMine*\\bin\\rubymine64.exe"},
		CLion:    {"JetBrains\\CLion*\\bin\\clion64.exe", "CLion*\\bin\\clion64.exe"},
		Rider:    {"JetBrains\\Rider*\\bin\\rider64.exe", "Rider*\\bin\\rider64.exe"},
		DataGrip: {"JetBrains\\DataGrip*\\bin\\datagrip64.exe", "DataGrip*\\bin\\datagrip64.exe"},
		AppCode:  {"JetBrains\\AppCode*\\bin\\appcode64.exe", "AppCode*\\bin\\appcode64.exe"},
	}

	// Check for JetBrains IDEs
	for ide, patterns := range jetbrainsApps {
		for _, dir := range directories {
			for _, pattern := range patterns {
				matches, _ := filepath.Glob(filepath.Join(dir, pattern))
				if len(matches) > 0 {
					result[ide] = matches[0]
					break
				}
			}
		}
	}

	// Check for Cursor
	cursorPaths := []string{
		filepath.Join(programFiles, "Cursor", "Cursor.exe"),
		filepath.Join(programFilesX86, "Cursor", "Cursor.exe"),
		filepath.Join(localAppData, "Programs", "Cursor", "Cursor.exe"),
	}
	for _, path := range cursorPaths {
		if _, err := os.Stat(path); err == nil {
			result[Cursor] = path
			break
		}
	}

	// Check for Windsurf
	windsurfPaths := []string{
		filepath.Join(programFiles, "Windsurf", "Windsurf.exe"),
		filepath.Join(programFilesX86, "Windsurf", "Windsurf.exe"),
		filepath.Join(localAppData, "Programs", "Windsurf", "Windsurf.exe"),
	}
	for _, path := range windsurfPaths {
		if _, err := os.Stat(path); err == nil {
			result[Windsurf] = path
			break
		}
	}

	return result, nil
}
