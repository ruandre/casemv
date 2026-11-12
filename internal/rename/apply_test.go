package rename_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/ruandre/casemv/internal/casing"
	"github.com/ruandre/casemv/internal/rename"
)

// A file created between planning and applying, by a sync client or another
// process, must stop the run rather than be replaced.
func TestApplyNeverReplacesAnEntryCreatedAfterPlanning(t *testing.T) {
	root := makeTree(t, "My File.txt")
	plan := build(t, rename.Options{Root: root}, casing.Kebab)
	late := filepath.Join(root, "my-file.txt")
	if err := os.WriteFile(late, []byte("precious"), 0o644); err != nil {
		t.Fatal(err)
	}

	done, err := plan.Apply(t.Context(), nil)

	if !errors.Is(err, fs.ErrExist) {
		t.Errorf("Apply error = %v, want fs.ErrExist", err)
	}
	if done != 0 {
		t.Errorf("Apply completed %d renames, want 0", done)
	}
	if got, err := os.ReadFile(late); err != nil || string(got) != "precious" {
		t.Errorf("late file content = %q (%v), want %q", got, err, "precious")
	}
}

// A directory swapped for a symlink after planning must not redirect renames
// outside the target.
func TestApplyStaysInsideTheTargetWhenADirectoryBecomesASymlink(t *testing.T) {
	root := makeTree(t, "Sub/My File.txt")
	outside := makeTree(t, "My File.txt")
	plan := build(t, rename.Options{Root: root, Recursive: true}, casing.Kebab)
	sub := filepath.Join(root, "Sub")
	if err := os.RemoveAll(sub); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, sub); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if _, err := plan.Apply(t.Context(), nil); err == nil {
		t.Error("Apply succeeded through a symlink leading outside the target")
	}
	assertTree(t, outside, "My File.txt")
}

// A symlink that stays inside the target still redirects a rename to an entry
// the plan never chose, here an excluded one.
func TestApplyStopsWhenADirectoryBecomesASymlinkInsideTheTarget(t *testing.T) {
	root := makeTree(t, "Sub/My File.txt", "Kept/My File.txt")
	plan := build(t, rename.Options{Root: root, Recursive: true, Exclude: []string{"Kept"}}, casing.Kebab)
	sub := filepath.Join(root, "Sub")
	if err := os.RemoveAll(sub); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("Kept", sub); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if _, err := plan.Apply(t.Context(), nil); err == nil {
		t.Error("Apply succeeded through a symlink swapped in after planning")
	}
	assertTree(t, filepath.Join(root, "Kept"), "My File.txt")
}

// Each rename resolves its directory from the target; that cost must not grow
// with depth.
func BenchmarkApplyDeepTree(b *testing.B) {
	const depth, filesPerDir = 100, 5
	spec := make([]string, 0, depth*filesPerDir)
	dir := ""
	for d := range depth {
		dir += fmt.Sprintf("Dir %d/", d)
		for f := range filesPerDir {
			spec = append(spec, fmt.Sprintf("%sFile %d.txt", dir, f))
		}
	}
	for b.Loop() {
		b.StopTimer()
		root := makeTree(b, spec...)
		plan, err := rename.Build(b.Context(), rename.Options{Root: root, Recursive: true}, casing.Kebab)
		if err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
		if _, err := plan.Apply(b.Context(), nil); err != nil {
			b.Fatal(err)
		}
	}
}
