//go:build docker

// Package docker runs the casemv test suites inside a disposable container.
// It is excluded from the default build; run it with:
//
//	go test -tags docker -count=1 -timeout 20m ./test/docker/
package docker

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	image      = "casemv-test:local"
	dockerfile = "Dockerfile"
	// buildContext is repository root relative to package.
	buildContext = "../.."
	stepTimeout  = 15 * time.Minute
)

// runArgs keep the container disposable and isolated: --rm removes it on exit,
// the module cache is already baked in so no network is needed, and no host
// path is mounted, writable or otherwise.
var runArgs = []string{
	"run", "--rm",
	"--network", "none",
	"--security-opt", "no-new-privileges",
}

var (
	buildOnce sync.Once
	errBuild  error
)

func TestSuitesPassInsideContainer(t *testing.T) {
	buildImage(t)
	out := dockerRun(t, append(runArgs, image)...)

	for _, want := range []string{"internal/casing", "internal/rename", "internal/cli", "test/e2e"} {
		if !strings.Contains(out, want) {
			t.Errorf("container output does not report package %s:\n%s", want, out)
		}
	}
	if strings.Contains(out, "FAIL") {
		t.Errorf("container reported a failure:\n%s", out)
	}
}

// TestRenameScenarioOnContainerFilesystem checks the flagship behaviour end to
// end without the Go test harness: a dotted directory name is converted whole
// and the tree is renamed from the leaves up.
func TestRenameScenarioOnContainerFilesystem(t *testing.T) {
	buildImage(t)

	const script = `
set -eu
mkdir -p "/work/1. Something/2. Nested Dir"
touch "/work/1. Something/2. Nested Dir/My File.TXT"
touch "/work/1. Something/README.md"
casemv kebab /work --recursive --dir --quiet
cd /work && find . | LC_ALL=C sort
`
	out := dockerRun(t, append(runArgs, image, "sh", "-c", script)...)

	want := strings.Join([]string{
		".",
		"./1-something",
		"./1-something/2-nested-dir",
		"./1-something/2-nested-dir/my-file.TXT",
		"./1-something/readme.md",
	}, "\n")
	if got := strings.TrimSpace(out); got != want {
		t.Errorf("container tree mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRootUserStillRenamesCorrectly(t *testing.T) {
	buildImage(t)
	args := append(append([]string{}, runArgs...), "--user", "0:0", image,
		"sh", "-c", `set -eu
mkdir -p "/tmp/t/A Dir"
touch "/tmp/t/A Dir/B File.txt"
casemv snake /tmp/t -r -D --quiet
cd /tmp/t && find . | LC_ALL=C sort`)
	out := dockerRun(t, args...)

	want := ".\n./a_dir\n./a_dir/b_file.txt"
	if got := strings.TrimSpace(out); got != want {
		t.Errorf("root run mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
}

// Kernels before 5.6 lack openat2, and older container runtimes deny syscalls
// their seccomp profile does not list with EPERM instead of ENOSYS. Either way
// renames must fall back rather than fail, and the rename package's tests,
// symlink refusal included, must still pass.
func TestRenameWithoutOpenat2(t *testing.T) {
	buildImage(t)
	for name, errno := range map[string]int{"EPERM": 1, "ENOSYS": 38} {
		t.Run(name, func(t *testing.T) {
			// Allow-by-default keeps the profile to the one syscall under
			// test; the container stays unprivileged, offline and unmounted.
			profile := fmt.Sprintf(`{"defaultAction":"SCMP_ACT_ALLOW","syscalls":`+
				`[{"names":["openat2"],"action":"SCMP_ACT_ERRNO","errnoRet":%d}]}`, errno)
			path := filepath.Join(t.TempDir(), "seccomp.json")
			if err := os.WriteFile(path, []byte(profile), 0o644); err != nil {
				t.Fatal(err)
			}
			seccomp := slices.Concat(runArgs, []string{"--security-opt", "seccomp=" + path, image})
			dockerRun(t, slices.Concat(seccomp, []string{"go", "test", "-count=1", "./internal/rename/"})...)

			args := slices.Concat(seccomp, []string{"sh", "-c", `set -eu
mkdir -p "/work/A Dir"
touch "/work/A Dir/B File.txt"
casemv snake /work -r -D --quiet
cd /work && find . | LC_ALL=C sort`})
			out := dockerRun(t, args...)

			want := ".\n./a_dir\n./a_dir/b_file.txt"
			if got := strings.TrimSpace(out); got != want {
				t.Errorf("tree mismatch\n got:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func buildImage(t *testing.T) {
	t.Helper()
	requireDocker(t)
	buildOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), stepTimeout)
		defer cancel()

		var out bytes.Buffer
		cmd := exec.CommandContext(ctx, "docker", "build", "-f", dockerfile, "-t", image, buildContext)
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			errBuild = fmt.Errorf("docker build: %w\n%s", err, out.String())
		}
	})
	if errBuild != nil {
		t.Fatal(errBuild)
	}
}

func dockerRun(t *testing.T, args ...string) string {
	t.Helper()
	return dockerCmd(t, args...)
}

func dockerCmd(t *testing.T, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), stepTimeout)
	defer cancel()

	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("docker %s: %v\n%s", strings.Join(args, " "), err, out.String())
	}
	return out.String()
}

func requireDocker(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not installed")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "docker", "info").Run(); err != nil {
		t.Skip("docker daemon unavailable")
	}
}
