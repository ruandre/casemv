package cli_test

import (
	"bytes"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/ruandre/casemv/internal/cli"
)

var allConventions = []string{"camel", "kebab", "lower", "pascal", "screaming", "sentence", "snake", "title", "train"}

// completion is what the hidden __complete command returns to the shell script.
type completion struct {
	values    []string
	directive cobra.ShellCompDirective
}

// complete asks for completions of the last word in line, from a working
// directory holding directories docs, src, and .git, and file notes.txt.
func complete(t *testing.T, line ...string) completion {
	t.Helper()
	t.Chdir(testTreeForCompletion(t))
	got := runCLI(t, "", append([]string{"__complete"}, line...)...)
	if got.err != nil {
		t.Fatalf("__complete %q: %v (output %q)", line, got.err, got.out)
	}
	return parseCompletion(t, got.out)
}

// testTreeForCompletion holds directories whose names would break cobra's
// completion output, which separates candidates with newlines and
// descriptions with tabs; none may be offered.
func testTreeForCompletion(t *testing.T) string {
	t.Helper()
	return makeTree(t, "docs/", "src/", ".git/", "notes.txt", "tab\tname/", "new\nline/")
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

func TestCompletion(t *testing.T) {
	const (
		noFiles = cobra.ShellCompDirectiveNoFileComp
		dirs    = cobra.ShellCompDirectiveFilterDirs
	)
	withDefault := "convention: kebab\n"
	tests := []struct {
		name   string
		config string
		env    map[string]string
		line   []string
		want   completion
	}{
		{"conventions first", "", nil, []string{""}, completion{allConventions, noFiles}},
		{"conventions by prefix", "", nil, []string{"s"}, completion{[]string{"screaming", "sentence", "snake"}, noFiles}},
		{"no directory without a default", "", nil, []string{"d"}, completion{nil, noFiles}},
		{"target after a convention", "", nil, []string{"kebab", ""}, completion{nil, dirs}},
		{"nothing after the target", "", nil, []string{"kebab", "docs", ""}, completion{nil, noFiles}},

		{"default adds directories", withDefault, nil, []string{""}, completion{append(slices.Clone(allConventions), "docs/", "src/"), noFiles}},
		{"default, shared prefix", withDefault, nil, []string{"s"}, completion{[]string{"screaming", "sentence", "snake", "src/"}, noFiles}},
		{"default, directory prefix uses the shell", withDefault, nil, []string{"d"}, completion{nil, dirs}},
		{"default, path uses the shell", withDefault, nil, []string{"./"}, completion{nil, dirs}},
		{"default, hidden prefix uses the shell", withDefault, nil, []string{"."}, completion{nil, dirs}},
		{"default, target after a convention", withDefault, nil, []string{"snake", ""}, completion{nil, dirs}},
		{"default, nothing after a target", withDefault, nil, []string{"docs", ""}, completion{nil, noFiles}},
		{"default from environment", "", map[string]string{"CASEMV_CONVENTION": "kebab"}, []string{"s"}, completion{[]string{"screaming", "sentence", "snake", "src/"}, noFiles}},
		{"default from flag", "", nil, []string{"-c", "kebab", "s"}, completion{[]string{"screaming", "sentence", "snake", "src/"}, noFiles}},

		{"text, no directories", withDefault, nil, []string{"--text", "s"}, completion{[]string{"screaming", "sentence", "snake"}, noFiles}},
		{"text, nothing after a convention", withDefault, nil, []string{"--text", "kebab", ""}, completion{nil, noFiles}},

		{"broken config is no default", "bogus: true\n", nil, []string{"s"}, completion{[]string{"screaming", "sentence", "snake"}, noFiles}},
		{"invalid default is no default", "convention: kebabb\n", nil, []string{"d"}, completion{nil, noFiles}},
		{"real config keys do not break it", "convention: kebab\nlower-ext: true\nrecursive: true\n", nil, []string{"k"}, completion{[]string{"kebab"}, noFiles}},

		{"convention flag value", "", nil, []string{"--convention", ""}, completion{allConventions, noFiles}},
		{"convention flag value by prefix", "", nil, []string{"-c", "k"}, completion{[]string{"kebab"}, noFiles}},
		{"config flag value", "", nil, []string{"--config", ""}, completion{[]string{"yaml", "yml"}, cobra.ShellCompDirectiveFilterFileExt}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			if tc.config != "" {
				withUserConfig(t, tc.config)
			}

			got := complete(t, tc.line...)

			if !slices.Equal(got.values, tc.want.values) || got.directive != tc.want.directive {
				t.Errorf("__complete %q = %q %v, want %q %v", tc.line, got.values, got.directive, tc.want.values, tc.want.directive)
			}
		})
	}
}

func TestCompletionScriptsWithAConfigFile(t *testing.T) {
	withUserConfig(t, "convention: kebab\nlower-ext: true\n")
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			got := runCLI(t, "", "completion", shell)
			if got.err != nil {
				t.Fatalf("completion %s: %v (output %q)", shell, got.err, got.out)
			}
			if !strings.Contains(got.out, "__complete") {
				t.Errorf("completion %s printed no script: %q", shell, got.out)
			}
		})
	}
}

func TestFishCompletionScriptHonoursFilters(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    []string
		without []string
	}{
		{"default", nil, []string{"__fish_complete_directories", "__casemv_complete_extensions", "__complete "}, []string{"__completeNoDesc", "not supported"}},
		{"no descriptions", []string{"--no-descriptions"}, []string{"__completeNoDesc "}, []string{"__complete "}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := runCLI(t, "", append([]string{"completion", "fish"}, tc.args...)...)
			if got.err != nil {
				t.Fatalf("completion fish: %v", got.err)
			}
			assertScript(t, got.out, tc.want, tc.without)
		})
	}
}

func assertScript(t *testing.T, script string, want, without []string) {
	t.Helper()
	for _, fragment := range want {
		if !strings.Contains(script, fragment) {
			t.Errorf("script is missing %q", fragment)
		}
	}
	for _, fragment := range without {
		if strings.Contains(script, fragment) {
			t.Errorf("script contains %q", fragment)
		}
	}
}

// Only fish gets casemv's own script; the others must stay exactly cobra's.
func TestOtherShellScriptsAreCobras(t *testing.T) {
	generators := map[string]func(*cobra.Command, *bytes.Buffer) error{
		"bash":       func(c *cobra.Command, b *bytes.Buffer) error { return c.GenBashCompletionV2(b, true) },
		"zsh":        func(c *cobra.Command, b *bytes.Buffer) error { return c.GenZshCompletion(b) },
		"powershell": func(c *cobra.Command, b *bytes.Buffer) error { return c.GenPowerShellCompletionWithDesc(b) },
	}
	for shell, generate := range generators {
		t.Run(shell, func(t *testing.T) {
			var want bytes.Buffer
			if err := generate(cli.NewRootCmd(), &want); err != nil {
				t.Fatal(err)
			}
			got := runCLI(t, "", "completion", shell)
			if got.err != nil {
				t.Fatalf("completion %s: %v", shell, got.err)
			}
			if got.out != want.String() {
				t.Errorf("completion %s differs from cobra's generator", shell)
			}
		})
	}
}

func TestFirstArgumentNeverOffersSubcommands(t *testing.T) {
	withUserConfig(t, "convention: kebab\n")
	got := complete(t, "c")
	for _, unwanted := range []string{"completion", "help"} {
		if slices.Contains(got.values, unwanted) {
			t.Errorf("completions %q offer subcommand %q", got.values, unwanted)
		}
	}
}
