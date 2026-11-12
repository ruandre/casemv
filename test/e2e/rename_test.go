//go:build e2e

package e2e

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestDottedDirectoriesConvertWhole(t *testing.T) {
	root := makeTree(t,
		"1. Something/2. Nested Dir/My File.TXT",
		"1. Something/1. Introduction.md",
		"3. Another Thing/",
	)
	casemv(t, "", "kebab", root, "-r", "-D").requireSuccess(t)

	assertTree(t, root,
		"1-something/",
		"1-something/1-introduction.md",
		"1-something/2-nested-dir/",
		"1-something/2-nested-dir/my-file.TXT",
		"3-another-thing/",
	)
}

func TestDeepTreeRenamesBottomUp(t *testing.T) {
	const depth = 8
	parts := make([]string, 0, depth)
	for level := range depth {
		parts = append(parts, fmt.Sprintf("%d. Level Dir", level))
	}
	deepest := strings.Join(parts, "/")
	root := makeTree(t, deepest+"/Leaf File.txt")

	casemv(t, "", "snake", root, "-r", "-D").requireSuccess(t)

	want := root
	for level := range depth {
		want = filepath.Join(want, fmt.Sprintf("%d_level_dir", level))
	}
	if _, err := os.Stat(filepath.Join(want, "leaf_file.txt")); err != nil {
		t.Errorf("deepest leaf missing after rename: %v", err)
	}
}

func TestDryRunChangesNothingAndSucceeds(t *testing.T) {
	root := makeTree(t, "My File.txt", "Sub Dir/Inner.txt")
	got := casemv(t, "", "kebab", root, "-r", "-D", "--dry-run").requireSuccess(t)

	assertTree(t, root, "My File.txt", "Sub Dir/", "Sub Dir/Inner.txt")
	if !strings.Contains(got.stdout, "dry run:") {
		t.Errorf("stdout %q is missing the dry-run summary", got.stdout)
	}
}

func TestCollisionFailsWithoutTouchingTheTree(t *testing.T) {
	root := makeTree(t, "My File.txt", "my_file.txt", "MY-FILE.txt")
	casemv(t, "", "kebab", root).requireFailure(t, "collision")

	assertTree(t, root, "MY-FILE.txt", "My File.txt", "my_file.txt")
}

func TestUnknownConventionFails(t *testing.T) {
	casemv(t, "", "kebab-case", t.TempDir()).requireFailure(t, "unknown naming convention")
}

func TestMissingTargetFails(t *testing.T) {
	casemv(t, "", "kebab", filepath.Join(t.TempDir(), "absent")).
		requireFailure(t, "read target directory")
}

func TestTextModeReadsStdin(t *testing.T) {
	got := casemv(t, "My File Name\n1. Something\n\nMore Content\n", "kebab", "--text").requireSuccess(t)
	if want := "my-file-name\n1-something\n\nmore-content\n"; got.stdout != want {
		t.Errorf("stdout = %q, want %q", got.stdout, want)
	}
}

func TestHiddenEntriesNeedTheFlag(t *testing.T) {
	root := makeTree(t, ".Config Dir/Inner File.txt", "Visible File.txt")

	casemv(t, "", "kebab", root, "-r", "-D").requireSuccess(t)
	assertTree(t, root, ".Config Dir/", ".Config Dir/Inner File.txt", "visible-file.txt")

	casemv(t, "", "kebab", root, "-r", "-D", "--hidden").requireSuccess(t)
	assertTree(t, root, ".config-dir/", ".config-dir/inner-file.txt", "visible-file.txt")
}

func TestNamesWithAwkwardCharacters(t *testing.T) {
	root := makeTree(t,
		"Weird\tTab Name.txt",
		"Ünïcödé Nàme.txt",
		"日本語 File.txt",
		"name with 'quotes'.txt",
		"trailing space .txt",
	)
	casemv(t, "", "kebab", root).requireSuccess(t)

	assertTree(t, root,
		"weird-tab-name.txt",
		"ünïcödé-nàme.txt",
		"日本語-file.txt",
		"name-with-quotes.txt",
		"trailing-space.txt",
	)
}

func TestNewlineInName(t *testing.T) {
	root := t.TempDir()
	name := "Line One\nLine Two.txt"
	if err := os.WriteFile(filepath.Join(root, name), nil, 0o644); err != nil {
		t.Skipf("filesystem rejects newlines in names: %v", err)
	}
	casemv(t, "", "kebab", root).requireSuccess(t)
	assertTree(t, root, "line-one-line-two.txt")
}

func TestUnwritableDirectoryReportsFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	root := makeTree(t, "Locked Dir/My File.txt")
	locked := filepath.Join(root, "Locked Dir")
	if err := os.Chmod(locked, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(locked, 0o755); err != nil {
			t.Errorf("restore permissions on %s: %v", locked, err)
		}
	})

	casemv(t, "", "kebab", root, "-r").requireFailure(t, "rename")
	assertTree(t, root, "Locked Dir/", "Locked Dir/My File.txt")
}

func TestLargeTreeIsRenamedCorrectly(t *testing.T) {
	const dirs, filesPerDir = 20, 100
	root := t.TempDir()
	for dir := range dirs {
		sub := filepath.Join(root, "Some Dir "+strconv.Itoa(dir))
		mustMkdir(t, sub)
		for file := range filesPerDir {
			path := filepath.Join(sub, "My File "+strconv.Itoa(file)+".txt")
			if err := os.WriteFile(path, nil, 0o644); err != nil {
				t.Fatalf("create %s: %v", path, err)
			}
		}
	}

	got := casemv(t, "", "kebab", root, "-r", "-D", "--quiet").requireSuccess(t)
	if got.stdout != "" {
		t.Errorf("quiet run wrote %q to stdout", got.stdout)
	}

	for dir := range dirs {
		sub := filepath.Join(root, "some-dir-"+strconv.Itoa(dir))
		if _, err := os.Stat(filepath.Join(sub, "my-file-"+strconv.Itoa(filesPerDir-1)+".txt")); err != nil {
			t.Fatalf("expected renamed entry missing: %v", err)
		}
	}
}

func TestRunningTwiceIsANoOp(t *testing.T) {
	root := makeTree(t, "1. Something/My File.txt")
	casemv(t, "", "kebab", root, "-r", "-D").requireSuccess(t)
	before := treePaths(t, root)

	got := casemv(t, "", "kebab", root, "-r", "-D").requireSuccess(t)
	if !strings.Contains(got.stdout, "0 entries renamed") {
		t.Errorf("stdout %q, want a zero-rename summary", got.stdout)
	}
	assertTree(t, root, before...)
}

func TestUndecodableFilenamesKeepTheirBytes(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"Caf\xe9 Menu.txt", "Na\xefve Report.md"} {
		if err := os.WriteFile(filepath.Join(root, name), nil, 0o644); err != nil {
			t.Fatalf("create %q: %v", name, err)
		}
	}

	casemv(t, "", "kebab", root).requireSuccess(t)
	assertTree(t, root, "caf\xe9-menu.txt", "na\xefve-report.md")
}

func TestSymlinkedTargetDirectoryIsFollowed(t *testing.T) {
	parent := makeTree(t, "real/My File.txt", "real/Sub Dir/Inner File.txt")
	link := filepath.Join(parent, "link")
	if err := os.Symlink(filepath.Join(parent, "real"), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	got := casemv(t, "", "kebab", link, "-r", "-D").requireSuccess(t)
	if strings.Contains(got.stdout, "0 entries renamed") {
		t.Errorf("symlinked target renamed nothing: %q", got.stdout)
	}
	assertTree(t, filepath.Join(parent, "real"),
		"my-file.txt", "sub-dir/", "sub-dir/inner-file.txt",
	)
}

// A casemv.yaml in the working directory must not widen the run.
func TestWorkingDirectoryConfigIsIgnored(t *testing.T) {
	root := makeTree(t, "Read Me.txt", ".git/COMMIT_EDITMSG")
	config := filepath.Join(root, "casemv.yaml")
	if err := os.WriteFile(config, []byte("recursive: true\ndir: true\nhidden: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.CommandContext(t.Context(), binary, "kebab", ".")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+t.TempDir())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}

	assertTree(t, root, "casemv.yaml", "read-me.txt", ".git/", ".git/COMMIT_EDITMSG")
}

func TestExplicitConfigFileStillApplies(t *testing.T) {
	root := makeTree(t, "Sub Dir/My File.txt")
	config := filepath.Join(t.TempDir(), "casemv.yaml")
	if err := os.WriteFile(config, []byte("recursive: true\ndir: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	casemv(t, "", "kebab", root, "--config", config).requireSuccess(t)
	assertTree(t, root, "sub-dir/", "sub-dir/my-file.txt")
}

func TestTextModeRejectsIncompatibleOptions(t *testing.T) {
	casemv(t, "input\n", "kebab", t.TempDir(), "--text").
		requireFailure(t, "--text does not accept a target argument")
	casemv(t, "input\n", "kebab", "--text", "--dry-run").
		requireFailure(t, "--text cannot be used with --dry-run")
	casemv(t, "input\n", "kebab", "--text", "--quiet").
		requireFailure(t, "--text cannot be used with --quiet")
}

func TestTextModeHandlesLongLines(t *testing.T) {
	const lineLen = 70 * 1024
	longLine := strings.Repeat("a", lineLen) + "\n"
	got := casemv(t, longLine, "lower", "--text").requireSuccess(t)
	want := strings.Repeat("a", lineLen) + "\n"
	if got.stdout != want {
		t.Errorf("stdout length = %d, want %d", len(got.stdout), len(want))
	}
}

func TestTextModePreservesWhitespaceAndEmptyLines(t *testing.T) {
	input := "  First Line  \n   \n\tThird Line\t\n\nFourth Line\n"
	got := casemv(t, input, "lower", "--text").requireSuccess(t)
	want := "  first line  \n   \n\tthird line\t\n\nfourth line\n"
	if got.stdout != want {
		t.Errorf("stdout = %q, want %q", got.stdout, want)
	}
}

func TestUnknownFlagDisplaysUsage(t *testing.T) {
	got := casemv(t, "", "kebab", "--recurse")
	got.requireFailure(t, "unknown flag: --recurse")
	if !strings.Contains(got.stderr, "Usage:") {
		t.Errorf("stderr %q is missing usage information", got.stderr)
	}
}

func TestRuntimeFailureHidesUsage(t *testing.T) {
	got := casemv(t, "", "kebab", filepath.Join(t.TempDir(), "absent"))
	got.requireFailure(t, "read target directory")
	if strings.Contains(got.stderr, "Usage:") {
		t.Errorf("stderr %q should not contain usage on runtime failure", got.stderr)
	}
}

// A reader that stops early, such as head, must not stop the run part-way.
func TestClosedStdoutStillRenamesEverything(t *testing.T) {
	// Enough progress output to fill the pipe buffer, so casemv is still
	// writing when the reader goes away.
	const files = 5000
	root := t.TempDir()
	for i := range files {
		if err := os.WriteFile(filepath.Join(root, "My File "+strconv.Itoa(i)), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	exitCode, stderr := runWithStdoutClosedEarly(t, "kebab", root)

	// -1 means a signal killed it.
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
	if !strings.Contains(stderr, "broken pipe") {
		t.Errorf("stderr = %q, want a broken pipe error", stderr)
	}
	for _, path := range treePaths(t, root) {
		if strings.Contains(path, " ") {
			t.Fatalf("%q was not renamed", path)
		}
	}
}

// runWithStdoutClosedEarly closes casemv's stdout after its first byte.
func runWithStdoutClosedEarly(t *testing.T, args ...string) (exitCode int, stderr string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), binary, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var errOut bytes.Buffer
	cmd.Stderr = &errOut
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+t.TempDir())
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := stdout.Read(make([]byte, 1)); err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := stdout.Close(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil && !errors.As(err, new(*exec.ExitError)) {
		t.Fatalf("wait: %v", err)
	}
	return cmd.ProcessState.ExitCode(), errOut.String()
}

func TestCleanInterruptReporting(t *testing.T) {
	signals := map[string]os.Signal{"SIGINT": os.Interrupt, "SIGTERM": syscall.SIGTERM, "SIGHUP": syscall.SIGHUP}
	for name, signal := range signals {
		t.Run(name, func(t *testing.T) { testCleanInterrupt(t, signal) })
	}
}

func testCleanInterrupt(t *testing.T, signal os.Signal) {
	t.Helper()
	proc := startTextMode(t)

	// A round-trip line shows the signal handler is installed.
	if _, err := proc.stdin.Write([]byte("First Line\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, len("first-line\n"))
	if _, err := proc.stdout.Read(buf); err != nil {
		t.Fatalf("read: %v", err)
	}

	// Stdin stays open and idle: one signal must end the run on its own.
	if err := proc.cmd.Process.Signal(signal); err != nil {
		t.Fatalf("signal: %v", err)
	}
	waited := make(chan error, 1)
	go func() { waited <- proc.cmd.Wait() }()
	var err error
	select {
	case err = <-waited:
	case <-time.After(5 * time.Second):
		t.Fatal("casemv still running 5s after one signal")
	}

	exit, ok := errors.AsType[*exec.ExitError](err)
	if !ok {
		t.Fatal("wait error is not an ExitError")
	}
	if exit.ExitCode() != 130 {
		t.Errorf("exit code = %d, want 130", exit.ExitCode())
	}
	if want := "casemv: interrupted\n"; proc.stderr.String() != want {
		t.Errorf("stderr = %q, want %q", proc.stderr.String(), want)
	}
}

// textProcess is a running `casemv kebab --text` with its standard streams.
type textProcess struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr *bytes.Buffer
}

func startTextMode(t *testing.T) textProcess {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), binary, "kebab", "--text")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+t.TempDir())
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	return textProcess{cmd: cmd, stdin: stdin, stdout: stdout, stderr: &stderr}
}

func TestLowerExtensionFlag(t *testing.T) {
	root := makeTree(t, "My File.TXT", "notes.MD", "Sub Dir/Inner.JSON")
	casemv(t, "", "kebab", root, "-r", "-D", "-x").requireSuccess(t)

	assertTree(t, root, "my-file.txt", "notes.md", "sub-dir/", "sub-dir/inner.json")
}

func TestConfigFileSetsDefaultConvention(t *testing.T) {
	root := makeTree(t, "My File.TXT")
	config := filepath.Join(t.TempDir(), "casemv.yaml")
	if err := os.WriteFile(config, []byte("convention: kebab\nlower-ext: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	casemv(t, "", root, "--config", config).requireSuccess(t)

	assertTree(t, root, "my-file.txt")
}
