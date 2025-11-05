package executable

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectInstalledIDEs(t *testing.T) {
	// This is a basic test to ensure the function runs without errors
	// Actual detection depends on the system where the test is run
	ides, err := detectInstalledIDEs()
	assert.NoError(t, err)

	// The result should be a map (might be empty if no IDEs are installed)
	assert.IsType(t, map[IDE]string{}, ides)
}

func TestDetectMockIDEs(t *testing.T) {
	// Skip if not on a supported OS
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		t.Skip("Skipping test on unsupported OS")
	}

	// Create a temporary directory structure to simulate IDE installations
	tempDir, err := os.MkdirTemp("", "ide-installed-test")
	assert.NoError(t, err)
	defer func(path string) {
		_ = os.RemoveAll(path)
	}(tempDir)

	var mockPaths []string

	switch runtime.GOOS {
	case "darwin":
		// Create mock macOS IDE directories
		appDir := filepath.Join(tempDir, "Applications")
		assert.NoError(t, os.MkdirAll(appDir, 0755))

		// Create PyCharm mock
		pycharmPath := filepath.Join(appDir, "PyCharm.app", "Contents", "MacOS")
		assert.NoError(t, os.MkdirAll(pycharmPath, 0755))
		pycharmExe := filepath.Join(pycharmPath, "PyCharm")
		createMockExecutable(t, pycharmExe)
		mockPaths = append(mockPaths, pycharmExe)

		// Create Cursor mock
		cursorPath := filepath.Join(appDir, "Cursor.app", "Contents", "MacOS")
		assert.NoError(t, os.MkdirAll(cursorPath, 0755))
		cursorExe := filepath.Join(cursorPath, "Cursor")
		createMockExecutable(t, cursorExe)
		mockPaths = append(mockPaths, cursorExe)

	case "linux":
		// Create mock Linux IDE directories
		binDir := filepath.Join(tempDir, "usr", "bin")
		assert.NoError(t, os.MkdirAll(binDir, 0755))

		// Create PyCharm mock
		pycharmExe := filepath.Join(binDir, "pycharm")
		createMockExecutable(t, pycharmExe)
		mockPaths = append(mockPaths, pycharmExe)

		// Create Cursor mock
		cursorExe := filepath.Join(binDir, "cursor")
		createMockExecutable(t, cursorExe)
		mockPaths = append(mockPaths, cursorExe)

	case "windows":
		// Create mock Windows IDE directories
		programFiles := filepath.Join(tempDir, "Program Files")
		assert.NoError(t, os.MkdirAll(programFiles, 0755))

		// Create PyCharm mock
		pycharmPath := filepath.Join(programFiles, "JetBrains", "PyCharm", "bin")
		assert.NoError(t, os.MkdirAll(pycharmPath, 0755))
		pycharmExe := filepath.Join(pycharmPath, "pycharm64.exe")
		createMockExecutable(t, pycharmExe)
		mockPaths = append(mockPaths, pycharmExe)

		// Create Cursor mock
		cursorPath := filepath.Join(programFiles, "Cursor")
		assert.NoError(t, os.MkdirAll(cursorPath, 0755))
		cursorExe := filepath.Join(cursorPath, "Cursor.exe")
		createMockExecutable(t, cursorExe)
		mockPaths = append(mockPaths, cursorExe)
	}

	// TODO: Implement a way to test the detection with the mock directories
	// This would require modifying the detection functions to accept a base directory
	// or using environment variables to override the default paths
	// For now, we just verify that the mock files were created correctly
	for _, path := range mockPaths {
		_, err := os.Stat(path)
		assert.NoError(t, err, "Mock executable should exist: %s", path)
	}
}

// Helper function to create a mock executable file
func createMockExecutable(t *testing.T, path string) {
	f, err := os.Create(path)
	assert.NoError(t, err)
	defer func(f *os.File) {
		_ = f.Close()
	}(f)

	// Make the file executable
	if runtime.GOOS != "windows" {
		assert.NoError(t, os.Chmod(path, 0755))
	}
}
