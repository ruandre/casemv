//go:build e2e

package e2e

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/ruandre/casemv/internal/testtree"
)

// invocation is the outcome of one casemv run.
type invocation struct {
	stdout   string
	stderr   string
	exitCode int
}

func casemv(t *testing.T, stdin string, args ...string) invocation {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(t.Context(), binary, args...)
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = isolatedEnv(t.TempDir())

	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit):
	default:
		t.Fatalf("run %s %v: %v", binary, args, err)
	}
	return invocation{stdout: stdout.String(), stderr: stderr.String(), exitCode: cmd.ProcessState.ExitCode()}
}

// isolatedEnv is the host environment without CASEMV_* variables, and with
// configHome as XDG_CONFIG_HOME, so neither the host's settings nor its user
// config reach casemv.
func isolatedEnv(configHome string, extra ...string) []string {
	env := slices.DeleteFunc(os.Environ(), func(entry string) bool {
		return strings.HasPrefix(entry, "CASEMV_")
	})
	return append(env, append([]string{"XDG_CONFIG_HOME=" + configHome}, extra...)...)
}

func (i invocation) requireSuccess(t *testing.T) invocation {
	t.Helper()
	if i.exitCode != 0 {
		t.Fatalf("exit code %d, want 0\nstdout: %s\nstderr: %s", i.exitCode, i.stdout, i.stderr)
	}
	return i
}

func (i invocation) requireFailure(t *testing.T, wantInStderr string) invocation {
	t.Helper()
	if i.exitCode == 0 {
		t.Fatalf("exit code 0, want failure\nstdout: %s", i.stdout)
	}
	if !strings.Contains(i.stderr, wantInStderr) {
		t.Errorf("stderr %q does not contain %q", i.stderr, wantInStderr)
	}
	return i
}

func makeTree(t *testing.T, spec ...string) string {
	t.Helper()
	return testtree.Make(t, spec...)
}

func treePaths(t *testing.T, root string) []string {
	t.Helper()
	return testtree.Snapshot(t, root)
}

func assertTree(t *testing.T, root string, want ...string) {
	t.Helper()
	testtree.Assert(t, root, want...)
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}
