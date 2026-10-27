package cli_test

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

var allConventions = []string{"camel", "kebab", "lower", "pascal", "screaming", "sentence", "snake", "title", "train"}

// completion is what the hidden __complete command returns to the shell script.
type completion struct {
	values    []string
	directive cobra.ShellCompDirective
}

// complete asks for completions of the last word in line, from a working
// directory holding directories docs, src, and .git, and file notes.txt.
func complete(t *testing.T, configHome string, line ...string) completion {
	t.Helper()
	t.Chdir(testTreeForCompletion(t))
	got := runCLIWithConfigHome(t, configHome, "", append([]string{"__complete"}, line...)...)
	if got.err != nil {
		t.Fatalf("__complete %q: %v (output %q)", line, got.err, got.out)
	}
	return parseCompletion(t, got.out)
}

func testTreeForCompletion(t *testing.T) string {
	t.Helper()
	return makeTree(t, "docs/", "src/", ".git/", "notes.txt")
}

func parseCompletion(t *testing.T, out string) completion {
	t.Helper()
	var c completion
	for line := range strings.Lines(out) {
		line = strings.TrimSuffix(line, "\n")
		if directive, ok := strings.CutPrefix(line, ":"); ok {
			n, err := strconv.Atoi(directive)
			if err != nil {
				t.Fatalf("bad directive line %q in %q", line, out)
			}
			c.directive = cobra.ShellCompDirective(n)
			return c
		}
		c.values = append(c.values, line)
	}
	t.Fatalf("no directive line in completion output %q", out)
	return c
}

// configHomeWith returns an XDG_CONFIG_HOME whose casemv config holds content.
func configHomeWith(t *testing.T, content string) string {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, "casemv")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "casemv.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestCompletion(t *testing.T) {
	const (
		noFiles = cobra.ShellCompDirectiveNoFileComp
		dirs    = cobra.ShellCompDirectiveFilterDirs
	)
	tests := []struct {
		name   string
		config string
		env    map[string]string
		line   []string
		want   completion
	}{
		{"conventions first", "", nil, []string{""}, completion{allConventions, noFiles}},
		{"conventions by prefix", "", nil, []string{"s"}, completion{[]string{"screaming", "sentence", "snake"}, noFiles}},
		{"no directories for the convention", "", nil, []string{"d"}, completion{nil, noFiles}},
		{"target after a convention", "", nil, []string{"kebab", ""}, completion{nil, dirs}},
		{"nothing after the target", "", nil, []string{"kebab", "docs", ""}, completion{nil, noFiles}},

		{"text, no directories", "", nil, []string{"--text", "s"}, completion{[]string{"screaming", "sentence", "snake"}, noFiles}},
		{"text, nothing after a convention", "", nil, []string{"--text", "kebab", ""}, completion{nil, noFiles}},

		{"broken config still completes", "bogus: true\n", nil, []string{"s"}, completion{[]string{"screaming", "sentence", "snake"}, noFiles}},
		{"real config keys do not break it", "recursive: true\ndir: true\n", nil, []string{"k"}, completion{[]string{"kebab"}, noFiles}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			configHome := t.TempDir()
			if tc.config != "" {
				configHome = configHomeWith(t, tc.config)
			}

			got := complete(t, configHome, tc.line...)

			if !slices.Equal(got.values, tc.want.values) || got.directive != tc.want.directive {
				t.Errorf("__complete %q = %q %v, want %q %v", tc.line, got.values, got.directive, tc.want.values, tc.want.directive)
			}
		})
	}
}

func TestCompletionScriptsWithAConfigFile(t *testing.T) {
	configHome := configHomeWith(t, "recursive: true\ndir: true\n")
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			got := runCLIWithConfigHome(t, configHome, "", "completion", shell)
			if got.err != nil {
				t.Fatalf("completion %s: %v (output %q)", shell, got.err, got.out)
			}
			if !strings.Contains(got.out, "__complete") {
				t.Errorf("completion %s printed no script: %q", shell, got.out)
			}
		})
	}
}
