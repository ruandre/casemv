package rename

import (
	"os"
	"slices"
	"testing"
)

func writeEmpty(path string) error { return os.WriteFile(path, nil, 0o644) }

func readNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names, nil
}

func assertNames(t *testing.T, dir string, want ...string) {
	t.Helper()
	got, err := readNames(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) {
		t.Errorf("entries = %q, want %q", got, want)
	}
}
