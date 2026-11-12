package testtree

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// SkipOnWindows skips a test whose fixture Windows cannot hold, such as a name
// with a control character, or a directory chmod makes read-only. reason says
// which.
func SkipOnWindows(tb testing.TB, reason string) {
	tb.Helper()
	if runtime.GOOS == "windows" {
		tb.Skip("Windows: " + reason)
	}
}

// SkipUnlessInvalidUTF8Names skips a test unless the filesystem of the temporary
// directory keeps a name that is not valid UTF-8. APFS refuses one, and Windows
// stores names as UTF-16, so it replaces the invalid byte.
func SkipUnlessInvalidUTF8Names(tb testing.TB) {
	tb.Helper()
	const name = "\xff"
	dir := tb.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
		tb.Skipf("filesystem refuses invalid UTF-8 names: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		tb.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != name {
		tb.Skip("filesystem does not keep invalid UTF-8 in names")
	}
}
