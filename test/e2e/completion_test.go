//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const bashCompletionLibrary = "/usr/share/bash-completion/bash_completion"

// bashComplete drives the generated bash script the way readline would, and
// returns COMPREPLY for the last word of line.
const bashComplete = `
source "$1" || exit 1
source <(casemv completion bash) || exit 1
shift
COMP_WORDS=("$@")
COMP_CWORD=$((${#COMP_WORDS[@]} - 1))
COMP_LINE="$*"
COMP_POINT=${#COMP_LINE}
__start_casemv
printf '%s\n' "${COMPREPLY[@]}"
`

// fishComplete asks fish itself what it would offer for line.
const fishComplete = `casemv completion fish | source; complete -C "$argv[1]"`

func TestBashCompletion(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not installed")
	}
	if _, err := os.Stat(bashCompletionLibrary); err != nil {
		t.Skip("bash-completion not installed")
	}
	tests := []struct {
		name   string
		config string
		words  []string
		want   []string
		reject []string
	}{
		{"conventions", "", []string{"casemv", "s"}, []string{"screaming", "sentence", "snake"}, []string{"src"}},
		{"target after a convention", "", []string{"casemv", "kebab", ""}, []string{"docs", "src"}, []string{"notes.txt"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"-c", bashComplete, "bash", bashCompletionLibrary}, tc.words...)
			got := runShell(t, bash, tc.config, args...)
			assertCompletions(t, got, tc.want, tc.reject)
		})
	}
}

func TestFishCompletion(t *testing.T) {
	fish, err := exec.LookPath("fish")
	if err != nil {
		t.Skip("fish not installed")
	}
	tests := []struct {
		name   string
		config string
		line   string
		want   []string
		reject []string
	}{
		{"conventions", "", "casemv s", []string{"screaming", "sentence", "snake"}, []string{"src/"}},
		// cobra's fish script lacks directory filtering, so files are offered too.
		{"target after a convention", "", "casemv kebab ", []string{"docs/", "src/"}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := runShell(t, fish, tc.config, "--no-config", "-c", fishComplete, tc.line)
			assertCompletions(t, got, tc.want, tc.reject)
		})
	}
}

// runShell runs a shell in a tree holding docs, src, "My Docs", and notes.txt,
// with casemv first on PATH and config, when non-empty, as the user config.
func runShell(t *testing.T, shell, config string, args ...string) []string {
	t.Helper()
	root := makeTree(t, "docs/", "src/", "My Docs/", "notes.txt")
	configHome := t.TempDir()
	if config != "" {
		mustMkdir(t, filepath.Join(configHome, "casemv"))
		if err := os.WriteFile(filepath.Join(configHome, "casemv", "casemv.yaml"), []byte(config), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	cmd := exec.CommandContext(t.Context(), shell, args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"XDG_CONFIG_HOME="+configHome,
		"PATH="+filepath.Dir(binary)+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s %q: %v", shell, args, err)
	}
	var values []string
	for line := range strings.Lines(string(out)) {
		value, _, _ := strings.Cut(strings.TrimSuffix(line, "\n"), "\t")
		values = append(values, value)
	}
	return values
}

func assertCompletions(t *testing.T, got, want, reject []string) {
	t.Helper()
	for _, value := range want {
		if !slices.Contains(got, value) {
			t.Errorf("completions %q are missing %q", got, value)
		}
	}
	for _, value := range reject {
		if slices.Contains(got, value) {
			t.Errorf("completions %q include %q", got, value)
		}
	}
}
