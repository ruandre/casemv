package rename

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/ruandre/casemv/internal/testtree"
)

// renameAfterCheck stands in for renameAt on filesystems without
// RENAME_NOREPLACE, where rename(2) between two names of one entry succeeds
// without changing anything; both must still land or refuse the rename.
var renameStrategies = map[string]move{
	"RENAME_NOREPLACE":  renameAt,
	"check then rename": renameAfterCheck,
}

func TestRenameChangesCaseOnCaseFoldingDir(t *testing.T) {
	parent, ok := testtree.CaseFoldingDir(t)
	if !ok {
		return
	}
	for name, rename := range renameStrategies {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(parent, name)
			if err := os.Mkdir(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			createFile(t, filepath.Join(dir, "Foo.txt"))

			if err := rename(openDir(t, dir), "Foo.txt", "foo.txt"); err != nil {
				t.Fatalf("rename: %v", err)
			}
			assertNames(t, dir, "foo.txt")
		})
	}
}

func TestRenameRefusesAnotherLinkToTheSameFile(t *testing.T) {
	for name, rename := range renameStrategies {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			createFile(t, filepath.Join(dir, "a.txt"))
			if err := os.Link(filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.txt")); err != nil {
				t.Fatal(err)
			}

			err := rename(openDir(t, dir), "a.txt", "b.txt")

			if !errors.Is(err, unix.EEXIST) {
				t.Errorf("rename error = %v, want EEXIST", err)
			}
			assertNames(t, dir, "a.txt", "b.txt")
		})
	}
}

func createFile(t *testing.T, path string) {
	t.Helper()
	if err := writeEmpty(path); err != nil {
		t.Fatal(err)
	}
}

func openDir(t *testing.T, dir string) int {
	t.Helper()
	fd, err := unix.Open(dir, unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := unix.Close(fd); err != nil {
			t.Error(err)
		}
	})
	return fd
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
