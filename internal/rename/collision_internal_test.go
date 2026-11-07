package rename

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// conflictsIn takes the filesystem's case behaviour as an argument so both
// branches run on an ordinary ext4 host; mounting a vfat image to exercise the
// folding branch end to end needs privileges the test suite does not take.
func TestConflictsInCaseFolding(t *testing.T) {
	const dir = "/tree"
	names := []string{"Foo Bar.txt", "foo-bar.TXT"}
	changed := map[string]string{filepath.Join(dir, "Foo Bar.txt"): "foo-bar.txt"}

	t.Run("case-sensitive keeps them distinct", func(t *testing.T) {
		if got := conflictsIn(dir, names, changed, false); len(got) != 0 {
			t.Errorf("got %+v, want no conflicts", got)
		}
	})

	t.Run("case-folding sees one target", func(t *testing.T) {
		got := conflictsIn(dir, names, changed, true)
		if len(got) != 1 {
			t.Fatalf("got %+v, want exactly one conflict", got)
		}
		if want := []string{"Foo Bar.txt", "foo-bar.TXT"}; !slices.Equal(got[0].Sources, want) {
			t.Errorf("sources = %v, want %v", got[0].Sources, want)
		}
	})
}

func TestConflictsInSingleFileCaseOnlyRenameCaseFolding(t *testing.T) {
	const dir = "/tree"
	names := []string{"README.txt"}
	changed := map[string]string{filepath.Join(dir, "README.txt"): "readme.txt"}

	if got := conflictsIn(dir, names, changed, true); len(got) != 0 {
		t.Errorf("got %+v, want no conflicts for single-file case-only rename with foldCase=true", got)
	}
}

func TestConflictsInCaseOnlyRenameWithSiblingsCaseFolding(t *testing.T) {
	const dir = "/tree"
	names := []string{"README.txt", "other.txt"}
	changed := map[string]string{filepath.Join(dir, "README.txt"): "readme.txt"}

	if got := conflictsIn(dir, names, changed, true); len(got) != 0 {
		t.Errorf("got %+v, want no conflicts for case-only rename with siblings with foldCase=true", got)
	}
}

func TestConflictsInSwapCycle(t *testing.T) {
	const dir = "/tree"
	names := []string{"a.txt", "b.txt"}
	changed := map[string]string{
		filepath.Join(dir, "a.txt"): "b.txt",
		filepath.Join(dir, "b.txt"): "a.txt",
	}

	got := conflictsIn(dir, names, changed, false)
	if len(got) != 2 {
		t.Fatalf("got %d conflicts, want 2: %+v", len(got), got)
	}
	if got[0].Name != "a.txt" || !slices.Equal(got[0].Sources, []string{"a.txt", "b.txt"}) {
		t.Errorf("conflict 0 = %+v, want name %q sources [a.txt b.txt]", got[0], "a.txt")
	}
	if got[1].Name != "b.txt" || !slices.Equal(got[1].Sources, []string{"a.txt", "b.txt"}) {
		t.Errorf("conflict 1 = %+v, want name %q sources [a.txt b.txt]", got[1], "b.txt")
	}
}

func TestCheckCollisionsSwapCycle(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.txt")
	fileB := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(fileA, []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileB, []byte("B"), 0o644); err != nil {
		t.Fatal(err)
	}

	tr := tree{
		siblings: map[string][]string{
			dir: {"a.txt", "b.txt"},
		},
	}
	changed := map[string]string{
		fileA: "b.txt",
		fileB: "a.txt",
	}

	err := checkCollisions(tr, changed)
	var collision *CollisionError
	if !errors.As(err, &collision) {
		t.Fatalf("checkCollisions error = %v, want *CollisionError", err)
	}
	if len(collision.Conflicts) != 2 {
		t.Fatalf("got %d conflicts, want 2: %+v", len(collision.Conflicts), collision.Conflicts)
	}

	gotA, err := os.ReadFile(fileA)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotA) != "A" {
		t.Errorf("file a.txt content = %q, want %q", string(gotA), "A")
	}
	gotB, err := os.ReadFile(fileB)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotB) != "B" {
		t.Errorf("file b.txt content = %q, want %q", string(gotB), "B")
	}
}

func TestConflictsInChain(t *testing.T) {
	const dir = "/tree"
	names := []string{"a.txt", "b.txt"}
	changed := map[string]string{
		filepath.Join(dir, "a.txt"): "b.txt",
		filepath.Join(dir, "b.txt"): "c.txt",
	}

	got := conflictsIn(dir, names, changed, false)
	if len(got) != 1 {
		t.Fatalf("got %d conflicts, want 1: %+v", len(got), got)
	}
	if got[0].Name != "b.txt" || !slices.Equal(got[0].Sources, []string{"a.txt", "b.txt"}) {
		t.Errorf("got %+v, want conflict on b.txt claimed by a.txt, b.txt", got[0])
	}
}

func TestConflictsInCycle3(t *testing.T) {
	const dir = "/tree"
	names := []string{"a.txt", "b.txt", "c.txt"}
	changed := map[string]string{
		filepath.Join(dir, "a.txt"): "b.txt",
		filepath.Join(dir, "b.txt"): "c.txt",
		filepath.Join(dir, "c.txt"): "a.txt",
	}

	got := conflictsIn(dir, names, changed, false)
	if len(got) != 3 {
		t.Fatalf("got %d conflicts, want 3: %+v", len(got), got)
	}
}

func TestConflictsInIgnoresUnaffectedDirectories(t *testing.T) {
	got := conflictsIn("/tree", []string{"a.txt", "A.TXT"}, nil, true)
	if len(got) != 0 {
		t.Errorf("got %+v, want no conflicts when nothing is renamed", got)
	}
}

func TestFlipCase(t *testing.T) {
	tests := map[string]string{
		"Mixed Case.txt": "mIXED cASE.TXT",
		"ǅ Ⅶ Ⓐ":          "ǆ ⅶ ⓐ",
		"1234":           "1234",
		"":               "",
	}
	for input, want := range tests {
		if got := flipCase(input); got != want {
			t.Errorf("flipCase(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestIgnoresCaseOnHostFilesystem(t *testing.T) {
	dir := t.TempDir()
	if err := writeEmpty(filepath.Join(dir, "Probe File.txt")); err != nil {
		t.Fatal(err)
	}
	folds := ignoresCase(dir, []string{"Probe File.txt"})

	// host answer must match actual filesystem behavior.
	err := writeEmpty(filepath.Join(dir, "probe file.txt"))
	entries, readErr := readNames(dir)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if err != nil {
		t.Fatalf("create second probe: %v", err)
	}
	if wantFolds := len(entries) == 1; folds != wantFolds {
		t.Errorf("ignoresCase = %v, but the directory holds %d entries", folds, len(entries))
	}
}

func TestCheckCollisionsIgnoresUnaffectedDirectories(t *testing.T) {
	tr := tree{
		siblings: map[string][]string{
			"/unaffected": {"foo.txt", "bar.txt"},
		},
	}
	if err := checkCollisions(tr, nil); err != nil {
		t.Errorf("checkCollisions got %v, want nil for empty changed", err)
	}

	tr = tree{
		siblings: map[string][]string{
			"/unaffected": {"foo.txt", "FOO.TXT"},
			"/affected":   {"a b.txt"},
		},
	}
	changed := map[string]string{
		"/affected/a b.txt": "a-b.txt",
	}
	if err := checkCollisions(tr, changed); err != nil {
		t.Errorf("checkCollisions got %v, want nil when unaffected dir has case collision", err)
	}
}

func TestIgnoresCaseShortCircuitsOnMissingFlipped(t *testing.T) {
	dir := t.TempDir()
	if ignoresCase(dir, []string{"Probe.txt"}) {
		t.Skip("filesystem is case-folding")
	}

	first := filepath.Join(dir, "First.txt")
	if err := writeEmpty(first); err != nil {
		t.Fatal(err)
	}

	second := filepath.Join(dir, "Second.txt")
	if err := writeEmpty(second); err != nil {
		t.Fatal(err)
	}
	flippedSecond := filepath.Join(dir, flipCase("Second.txt"))
	if err := os.Link(second, flippedSecond); err != nil {
		t.Skip("hard links unsupported")
	}

	// missing flipped probe must short-circuit before evaluating subsequent entries.
	if got := ignoresCase(dir, []string{"First.txt", "Second.txt"}); got {
		t.Errorf("ignoresCase = true, want false")
	}
}

// Every case-folding filesystem folds ASCII letters, but folding beyond ASCII
// varies, so a missing non-ASCII probe must not decide the answer.
func TestIgnoresCaseLooksPastAMissingNonASCIIProbe(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"0-ı", "Second.txt"} {
		if err := writeEmpty(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	// A second link stands in for a folding lookup of the flipped name.
	if err := os.Link(filepath.Join(dir, "Second.txt"), filepath.Join(dir, "sECOND.TXT")); err != nil {
		t.Skip("hard links unsupported")
	}

	if !ignoresCase(dir, []string{"0-ı", "Second.txt"}) {
		t.Error("ignoresCase = false, want true")
	}
}

// Simple-folding filesystems (vfat, NTFS, APFS) hold "straße" and "strasse"
// side by side; full folding makes them one key, but neither is renamed.
func TestConflictsInIgnoresClaimsNoRenameTakes(t *testing.T) {
	const dir = "/tree"
	names := []string{"My File.txt", "strasse.txt", "straße.txt"}
	changed := map[string]string{filepath.Join(dir, "My File.txt"): "my-file.txt"}

	if got := conflictsIn(dir, names, changed, true); len(got) != 0 {
		t.Errorf("got %+v, want no conflicts", got)
	}
}

// The filesystem can resolve a target that the scanned names do not show,
// here one created after the scan; that must be reported alongside a name
// collision in the same directory, not only once it is fixed.
func TestCheckCollisionsReportsOccupiedTargetsWithNameCollisions(t *testing.T) {
	dir := t.TempDir()
	names := []string{"A B.txt", "C D.txt", "a_b.txt"}
	for _, name := range append(names, "c-d.txt") {
		if err := writeEmpty(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	changed := map[string]string{
		filepath.Join(dir, "A B.txt"): "a-b.txt",
		filepath.Join(dir, "a_b.txt"): "a-b.txt",
		filepath.Join(dir, "C D.txt"): "c-d.txt",
	}

	err := checkCollisions(tree{siblings: map[string][]string{dir: names}}, changed)

	collision, ok := errors.AsType[*CollisionError](err)
	if !ok {
		t.Fatalf("checkCollisions error = %v, want *CollisionError", err)
	}
	got := make([]string, len(collision.Conflicts))
	for i, c := range collision.Conflicts {
		got[i] = c.Name
	}
	if want := []string{"a-b.txt", "c-d.txt"}; !slices.Equal(got, want) {
		t.Errorf("conflict names = %q, want %q", got, want)
	}
}
