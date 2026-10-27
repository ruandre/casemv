package rename_test

import (
	"testing"

	"github.com/ruandre/casemv/internal/testtree"
)

func makeTree(tb testing.TB, spec ...string) string {
	tb.Helper()
	return testtree.Make(tb, spec...)
}

func snapshot(t *testing.T, root string) []string {
	t.Helper()
	return testtree.Snapshot(t, root)
}

func assertTree(t *testing.T, root string, want ...string) {
	t.Helper()
	testtree.Assert(t, root, want...)
}
