package rename

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/ruandre/casemv/internal/testtree"
)

// rootRenamer serves systems without openat2-style resolution; these tests
// run it on every platform so its guarantees are checked in CI.
func TestRootRenamerRenamesBeneathTheRoot(t *testing.T) {
	root := t.TempDir()
	createRootFile(t, root, "A/B/My File.txt")

	if err := openRootRenamer(t, root).rename("A/B", "My File.txt", "my-file.txt"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	assertNames(t, filepath.Join(root, "A", "B"), "my-file.txt")
}

func TestRootRenamerRefusesSymlinksAndParentComponents(t *testing.T) {
	root := t.TempDir()
	createRootFile(t, root, "Kept/My File.txt")
	if err := os.MkdirAll(filepath.Join(root, "A"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("Kept", filepath.Join(root, "Sub")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	renamer := openRootRenamer(t, root)

	for _, dir := range []string{"Sub", "A/../Sub", "A/../Kept"} {
		if err := renamer.rename(dir, "My File.txt", "my-file.txt"); err == nil {
			t.Errorf("rename in %q succeeded, want refusal", dir)
		}
	}
	assertNames(t, filepath.Join(root, "Kept"), "My File.txt")
}

func TestRootRenamerRefusesAnExistingTarget(t *testing.T) {
	root := t.TempDir()
	createRootFile(t, root, "a.txt")
	createRootFile(t, root, "b.txt")

	if err := openRootRenamer(t, root).rename(".", "a.txt", "b.txt"); !errors.Is(err, fs.ErrExist) {
		t.Errorf("rename error = %v, want fs.ErrExist", err)
	}
	assertNames(t, root, "a.txt", "b.txt")
}

// rename(2) between two links to one file succeeds without changing anything.
func TestRootRenamerRefusesAnotherLinkToTheSameFile(t *testing.T) {
	root := t.TempDir()
	createRootFile(t, root, "a.txt")
	if err := os.Link(filepath.Join(root, "a.txt"), filepath.Join(root, "b.txt")); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}

	if err := openRootRenamer(t, root).rename(".", "a.txt", "b.txt"); !errors.Is(err, fs.ErrExist) {
		t.Errorf("rename error = %v, want fs.ErrExist", err)
	}
	assertNames(t, root, "a.txt", "b.txt")
}

func TestRootRenamerChangesCaseOnCaseFoldingDir(t *testing.T) {
	dir, ok := testtree.CaseFoldingDir(t)
	if !ok {
		return
	}
	createRootFile(t, dir, "Foo.txt")

	if err := openRootRenamer(t, dir).rename(".", "Foo.txt", "foo.txt"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	assertNames(t, dir, "foo.txt")
}

func openRootRenamer(t *testing.T, dir string) rootRenamer {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	})
	return rootRenamer{root: root}
}

func createRootFile(t *testing.T, root, name string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeEmpty(path); err != nil {
		t.Fatal(err)
	}
}
