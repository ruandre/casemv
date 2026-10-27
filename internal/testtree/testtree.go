// Package testtree manages filesystem fixtures for tests.
package testtree

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Make creates filesystem tree under temporary directory from spec.
func Make(tb testing.TB, spec ...string) string {
	tb.Helper()
	root := tb.TempDir()
	for _, entry := range spec {
		path := filepath.Join(root, filepath.FromSlash(entry))
		if strings.HasSuffix(entry, "/") {
			mkdirAll(tb, path)
			continue
		}
		mkdirAll(tb, filepath.Dir(path))
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			tb.Fatalf("create file %s: %v", path, err)
		}
	}
	return root
}

func mkdirAll(tb testing.TB, path string) {
	tb.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		tb.Fatalf("create dir %s: %v", path, err)
	}
}

// Snapshot returns sorted relative slash paths under root with directory entries suffixed by /.
func Snapshot(tb testing.TB, root string) []string {
	tb.Helper()
	var paths []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			rel += "/"
		}
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		tb.Fatalf("snapshot %s: %v", root, err)
	}
	slices.Sort(paths)
	return paths
}

// Assert verifies filesystem tree under root matches expected paths.
func Assert(tb testing.TB, root string, want ...string) {
	tb.Helper()
	expected := slices.Clone(want)
	slices.Sort(expected)
	got := Snapshot(tb, root)
	if !slices.Equal(got, expected) {
		tb.Errorf("tree mismatch\n got: %v\nwant: %v", got, expected)
	}
}
