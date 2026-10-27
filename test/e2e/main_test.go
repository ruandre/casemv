//go:build e2e

// Package e2e drives the built casemv binary against real directory trees.
// It is intended to run inside the disposable container built by
// test/docker/Dockerfile, but works anywhere: set CASEMV_BIN to an existing
// binary, or let TestMain build one.
package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

var binary string

func TestMain(m *testing.M) {
	code, err := run(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e setup:", err)
		os.Exit(1)
	}
	os.Exit(code)
}

func run(m *testing.M) (int, error) {
	if binary = os.Getenv("CASEMV_BIN"); binary != "" {
		return m.Run(), nil
	}
	dir, err := os.MkdirTemp("", "casemv-e2e")
	if err != nil {
		return 0, err
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintln(os.Stderr, "e2e cleanup:", err)
		}
	}()

	binary = filepath.Join(dir, "casemv")
	build := exec.CommandContext(context.Background(), "go", "build", "-o", binary, ".")
	build.Dir = filepath.Join("..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		return 0, fmt.Errorf("build casemv: %w\n%s", err, out)
	}
	return m.Run(), nil
}
