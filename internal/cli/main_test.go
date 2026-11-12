package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain keeps the host's CASEMV_* variables and user config out of every
// test. Each run sees an empty user config directory unless the test calls
// withUserConfig.
func TestMain(m *testing.M) {
	os.Exit(runIsolated(m))
}

func runIsolated(m *testing.M) int {
	home, err := os.MkdirTemp("", "casemv-cli-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(home) //nolint:errcheck // a leftover temporary directory fails no test

	for _, entry := range os.Environ() {
		if name, _, _ := strings.Cut(entry, "="); strings.HasPrefix(name, "CASEMV_") {
			if err := os.Unsetenv(name); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
		}
	}
	for key, value := range userConfigEnv(home) {
		if err := os.Setenv(key, value); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	return m.Run()
}

// userConfigEnv makes os.UserConfigDir resolve under home on every OS.
func userConfigEnv(home string) map[string]string {
	return map[string]string{
		"HOME":            home, // macOS: $HOME/Library/Application Support.
		"XDG_CONFIG_HOME": filepath.Join(home, ".config"),
		"AppData":         filepath.Join(home, "AppData"), // Windows.
	}
}

// withUserConfig points the user config directory at a new one whose
// casemv.yaml holds content, and returns that directory.
func withUserConfig(t *testing.T, content string) string {
	t.Helper()
	for key, value := range userConfigEnv(t.TempDir()) {
		t.Setenv(key, value)
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "casemv"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "casemv", "casemv.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}
