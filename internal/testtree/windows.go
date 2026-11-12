package testtree

import (
	"runtime"
	"testing"
)

// SkipOnWindows skips a test whose fixture Windows cannot hold, such as a name
// with a control character or invalid UTF-8, or a directory chmod makes
// read-only. reason says which.
func SkipOnWindows(tb testing.TB, reason string) {
	tb.Helper()
	if runtime.GOOS == "windows" {
		tb.Skip("Windows: " + reason)
	}
}
