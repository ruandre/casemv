package rename_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ruandre/casemv/internal/casing"
	"github.com/ruandre/casemv/internal/rename"
	"github.com/ruandre/casemv/internal/testtree"
)

// NFC "é" and its NFD spelling "e" + U+0301 are one name to the kernel's casefold.
const (
	cafeNFC = "Café"
	cafeNFD = "café"
)

// Each case holds names the kernel's casefold lookup treats as one entry while
// strings.ToLower does not, plus probe blind spots.
func TestCollisionsMatchKernelCaseFolding(t *testing.T) {
	parent, ok := testtree.CaseFoldingDir(t)
	if !ok {
		return
	}
	tests := []struct {
		name  string
		files []string
	}{
		{"unicode normalization", []string{cafeNFC + " Menu.txt", cafeNFD + "-menu.txt"}},
		{"full case folding", []string{"Straße Map.txt", "strasse-map.txt"}},
		{"long s folds to s", []string{"Baſ Note.txt", "bas-note.txt"}},
		{"undecodable sibling does not blind the probe", []string{"A B.txt", "a_b.TXT", "0x\xe9y"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := makeTreeIn(t, parent, tc.files...)
			before := snapshot(t, root)

			_, err := rename.Build(t.Context(), rename.Options{Root: root}, casing.Kebab)

			if _, ok := errors.AsType[*rename.CollisionError](err); !ok {
				t.Errorf("Build error = %v, want *CollisionError", err)
			}
			assertTree(t, root, before...)
		})
	}
}

// Undecodable names compare byte for byte under casefold, so two of them that
// differ only in an invalid byte are distinct entries, not a collision.
func TestUndecodableNamesDoNotFalselyCollideWhenFolding(t *testing.T) {
	parent, ok := testtree.CaseFoldingDir(t)
	if !ok {
		return
	}
	root := makeTreeIn(t, parent, "Read Me.txt", "caf\xe9.txt", "caf\xe8.txt")

	apply(t, rename.Options{Root: root}, casing.Kebab)

	assertTree(t, root, "read-me.txt", "caf\xe9.txt", "caf\xe8.txt")
}

// rename(2) between two spellings of one casefold entry succeeds without
// changing the stored name, so a plain rename reports work it did not do.
func TestCaseOnlyRenameLandsWhenFolding(t *testing.T) {
	parent, ok := testtree.CaseFoldingDir(t)
	if !ok {
		return
	}
	root := makeTreeIn(t, parent, "README.md", "Sub Dir/")

	apply(t, rename.Options{Root: root, IncludeDirs: true}, casing.Kebab)

	assertTree(t, root, "readme.md", "sub-dir/")
}

func makeTreeIn(t *testing.T, parent string, names ...string) string {
	t.Helper()
	//nolint:usetesting // must sit on the casefold mount, which vanishes with the namespace
	root, err := os.MkdirTemp(parent, "tree")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		path := filepath.Join(root, name)
		if name[len(name)-1] == '/' {
			err = os.Mkdir(path, 0o755)
		} else {
			err = os.WriteFile(path, []byte(name), 0o644)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	return root
}
