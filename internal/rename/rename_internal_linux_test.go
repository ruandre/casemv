package rename

import (
	"errors"
	"os"
	"path/filepath"
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

// Without openat2, dir resolves one component at a time and must still refuse
// symlinks, including ones that stay inside the target, and "..".
func TestRenameWithoutOpenat2StaysBeneathTheTargetWithoutSymlinks(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"Kept", "A/B"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
		createFile(t, filepath.Join(root, dir, "My File.txt"))
	}
	if err := os.Symlink("Kept", filepath.Join(root, "Sub")); err != nil {
		t.Fatal(err)
	}
	target, err := openTargetDir(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := target.close(); err != nil {
			t.Error(err)
		}
	})

	for _, dir := range []string{"Sub", "A/../Sub", ".."} {
		if err := target.renameWithoutOpenat2(dir, "My File.txt", "my-file.txt"); err == nil {
			t.Errorf("rename in %q succeeded, want refusal", dir)
		}
	}
	assertNames(t, filepath.Join(root, "Kept"), "My File.txt")

	if err := target.renameWithoutOpenat2("A/B", "My File.txt", "my-file.txt"); err != nil {
		t.Fatalf("rename in A/B: %v", err)
	}
	assertNames(t, filepath.Join(root, "A", "B"), "my-file.txt")
}
