package cli_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ruandre/casemv/internal/cli"
	"github.com/ruandre/casemv/internal/rename"
	"github.com/ruandre/casemv/internal/testtree"
)

// result captures everything a command run produces.
type result struct {
	out string
	err error
}

// runCLI executes the root command in process. TestMain keeps the host's
// config and environment out of it.
func runCLI(t *testing.T, stdin string, args ...string) result {
	t.Helper()
	var out bytes.Buffer
	cmd := cli.NewRootCmd()
	cmd.SetArgs(args)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(stdin))
	err := cmd.ExecuteContext(t.Context())
	return result{out: out.String(), err: err}
}

func makeTree(t *testing.T, spec ...string) string {
	t.Helper()
	return testtree.Make(t, spec...)
}

func assertTree(t *testing.T, root string, want ...string) {
	t.Helper()
	testtree.Assert(t, root, want...)
}

func TestRenameEndToEnd(t *testing.T) {
	root := makeTree(t, "1. Something/2. Nested Dir/My File.TXT")
	got := runCLI(t, "", "kebab", root, "--recursive", "--dir")

	if got.err != nil {
		t.Fatalf("run: %v (output %q)", got.err, got.out)
	}
	assertTree(t, root, "1-something/", "1-something/2-nested-dir/", "1-something/2-nested-dir/my-file.TXT")
	if !strings.Contains(got.out, "3 entries renamed in ") {
		t.Errorf("output %q is missing the summary line", got.out)
	}
}

func TestTargetDefaultsToTheWorkingDirectory(t *testing.T) {
	root := makeTree(t, "My File.txt")
	t.Chdir(root)

	if got := runCLI(t, "", "snake"); got.err != nil {
		t.Fatalf("run: %v (output %q)", got.err, got.out)
	}
	assertTree(t, root, "my_file.txt")
}

// An unset shell variable, as in casemv kebab "$DIR", must not rename the
// working directory.
func TestEmptyArgumentIsAnError(t *testing.T) {
	tests := map[string][]string{
		"empty target":                     {"kebab", ""},
		"empty target with a default":      {"--convention", "kebab", ""},
		"empty target in text mode":        {"kebab", "", "--text"},
		"empty convention before a target": {"", "."},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			root := makeTree(t, "My File.txt")
			t.Chdir(root)

			got := runCLI(t, "", args...)
			if got.err == nil || !strings.Contains(got.err.Error(), "empty argument") {
				t.Errorf("error = %v, want an empty argument error (output %q)", got.err, got.out)
			}
			assertTree(t, root, "My File.txt")
		})
	}
}

func TestDryRunLeavesTheTreeUntouched(t *testing.T) {
	root := makeTree(t, "My File.txt")
	got := runCLI(t, "", "kebab", root, "--dry-run")

	if got.err != nil {
		t.Fatalf("run: %v", got.err)
	}
	assertTree(t, root, "My File.txt")
	for _, want := range []string{"my-file.txt", "dry run: 1 entry would be renamed"} {
		if !strings.Contains(got.out, want) {
			t.Errorf("output %q is missing %q", got.out, want)
		}
	}
}

func TestQuietSuppressesOutput(t *testing.T) {
	root := makeTree(t, "My File.txt")
	got := runCLI(t, "", "kebab", root, "--quiet")

	if got.err != nil {
		t.Fatalf("run: %v", got.err)
	}
	if got.out != "" {
		t.Errorf("output = %q, want empty", got.out)
	}
	assertTree(t, root, "my-file.txt")
}

func TestTextModePreservesBlankLines(t *testing.T) {
	got := runCLI(t, "My File Name\n1. Something\n\nStill Converted\n", "kebab", "--text")

	if got.err != nil {
		t.Fatalf("run: %v", got.err)
	}
	if want := "my-file-name\n1-something\n\nstill-converted\n"; got.out != want {
		t.Errorf("output = %q, want %q", got.out, want)
	}
}

func TestTextModeAppliesNoExtensionRules(t *testing.T) {
	got := runCLI(t, "archive.tar.gz\n", "kebab", "--text")

	if want := "archive-tar-gz\n"; got.out != want {
		t.Errorf("output = %q, want %q", got.out, want)
	}
}

func TestCollisionIsReportedAndNothingIsRenamed(t *testing.T) {
	root := makeTree(t, "My File.txt", "my_file.txt")
	got := runCLI(t, "", "kebab", root)

	if _, ok := errors.AsType[*rename.CollisionError](got.err); !ok {
		t.Fatalf("error = %v, want *rename.CollisionError", got.err)
	}
	assertTree(t, root, "My File.txt", "my_file.txt")
}

func TestArgumentErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"no convention", nil},
		{"unknown convention", []string{"kebab-case", "."}},
		{"too many arguments", []string{"kebab", ".", "extra"}},
		{"invalid exclude pattern", []string{"kebab", ".", "--exclude", "["}},
		{"missing target", []string{"kebab", "./does-not-exist"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			if got := runCLI(t, "", tc.args...); got.err == nil {
				t.Errorf("run succeeded, want an error (output %q)", got.out)
			}
		})
	}
}

func TestConfigFileSuppliesDefaults(t *testing.T) {
	root := makeTree(t, "Sub Dir/My File.txt")
	config := filepath.Join(t.TempDir(), "casemv.yaml")
	if err := os.WriteFile(config, []byte("recursive: true\ndir: true\nquiet: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := runCLI(t, "", "kebab", root, "--config", config)

	if got.err != nil {
		t.Fatalf("run: %v (output %q)", got.err, got.out)
	}
	assertTree(t, root, "sub-dir/", "sub-dir/my-file.txt")
}

// Viper would otherwise read casemv.json, casemv.toml and the rest, and pick
// one of them over casemv.yaml.
func TestUserConfigIsOnlyCasemvYAML(t *testing.T) {
	root := makeTree(t, "Sub Dir/My File.txt")
	configDir := withUserConfig(t, "recursive: true\n")
	others := map[string]string{"casemv.json": `{"dir": true}`, "casemv.yml": "dir: true\n", "casemv.toml": "dir = true\n"}
	for name, content := range others {
		if err := os.WriteFile(filepath.Join(configDir, "casemv", name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if got := runCLI(t, "", "kebab", root); got.err != nil {
		t.Fatalf("run: %v (output %q)", got.err, got.out)
	}
	assertTree(t, root, "Sub Dir/", "Sub Dir/my-file.txt")
}

// A casemv.yaml in the working directory must not be read: an untrusted
// directory could otherwise escalate a top-level rename into a recursive,
// hidden-inclusive one that rewrites .git.
func TestConfigFileInTheWorkingDirectoryIsIgnored(t *testing.T) {
	root := makeTree(t, "Sub Dir/My File.txt", ".git/COMMIT_EDITMSG")
	t.Chdir(root)
	if err := os.WriteFile("casemv.yaml", []byte("recursive: true\ndir: true\nhidden: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := runCLI(t, "", "kebab", "."); got.err != nil {
		t.Fatalf("run: %v (output %q)", got.err, got.out)
	}
	assertTree(t, root,
		"casemv.yaml",
		"Sub Dir/", "Sub Dir/My File.txt",
		".git/", ".git/COMMIT_EDITMSG",
	)
}

func TestFlagsOverrideTheConfigFile(t *testing.T) {
	root := makeTree(t, "Sub Dir/My File.txt")
	config := filepath.Join(t.TempDir(), "casemv.yaml")
	if err := os.WriteFile(config, []byte("recursive: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := runCLI(t, "", "kebab", root, "--config", config, "--recursive"); got.err != nil {
		t.Fatalf("run: %v", got.err)
	}
	assertTree(t, root, "Sub Dir/", "Sub Dir/my-file.txt")
}

func TestMissingExplicitConfigFileIsAnError(t *testing.T) {
	got := runCLI(t, "", "kebab", t.TempDir(), "--config", filepath.Join(t.TempDir(), "absent.yaml"))
	if got.err == nil {
		t.Error("run succeeded, want an error for a missing --config file")
	}
}

func TestEnvironmentSuppliesDefaults(t *testing.T) {
	root := makeTree(t, "Sub Dir/My File.txt")
	t.Setenv("CASEMV_RECURSIVE", "true")

	if got := runCLI(t, "", "kebab", root); got.err != nil {
		t.Fatalf("run: %v", got.err)
	}
	assertTree(t, root, "Sub Dir/", "Sub Dir/my-file.txt")
}

func TestLowerExtensionSources(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		env    map[string]string
		config string
	}{
		{"long flag", []string{"--lower-ext"}, nil, ""},
		{"short flag", []string{"-x"}, nil, ""},
		{"environment", nil, map[string]string{"CASEMV_LOWER_EXT": "true"}, ""},
		{"config file", nil, nil, "lower-ext: true\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := makeTree(t, "My File.TXT", "Sub Dir.D/")
			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			args := append([]string{"kebab", root, "--dir"}, tc.args...)
			if tc.config != "" {
				args = append(args, "--config", writeConfig(t, tc.config))
			}

			if got := runCLI(t, "", args...); got.err != nil {
				t.Fatalf("run: %v (output %q)", got.err, got.out)
			}
			assertTree(t, root, "my-file.txt", "sub-dir-d/")
		})
	}
}

func TestExtensionsAreKeptWithoutLowerExtension(t *testing.T) {
	root := makeTree(t, "My File.TXT")

	if got := runCLI(t, "", "kebab", root); got.err != nil {
		t.Fatalf("run: %v (output %q)", got.err, got.out)
	}
	assertTree(t, root, "my-file.TXT")
}

func TestRepeatedExcludeFlags(t *testing.T) {
	root := makeTree(t, "Keep One.txt", "Skip One.log", "Skip Two.tmp")
	got := runCLI(t, "", "kebab", root, "-e", "*.log", "-e", "*.tmp")

	if got.err != nil {
		t.Fatalf("run: %v", got.err)
	}
	assertTree(t, root, "keep-one.txt", "Skip One.log", "Skip Two.tmp")
}

func TestVersionFlag(t *testing.T) {
	got := runCLI(t, "", "--version")
	if got.err != nil {
		t.Fatalf("run: %v", got.err)
	}
	if !strings.Contains(got.out, "casemv version ") {
		t.Errorf("output = %q, want a version line", got.out)
	}
}

func TestHelpListsEveryConvention(t *testing.T) {
	got := runCLI(t, "", "--help")
	if got.err != nil {
		t.Fatalf("run: %v", got.err)
	}
	for _, want := range []string{"kebab", "screaming", "1. Something", "--dry-run", "-x, --lower-ext", "-c, --convention", "[convention] [target]"} {
		if !strings.Contains(got.out, want) {
			t.Errorf("help output is missing %q", want)
		}
	}
}

func TestOutputGoesToTheConfiguredWriterNotStdout(t *testing.T) {
	root := makeTree(t, "My File.txt")
	stdout := os.Stdout
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = write
	t.Cleanup(func() { os.Stdout = stdout })

	runCLI(t, "", "kebab", root)
	if err = write.Close(); err != nil {
		t.Fatal(err)
	}

	leaked, err := io.ReadAll(read)
	if err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}
	if len(leaked) != 0 {
		t.Errorf("command wrote %q directly to os.Stdout", leaked)
	}
}

// failingWriter reports a write error, standing in for a closed pipe.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("pipe closed") }

// failingReader reports a read error partway through stdin.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("input truncated") }

func runWith(t *testing.T, stdin io.Reader, stdout io.Writer, args ...string) error {
	t.Helper()
	cmd := cli.NewRootCmd()
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(io.Discard)
	cmd.SetIn(stdin)
	return cmd.ExecuteContext(t.Context())
}

func TestWriteErrorIsReportedAfterRenaming(t *testing.T) {
	root := makeTree(t, "My File.txt")
	err := runWith(t, strings.NewReader(""), failingWriter{}, "kebab", root)

	if err == nil || !strings.Contains(err.Error(), "write output") {
		t.Fatalf("error = %v, want a write-output error", err)
	}
	// The rename still happened; only the report failed.
	assertTree(t, root, "my-file.txt")
}

func TestWriteErrorStopsTextMode(t *testing.T) {
	err := runWith(t, strings.NewReader("One\nTwo\n"), failingWriter{}, "kebab", "--text")
	if err == nil || !strings.Contains(err.Error(), "write output") {
		t.Fatalf("error = %v, want a write-output error", err)
	}
}

func TestReadErrorIsReportedInTextMode(t *testing.T) {
	var out bytes.Buffer
	err := runWith(t, failingReader{}, &out, "kebab", "--text")
	if err == nil || !strings.Contains(err.Error(), "read input") {
		t.Fatalf("error = %v, want a read-input error", err)
	}
}

func TestExecuteReturnsProcessExitCodes(t *testing.T) {
	tests := []struct {
		name string
		args func(root string) []string
		want int
	}{
		{"success", func(root string) []string { return []string{"casemv", "kebab", root, "--quiet"} }, 0},
		{"failure", func(string) []string { return []string{"casemv", "not-a-convention"} }, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := makeTree(t, "My File.txt")
			t.Chdir(t.TempDir())

			args := os.Args
			stderr := os.Stderr
			devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
			if err != nil {
				t.Fatal(err)
			}
			os.Args, os.Stderr = tc.args(root), devNull
			t.Cleanup(func() {
				os.Args, os.Stderr = args, stderr
				if err := devNull.Close(); err != nil {
					t.Errorf("close %s: %v", os.DevNull, err)
				}
			})

			if got := cli.Execute(); got != tc.want {
				t.Errorf("Execute() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestBuildVersionIsNeverEmpty(t *testing.T) {
	got := runCLI(t, "", "--version")
	if fields := strings.Fields(got.out); len(fields) < 3 {
		t.Errorf("version output %q has no version token", got.out)
	}
}

func TestRenamesAreReportedAsTheyLand(t *testing.T) {
	root := makeTree(t, "A File.txt", "B File.txt")
	got := runCLI(t, "", "kebab", root)

	if got.err != nil {
		t.Fatalf("run: %v", got.err)
	}
	for _, want := range []string{"a-file.txt", "b-file.txt", "2 entries renamed in "} {
		if !strings.Contains(got.out, want) {
			t.Errorf("output %q is missing %q", got.out, want)
		}
	}
}

// A failed run must not leave a listing claiming renames that never happened.
func TestFailedRunReportsOnlyCompletedRenames(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	root := makeTree(t, "A Dir/Deep File.txt", "Locked Dir/Blocked File.txt")
	locked := filepath.Join(root, "Locked Dir")
	if err := os.Chmod(locked, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(locked, 0o755); err != nil {
			t.Errorf("restore permissions: %v", err)
		}
	})

	got := runCLI(t, "", "kebab", root, "--recursive")
	if got.err == nil {
		t.Fatal("run succeeded, want a permission error")
	}
	if strings.Contains(got.out, "blocked-file.txt") {
		t.Errorf("output %q reports a rename that never happened", got.out)
	}
	if !strings.Contains(got.out, "deep-file.txt") {
		t.Errorf("output %q omits the rename that did happen", got.out)
	}
}

func TestDryRunStillListsThePlanUpFront(t *testing.T) {
	root := makeTree(t, "A File.txt")
	got := runCLI(t, "", "kebab", root, "--dry-run")

	if got.err != nil {
		t.Fatalf("run: %v", got.err)
	}
	if !strings.Contains(got.out, "a-file.txt") {
		t.Errorf("dry-run output %q omits the planned rename", got.out)
	}
	assertTree(t, root, "A File.txt")
}

func TestInterruptedRunExitsWithTheSignalCode(t *testing.T) {
	root := makeTree(t, "My File.txt")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	var out bytes.Buffer
	cmd := cli.NewRootCmd()
	cmd.SetArgs([]string{"kebab", root})
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(""))

	err := cmd.ExecuteContext(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	assertTree(t, root, "My File.txt")
}

func TestUndecodableNameSurvivesTheCLI(t *testing.T) {
	root := makeTree(t, "Caf\xe9 Menu.txt")
	if got := runCLI(t, "", "kebab", root); got.err != nil {
		t.Fatalf("run: %v", got.err)
	}
	assertTree(t, root, "caf\xe9-menu.txt")
}

func TestReportMapsErrorsToExitCodes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
		says string
	}{
		{"success", nil, 0, ""},
		{"failure", errors.New("boom"), 1, "casemv: boom\n"},
		{"interrupted", context.Canceled, cli.ExitInterrupted, "casemv: interrupted\n"},
		{"wrapped interrupt", fmt.Errorf("apply: %w", context.Canceled), cli.ExitInterrupted, "casemv: interrupted\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if got := cli.Report(&out, tc.err); got != tc.want {
				t.Errorf("Report(%v) = %d, want %d", tc.err, got, tc.want)
			}
			if tc.says != "" && out.String() != tc.says {
				t.Errorf("output = %q, want %q", out.String(), tc.says)
			}
			if tc.says == "" && out.Len() != 0 {
				t.Errorf("output = %q, want empty", out.String())
			}
			if strings.Contains(out.String(), "context canceled") {
				t.Errorf("output %q leaks internal error string", out.String())
			}
		})
	}
}

func TestTextModeRejectsIncompatibleOptions(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"rejects target argument", []string{"kebab", "some-target", "--text"}, "--text does not accept a target argument"},
		{"rejects dry-run", []string{"kebab", "--text", "--dry-run"}, "--text cannot be used with --dry-run"},
		{"rejects quiet", []string{"kebab", "--text", "--quiet"}, "--text cannot be used with --quiet"},
		{"rejects lower-ext", []string{"kebab", "--text", "--lower-ext"}, "--text cannot be used with --lower-ext"},
		{"rejects recursive", []string{"kebab", "--text", "-r"}, "--text cannot be used with --recursive"},
		{"rejects dir", []string{"kebab", "--text", "-D"}, "--text cannot be used with --dir"},
		{"rejects hidden", []string{"kebab", "--text", "-H"}, "--text cannot be used with --hidden"},
		{"rejects exclude", []string{"kebab", "--text", "-e", "*.bak"}, "--text cannot be used with --exclude"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := runCLI(t, "hello\n", tc.args...)
			if got.err == nil {
				t.Fatalf("run succeeded, want error %q", tc.want)
			}
			if !strings.Contains(got.err.Error(), tc.want) {
				t.Errorf("error = %q, want substring %q", got.err.Error(), tc.want)
			}
		})
	}
}

// Defaults set for renaming must not stop --text: only flags typed on the
// command line conflict with it.
func TestTextModeIgnoresRenameDefaults(t *testing.T) {
	const defaults = "dry-run: true\nquiet: true\nlower-ext: true\n"
	tests := []struct {
		name   string
		config string
		env    map[string]string
		args   []string
	}{
		{"user config file", defaults, nil, nil},
		{"environment", "", map[string]string{"CASEMV_DRY_RUN": "true", "CASEMV_QUIET": "true", "CASEMV_LOWER_EXT": "true"}, nil},
		{"flags turned off", "", nil, []string{"--dry-run=false", "--quiet=false", "--lower-ext=false"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			args := append([]string{"kebab", "--text"}, tc.args...)

			withUserConfig(t, tc.config)
			got := runCLI(t, "My File.TXT\n", args...)

			if got.err != nil {
				t.Fatalf("run: %v", got.err)
			}
			if got.out != "my-file-txt\n" {
				t.Errorf("output = %q, want %q", got.out, "my-file-txt\n")
			}
		})
	}
}

// A stored text default would turn every run into a stdin filter.
func TestTextModeIsCommandLineOnly(t *testing.T) {
	tests := []struct {
		name   string
		config string
		env    map[string]string
	}{
		{"config file", "text: true\n", nil},
		{"environment", "", map[string]string{"CASEMV_TEXT": "true"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			root := makeTree(t, "My File.txt")

			withUserConfig(t, tc.config)
			got := runCLI(t, "", "kebab", root)

			if want := `"text" cannot be set`; got.err == nil || !strings.Contains(got.err.Error(), want) {
				t.Errorf("error = %v, want substring %q", got.err, want)
			}
			assertTree(t, root, "My File.txt")
		})
	}
}

func TestTextModeAcceptsFileFlagsSetToFalse(t *testing.T) {
	got := runCLI(t, "My File\n", "kebab", "--text", "--dry-run=false", "--recursive=false")
	if got.err != nil {
		t.Fatalf("run: %v", got.err)
	}
	if got.out != "my-file\n" {
		t.Errorf("output = %q, want %q", got.out, "my-file\n")
	}
}

func TestTextModeHandlesLinesLongerThan64KiB(t *testing.T) {
	const lineLen = 70 * 1024
	longLine := strings.Repeat("a", lineLen) + "\n"
	got := runCLI(t, longLine, "kebab", "--text")
	if got.err != nil {
		t.Fatalf("run: %v", got.err)
	}
	want := strings.Repeat("a", lineLen) + "\n"
	if got.out != want {
		t.Errorf("output length = %d, want %d", len(got.out), len(want))
	}
}

func TestTextModePreservesWhitespaceAndEmptyLines(t *testing.T) {
	input := "  First Line  \n   \n\tThird Line\t\n\nFourth Line\n"
	got := runCLI(t, input, "lower", "--text")
	if got.err != nil {
		t.Fatalf("run: %v", got.err)
	}
	want := "  first line  \n   \n\tthird line\t\n\nfourth line\n"
	if got.out != want {
		t.Errorf("output = %q, want %q", got.out, want)
	}
}

func TestTextModePreservesCRLFEmptyLines(t *testing.T) {
	input := "First Line\r\n\r\nSecond Line\r\n"
	got := runCLI(t, input, "kebab", "--text")
	if got.err != nil {
		t.Fatalf("run: %v", got.err)
	}
	want := "first-line\n\nsecond-line\n"
	if got.out != want {
		t.Errorf("output = %q, want %q", got.out, want)
	}
}

// Only the line terminator goes; a carriage return before it is content.
func TestTextModeStripsOnlyTheLineTerminator(t *testing.T) {
	got := runCLI(t, "A\r\r\nB\r\n", "lower", "--text")
	if got.err != nil {
		t.Fatalf("run: %v", got.err)
	}
	if want := "a\r\nb\n"; got.out != want {
		t.Errorf("output = %q, want %q", got.out, want)
	}
}

// A carriage return ends a line only before a newline, so a last line
// without one keeps it.
func TestTextModeKeepsCarriageReturnAtEndOfInput(t *testing.T) {
	got := runCLI(t, "A\nB\r", "lower", "--text")
	if got.err != nil {
		t.Fatalf("run: %v", got.err)
	}
	if want := "a\nb\r\n"; got.out != want {
		t.Errorf("output = %q, want %q", got.out, want)
	}
}

func TestTextModeProcessesLineWithoutTrailingNewline(t *testing.T) {
	got := runCLI(t, "First Line\nSecond Line", "kebab", "--text")
	if got.err != nil {
		t.Fatalf("run: %v", got.err)
	}
	if want := "first-line\nsecond-line\n"; got.out != want {
		t.Errorf("output = %q, want %q", got.out, want)
	}
}

func TestTextModeProcessesSingleLineWithoutTrailingNewline(t *testing.T) {
	got := runCLI(t, "Single Line", "kebab", "--text")
	if got.err != nil {
		t.Fatalf("run: %v", got.err)
	}
	if want := "single-line\n"; got.out != want {
		t.Errorf("output = %q, want %q", got.out, want)
	}
}

func TestInterruptedRunDoesNotWrapError(t *testing.T) {
	root := makeTree(t, "File One.txt", "File Two.txt")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	var out bytes.Buffer
	cmd := cli.NewRootCmd()
	cmd.SetArgs([]string{"kebab", root})
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(""))

	err := cmd.ExecuteContext(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if strings.Contains(fmt.Sprint(err), "renamed before the failure") {
		t.Errorf("error %v contains misleading failure suffix", err)
	}
}

func TestUnknownFlagDisplaysUsage(t *testing.T) {
	got := runCLI(t, "", "kebab", "--recurse")
	if got.err == nil {
		t.Fatal("run succeeded, want an error for unknown flag")
	}
	if !strings.Contains(got.err.Error(), "unknown flag: --recurse") {
		t.Errorf("error = %q, want unknown flag error", got.err.Error())
	}
	if !strings.Contains(got.out, "Usage:") {
		t.Errorf("output %q is missing usage information", got.out)
	}
}

func TestRuntimeFailureHidesUsage(t *testing.T) {
	got := runCLI(t, "", "kebab", filepath.Join(t.TempDir(), "nonexistent"))
	if got.err == nil {
		t.Fatal("run succeeded, want an error for nonexistent directory")
	}
	if strings.Contains(got.out, "Usage:") {
		t.Errorf("output %q should not contain usage on runtime failure", got.out)
	}
}

// viper's GetBool reads an unparsable value as false, so "yes" silently
// turned a dry run into a real one.
func TestInvalidBooleanSettingIsAnError(t *testing.T) {
	tests := []struct {
		name   string
		env    map[string]string
		config string
	}{
		{"env dry run yes", map[string]string{"CASEMV_DRY_RUN": "yes"}, ""},
		{"env recursive on", map[string]string{"CASEMV_RECURSIVE": "on"}, ""},
		{"env lower-ext yes", map[string]string{"CASEMV_LOWER_EXT": "yes"}, ""},
		{"config dry run yes", nil, "dry-run: yes\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := makeTree(t, "My File.txt")
			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			args := []string{"kebab", root}
			if tc.config != "" {
				args = append(args, "--config", writeConfig(t, tc.config))
			}

			got := runCLI(t, "", args...)

			if got.err == nil || !strings.Contains(got.err.Error(), "invalid boolean") {
				t.Errorf("error = %v, want an invalid boolean error", got.err)
			}
			assertTree(t, root, "My File.txt")
		})
	}
}

func TestUnknownConfigKeyIsAnError(t *testing.T) {
	root := makeTree(t, "My File.txt")
	config := writeConfig(t, "dry_run: true\n")

	got := runCLI(t, "", "kebab", root, "--config", config)

	if got.err == nil || !strings.Contains(got.err.Error(), `unknown config key "dry_run"`) {
		t.Errorf("error = %v, want an unknown key error", got.err)
	}
	assertTree(t, root, "My File.txt")
}

// Cobra adds --help and --version; neither is a setting.
func TestHelpAndVersionAreNotSettings(t *testing.T) {
	for _, key := range []string{"help", "version"} {
		t.Run(key+" config key", func(t *testing.T) {
			root := makeTree(t, "My File.txt")
			config := writeConfig(t, key+": true\n")

			got := runCLI(t, "", "kebab", root, "--config", config)

			if got.err == nil || !strings.Contains(got.err.Error(), fmt.Sprintf("unknown config key %q", key)) {
				t.Errorf("error = %v, want an unknown key error", got.err)
			}
			assertTree(t, root, "My File.txt")
		})
		t.Run(key+" environment variable", func(t *testing.T) {
			root := makeTree(t, "My File.txt")
			t.Setenv("CASEMV_"+strings.ToUpper(key), "1.2.3")

			if got := runCLI(t, "", "kebab", root); got.err != nil {
				t.Fatalf("run: %v (output %q)", got.err, got.out)
			}
			assertTree(t, root, "my-file.txt")
		})
	}
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "casemv.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// Names come from trees the user may not control; printed raw, escape
// sequences would drive the terminal and newlines would fake output lines.
func TestProgressQuotesNamesATerminalWouldInterpret(t *testing.T) {
	const evil = "Evil\x1b]0;pwned\x07Name.txt"
	const spoof = "Fake\nX.txt -> gone.txt"
	root := makeTree(t, evil, spoof, "Plain Name.txt")

	got := runCLI(t, "", "kebab", root, "--dry-run")

	if got.err != nil {
		t.Fatalf("run: %v", got.err)
	}
	if strings.ContainsAny(got.out, "\x1b\x07") {
		t.Errorf("output carries raw control bytes: %q", got.out)
	}
	if lines := strings.Count(got.out, "\n"); lines != 4 {
		t.Errorf("output has %d lines, want 4 (3 renames + summary): %q", lines, got.out)
	}
	for _, want := range []string{
		strconv.Quote(filepath.Join(root, evil)),
		strconv.Quote(filepath.Join(root, spoof)),
		filepath.Join(root, "Plain Name.txt") + " -> plain-name.txt\n",
	} {
		if !strings.Contains(got.out, want) {
			t.Errorf("output %q lacks %q", got.out, want)
		}
	}
}

// Errors carry paths raw, and a newline in one would fake an output line.
func TestReportEscapesControlCharactersAndLineBreaks(t *testing.T) {
	var out bytes.Buffer
	cli.Report(&out, errors.New("walk Evil\x1b[2J\xe9\ncasemv: fake: denied a\\b"))

	if want := "casemv: walk Evil\\x1b[2J\\xe9\\ncasemv: fake: denied a\\\\b\n"; out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}

// A collision report spans lines, one per conflict; names in it must not add any.
func TestReportKeepsOneLinePerCollision(t *testing.T) {
	root := makeTree(t, "Fake\ncasemv: Dir/My File.txt", "Fake\ncasemv: Dir/my_file.txt")
	got := runCLI(t, "", "kebab", root, "--recursive")
	if got.err == nil {
		t.Fatal("run succeeded, want a collision")
	}

	var out bytes.Buffer
	cli.Report(&out, got.err)

	if lines := strings.Count(out.String(), "\n"); lines != 2 {
		t.Errorf("report has %d lines, want 2 (summary + conflict): %q", lines, out.String())
	}
	if want := strconv.Quote(root + "/Fake\ncasemv: Dir"); !strings.Contains(out.String(), want) {
		t.Errorf("report %q lacks quoted directory %s", out.String(), want)
	}
}

// The collision message quotes its paths already; escaping again would double backslashes.
func TestReportEscapesCollisionBackslashesOnce(t *testing.T) {
	collision := &rename.CollisionError{Conflicts: []rename.Conflict{{Dir: "/d", Name: "n", Sources: []string{`a\b.txt`, "c"}}}}

	var out bytes.Buffer
	cli.Report(&out, collision)

	if want := `claimed by "a\\b.txt", "c"` + "\n"; !strings.HasSuffix(out.String(), want) {
		t.Errorf("report %q does not end with %q", out.String(), want)
	}
}

// Only the collision message quotes its paths; text wrapping it may hold raw ones.
func TestReportEscapesLineBreaksOfAWrappedCollision(t *testing.T) {
	collision := &rename.CollisionError{Conflicts: []rename.Conflict{{Dir: "/d", Name: "n", Sources: []string{"a", "b"}}}}

	var out bytes.Buffer
	cli.Report(&out, fmt.Errorf("plan x\ny: %w", collision))

	if lines := strings.Count(out.String(), "\n"); lines != 1 {
		t.Errorf("report has %d lines, want 1: %q", lines, out.String())
	}
	if !strings.Contains(out.String(), `plan x\ny: `) {
		t.Errorf("report %q lacks the escaped wrapper text", out.String())
	}
}

func TestCollisionSourcesAreQuoted(t *testing.T) {
	root := makeTree(t, "My File.txt", "my\nfile.txt")

	got := runCLI(t, "", "kebab", root)

	if got.err == nil || !strings.Contains(got.err.Error(), `"my\nfile.txt"`) {
		t.Errorf("error = %q, want the newline source quoted", got.err)
	}
}

// countingWriter records each Write call, to show how output is batched.
type countingWriter struct {
	bytes.Buffer

	writes int
}

func (w *countingWriter) Write(p []byte) (int, error) {
	w.writes++
	return w.Buffer.Write(p)
}

func TestTextModeBatchesWritesForBufferedInput(t *testing.T) {
	var in, want strings.Builder
	for i := range 1000 {
		fmt.Fprintf(&in, "My File %d\n", i)
		fmt.Fprintf(&want, "my-file-%d\n", i)
	}
	out := &countingWriter{}

	if err := runWith(t, strings.NewReader(in.String()), out, "kebab", "--text"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if out.String() != want.String() {
		t.Error("converted output differs from the expected lines")
	}
	if out.writes > 10 {
		t.Errorf("%d writes for 1000 lines, want them batched", out.writes)
	}
}

// Batching must not hold back a line while casemv waits for the next one,
// or an interactive or streaming caller would see nothing.
func TestTextModeWritesEachLineBeforeWaitingForMore(t *testing.T) {
	stdin, feed := io.Pipe()
	lines := make(chan string, 10)
	done := make(chan error, 1)
	go func() { done <- runWith(t, stdin, lineWriter(lines), "kebab", "--text") }()

	if _, err := feed.Write([]byte("My File\n")); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-lines:
		if got != "my-file\n" {
			t.Errorf("first write = %q, want %q", got, "my-file\n")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("line not written while input stayed open")
	}
	if err := feed.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Errorf("run: %v", err)
	}
}

// A read blocked on idle input ignores signals; cancelling must not wait for it.
func TestTextModeStopsWhileWaitingForInput(t *testing.T) {
	stdin, feed := io.Pipe()
	t.Cleanup(func() {
		if err := feed.Close(); err != nil {
			t.Error(err)
		}
	})
	lines := make(chan string, 10)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	cmd := cli.NewRootCmd()
	cmd.SetArgs([]string{"kebab", "--text"})
	cmd.SetIn(stdin)
	cmd.SetOut(lineWriter(lines))
	cmd.SetErr(io.Discard)
	done := make(chan error, 1)
	go func() { done <- cmd.ExecuteContext(ctx) }()

	// Once the first line is out, casemv is waiting for the second.
	if _, err := feed.Write([]byte("My File\n")); err != nil {
		t.Fatal(err)
	}
	<-lines
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("run error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run still waiting for input after cancel")
	}
}

type lineWriter chan string

func (w lineWriter) Write(p []byte) (int, error) {
	w <- string(p)
	return len(p), nil
}

func TestDryRunListingIsWrittenOnce(t *testing.T) {
	spec := make([]string, 20)
	for i := range spec {
		spec[i] = fmt.Sprintf("My File %d.txt", i)
	}
	root := makeTree(t, spec...)
	out := &countingWriter{}

	if err := runWith(t, strings.NewReader(""), out, "kebab", root, "--dry-run"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if out.writes != 1 {
		t.Errorf("dry run made %d writes, want 1", out.writes)
	}
}

// Progress stays unbuffered: a run killed part-way must have printed every
// rename that landed.
func TestEachRenameIsWrittenAsItLands(t *testing.T) {
	root := makeTree(t, "A File.txt", "B File.txt", "C File.txt")
	out := &countingWriter{}

	if err := runWith(t, strings.NewReader(""), out, "kebab", root); err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := 3 + 1; out.writes != want {
		t.Errorf("%d writes, want %d: one per rename plus the summary", out.writes, want)
	}
}

func TestExcludeSources(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		env    string
		config string
	}{
		{"flags", []string{"-e", "Skip Me.txt", "-e", "*.log"}, "", ""},
		{"environment splits at colons", nil, "Skip Me.txt:*.log", ""},
		{"config list", nil, "", "exclude: [Skip Me.txt, '*.log']\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := makeTree(t, "Skip Me.txt", "Skip Me.log", "Keep Me.txt")
			if tc.env != "" {
				t.Setenv("CASEMV_EXCLUDE", tc.env)
			}
			args := append([]string{"kebab", root}, tc.args...)
			if tc.config != "" {
				args = append(args, "--config", writeConfig(t, tc.config))
			}

			if got := runCLI(t, "", args...); got.err != nil {
				t.Fatalf("run: %v (output %q)", got.err, got.out)
			}
			assertTree(t, root, "Skip Me.txt", "Skip Me.log", "keep-me.txt")
		})
	}
}

// Viper reads a mistyped value as empty, which would silently drop excludes.
func TestMistypedConfigValuesAreErrors(t *testing.T) {
	for _, config := range []string{
		"convention: [kebab]\n",
		"convention: {a: 1}\n",
		"exclude: {a: 1}\n",
		"exclude: 123\n",
		"exclude: [1]\n",
	} {
		t.Run(config, func(t *testing.T) {
			root := makeTree(t, "My File.txt")

			got := runCLI(t, "", "kebab", root, "--config", writeConfig(t, config))

			if got.err == nil || !strings.Contains(got.err.Error(), "invalid") {
				t.Errorf("error = %v, want an invalid value error", got.err)
			}
			assertTree(t, root, "My File.txt")
		})
	}
}

func TestConfigKeyInsideConfigIsAnError(t *testing.T) {
	root := makeTree(t, "My File.txt")
	config := writeConfig(t, "config: "+writeConfig(t, "recursive: true\n")+"\n")

	got := runCLI(t, "", "kebab", root, "--config", config)

	if got.err == nil || !strings.Contains(got.err.Error(), `"config"`) {
		t.Errorf("error = %v, want an error naming the config key", got.err)
	}
	assertTree(t, root, "My File.txt")
}

func TestConfigEnvironmentSelectsTheConfigFile(t *testing.T) {
	t.Run("used without --config", func(t *testing.T) {
		root := makeTree(t, "Sub Dir/My File.txt")
		t.Setenv("CASEMV_CONFIG", writeConfig(t, "recursive: true\n"))

		if got := runCLI(t, "", "kebab", root); got.err != nil {
			t.Fatalf("run: %v", got.err)
		}
		assertTree(t, root, "Sub Dir/", "Sub Dir/my-file.txt")
	})
	t.Run("must exist", func(t *testing.T) {
		t.Setenv("CASEMV_CONFIG", filepath.Join(t.TempDir(), "absent.yaml"))

		if got := runCLI(t, "", "kebab", makeTree(t)); got.err == nil {
			t.Error("run succeeded, want an error for a missing CASEMV_CONFIG file")
		}
	})
	t.Run("--config wins", func(t *testing.T) {
		root := makeTree(t, "My File.txt")
		t.Setenv("CASEMV_CONFIG", writeConfig(t, "recursive: maybe\n"))

		if got := runCLI(t, "", "kebab", root, "--config", writeConfig(t, "dir: true\n")); got.err != nil {
			t.Fatalf("run: %v", got.err)
		}
		assertTree(t, root, "my-file.txt")
	})
}

func TestConfigErrorsAreReportedInKeyOrder(t *testing.T) {
	config := writeConfig(t, "h: 1\ng: 1\nf: 1\ne: 1\nd: 1\nc: 1\nb: 1\na: 1\n")

	got := runCLI(t, "", "kebab", makeTree(t), "--config", config)

	if got.err == nil || !strings.Contains(got.err.Error(), `unknown config key "a"`) {
		t.Errorf("error = %v, want the first key in order reported", got.err)
	}
}
