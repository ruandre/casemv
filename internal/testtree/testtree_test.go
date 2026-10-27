package testtree_test

import (
	"slices"
	"testing"

	"github.com/ruandre/casemv/internal/testtree"
)

func TestMakeAndSnapshot(t *testing.T) {
	t.Parallel()

	root := testtree.Make(t,
		"docs/",
		"src/main.go",
		"README.md",
	)

	got := testtree.Snapshot(t, root)
	want := []string{
		"README.md",
		"docs/",
		"src/",
		"src/main.go",
	}

	if !slices.Equal(got, want) {
		t.Fatalf("snapshot mismatch\n got: %v\nwant: %v", got, want)
	}
}

func TestSnapshotEmptyDir(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	got := testtree.Snapshot(t, root)
	if len(got) != 0 {
		t.Fatalf("expected empty snapshot, got %v", got)
	}
}

func TestAssert(t *testing.T) {
	t.Parallel()

	root := testtree.Make(t,
		"nested/sub/file.txt",
		"empty_dir/",
	)

	testtree.Assert(t, root,
		"empty_dir/",
		"nested/",
		"nested/sub/",
		"nested/sub/file.txt",
	)
}
