package testtree

import (
	"errors"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

const caseFoldingDirEnv = "CASEMV_TEST_CASEFOLD_DIR"

// exitSetupFailed separates "namespace or mount unavailable" from a failing test.
const exitSetupFailed = 77

// mountCaseFolding mounts casefold tmpfs at $1, marks $2 case-folding, then runs the test binary.
const mountCaseFolding = `mount -t tmpfs -o casefold tmpfs "$1" || exit 77
mkdir "$1/cf" && chattr +F "$1/cf" || exit 77
` + caseFoldingDirEnv + `="$1/cf" exec "$2" -test.run="$3" -test.count=1 -test.v`

// CaseFoldingDir returns an empty directory whose lookups fold case the way
// the kernel does (casefold tmpfs). The test runs twice: this call re-runs the
// top-level test in an unprivileged user and mount namespace and returns
// ok=false, so the caller returns; inside the namespace it returns the
// directory and ok=true. Skips when the host cannot provide such a mount.
func CaseFoldingDir(t *testing.T) (dir string, ok bool) {
	t.Helper()
	if parent := os.Getenv(caseFoldingDirEnv); parent != "" {
		//nolint:usetesting // must sit on the casefold mount, which vanishes with the namespace
		dir, err := os.MkdirTemp(parent, "t")
		if err != nil {
			t.Fatalf("create case-folding dir: %v", err)
		}
		return dir, true
	}
	if strings.Contains(t.Name(), "/") {
		t.Fatal("CaseFoldingDir must be called from a top-level test")
	}
	if err := exec.CommandContext(t.Context(), "unshare", "-rm", "true").Run(); err != nil {
		t.Skipf("unprivileged user namespaces unavailable: %v", err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("locate test binary: %v", err)
	}

	pattern := "^" + regexp.QuoteMeta(t.Name()) + "$"
	//nolint:gosec // fixed script; arguments are this test binary and its own test name
	cmd := exec.CommandContext(t.Context(),
		"unshare", "-rm", "sh", "-c", mountCaseFolding, "sh", t.TempDir(), exe, pattern)
	out, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr) && exitErr.ExitCode() == exitSetupFailed:
		t.Skipf("casefold tmpfs unavailable:\n%s", out)
	case err != nil:
		t.Fatalf("test failed on case-folding filesystem: %v\n%s", err, out)
	default:
		t.Logf("passed on case-folding filesystem:\n%s", out)
	}
	return "", false
}
