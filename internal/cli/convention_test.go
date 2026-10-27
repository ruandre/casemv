package cli_test

import (
	"strings"
	"testing"
)

func TestDefaultConventionSources(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		env    map[string]string
		config string
	}{
		{"long flag", []string{"--convention", "kebab"}, nil, ""},
		{"short flag", []string{"-c", "kebab"}, nil, ""},
		{"environment", nil, map[string]string{"CASEMV_CONVENTION": "kebab"}, ""},
		{"config file", nil, nil, "convention: kebab\n"},
		{"name is case-insensitive", nil, nil, "convention: Kebab\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := makeTree(t, "My File.txt")
			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			args := append([]string{root}, tc.args...)
			if tc.config != "" {
				args = append(args, "--config", writeConfig(t, tc.config))
			}

			if got := runCLI(t, "", args...); got.err != nil {
				t.Fatalf("run: %v (output %q)", got.err, got.out)
			}
			assertTree(t, root, "my-file.txt")
		})
	}
}

func TestDefaultConventionWithoutArguments(t *testing.T) {
	root := makeTree(t, "My File.txt")
	t.Chdir(root)
	t.Setenv("CASEMV_CONVENTION", "snake")

	if got := runCLI(t, ""); got.err != nil {
		t.Fatalf("run: %v (output %q)", got.err, got.out)
	}
	assertTree(t, root, "my_file.txt")
}

// The configuration this feature exists for: one config, then a bare casemv.
func TestConfigFileSetsConventionAndLowerExtension(t *testing.T) {
	root := makeTree(t, "My File.TXT", "Sub Dir/Inner File.JSON")
	config := writeConfig(t, "convention: kebab\nlower-ext: true\nrecursive: true\n")

	if got := runCLI(t, "", root, "--config", config); got.err != nil {
		t.Fatalf("run: %v (output %q)", got.err, got.out)
	}
	assertTree(t, root, "my-file.txt", "Sub Dir/", "Sub Dir/inner-file.json")
}

func TestConventionArgumentOverridesTheDefault(t *testing.T) {
	tests := []struct {
		name string
		args func(root string) []string
	}{
		{"with target", func(root string) []string { return []string{"snake", root} }},
		{"alone", func(string) []string { return []string{"snake"} }},
		{"over the flag", func(root string) []string { return []string{"snake", root, "-c", "kebab"} }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := makeTree(t, "My File.txt")
			t.Chdir(root)
			t.Setenv("CASEMV_CONVENTION", "kebab")

			if got := runCLI(t, "", tc.args(root)...); got.err != nil {
				t.Fatalf("run: %v (output %q)", got.err, got.out)
			}
			assertTree(t, root, "my_file.txt")
		})
	}
}

// A lone argument naming a convention is the convention, so a directory with
// such a name is reached through a path.
func TestDirectoryNamedLikeAConventionNeedsAPath(t *testing.T) {
	root := makeTree(t, "snake/My File.txt", "Top File.txt")
	t.Chdir(root)
	t.Setenv("CASEMV_CONVENTION", "kebab")

	if got := runCLI(t, "", "./snake"); got.err != nil {
		t.Fatalf("run: %v (output %q)", got.err, got.out)
	}
	assertTree(t, root, "snake/", "snake/my-file.txt", "Top File.txt")
}

func TestMissingConventionIsAnError(t *testing.T) {
	root := makeTree(t, "My File.txt")
	t.Chdir(root)

	got := runCLI(t, "")

	if got.err == nil || !strings.Contains(got.err.Error(), "missing naming convention") {
		t.Errorf("error = %v, want a missing convention error", got.err)
	}
	assertTree(t, root, "My File.txt")
}

func TestLoneUnknownArgumentWithoutDefaultIsAnError(t *testing.T) {
	got := runCLI(t, "", "kebab-case")
	if got.err == nil || !strings.Contains(got.err.Error(), `unknown naming convention "kebab-case"`) {
		t.Errorf("error = %v, want an unknown convention error", got.err)
	}
}

// An invalid default fails even when an argument overrides it, as an invalid
// boolean does: a broken config should not lie in wait.
func TestInvalidDefaultConventionIsAnError(t *testing.T) {
	tests := []struct {
		name   string
		env    map[string]string
		config string
	}{
		{"environment", map[string]string{"CASEMV_CONVENTION": "kebabb"}, ""},
		{"config file", nil, "convention: kebabb\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := makeTree(t, "My File.txt")
			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			args := []string{"snake", root}
			if tc.config != "" {
				args = append(args, "--config", writeConfig(t, tc.config))
			}

			got := runCLI(t, "", args...)

			if got.err == nil || !strings.Contains(got.err.Error(), `unknown naming convention "kebabb"`) {
				t.Errorf("error = %v, want an unknown convention error", got.err)
			}
			assertTree(t, root, "My File.txt")
		})
	}
}

func TestTextModeUsesTheDefaultConvention(t *testing.T) {
	t.Setenv("CASEMV_CONVENTION", "kebab")

	got := runCLI(t, "My File\n", "--text")

	if got.err != nil {
		t.Fatalf("run: %v", got.err)
	}
	if got.out != "my-file\n" {
		t.Errorf("output = %q, want %q", got.out, "my-file\n")
	}
}

func TestTextModeWithDefaultRejectsATarget(t *testing.T) {
	t.Setenv("CASEMV_CONVENTION", "kebab")

	got := runCLI(t, "My File\n", "./somewhere", "--text")

	if got.err == nil || !strings.Contains(got.err.Error(), "--text does not accept a target argument") {
		t.Errorf("error = %v, want a target rejection", got.err)
	}
}
