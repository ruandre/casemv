package rename_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ruandre/casemv/internal/casing"
	"github.com/ruandre/casemv/internal/rename"
)

// build plans against root with given options, failing test on error.
func build(t *testing.T, opts rename.Options, convention rename.Namer) rename.Plan {
	t.Helper()
	plan, err := rename.Build(t.Context(), opts, convention)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return plan
}

// apply plans, applies renames, failing test on error.
func apply(t *testing.T, opts rename.Options, convention rename.Namer) int {
	t.Helper()
	plan := build(t, opts, convention)
	done, err := plan.Apply(t.Context(), nil)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if done != len(plan.Renames) {
		t.Fatalf("Apply reported %d of %d renames", done, len(plan.Renames))
	}
	return done
}

func TestRecursiveRenameConvertsDottedDirectories(t *testing.T) {
	root := makeTree(t,
		"1. Something/2. Nested Dir/My File.TXT",
		"1. Something/README.md",
		"archive.tar.gz",
	)
	apply(t, rename.Options{Root: root, Recursive: true, IncludeDirs: true}, casing.Kebab)

	assertTree(t, root,
		"1-something/",
		"1-something/2-nested-dir/",
		"1-something/2-nested-dir/my-file.TXT",
		"1-something/readme.md",
		"archive-tar.gz",
	)
}

func TestPlanIsOrderedDeepestFirst(t *testing.T) {
	root := makeTree(t, "A Dir/B Dir/C Dir/D File.txt")
	plan := build(t, rename.Options{Root: root, Recursive: true, IncludeDirs: true}, casing.Kebab)

	if len(plan.Renames) != 4 {
		t.Fatalf("got %d renames, want 4: %v", len(plan.Renames), plan.Renames)
	}
	previous := -1
	for _, r := range plan.Renames {
		depth := strings.Count(strings.TrimPrefix(r.From, root), string(filepath.Separator))
		if previous >= 0 && depth > previous {
			t.Fatalf("rename %q at depth %d follows depth %d; plan is not deepest-first", r.From, depth, previous)
		}
		previous = depth
	}
}

// Every source path in the plan must still exist when its rename runs. A
// top-down plan would fail here: renaming "A Dir" first invalidates the paths
// recorded for everything beneath it.
func TestEveryPlannedSourceStillExistsWhenApplied(t *testing.T) {
	root := makeTree(t,
		"A Dir/B Dir/C File.txt",
		"A Dir/B Dir/D File.txt",
		"A Dir/E Dir/F File.txt",
	)
	plan := build(t, rename.Options{Root: root, Recursive: true, IncludeDirs: true}, casing.Snake)

	for _, r := range plan.Renames {
		if _, err := os.Lstat(r.From); err != nil {
			t.Fatalf("source %q missing when its turn came: %v", r.From, err)
		}
		if err := os.Rename(r.From, r.To); err != nil {
			t.Fatalf("rename %q: %v", r.From, err)
		}
	}
	assertTree(t, root,
		"a_dir/", "a_dir/b_dir/", "a_dir/b_dir/c_file.txt", "a_dir/b_dir/d_file.txt",
		"a_dir/e_dir/", "a_dir/e_dir/f_file.txt",
	)
}

func TestNonRecursiveTouchesOnlyTheTopLevel(t *testing.T) {
	root := makeTree(t, "Top File.txt", "Sub Dir/Inner File.txt")
	apply(t, rename.Options{Root: root, IncludeDirs: true}, casing.Kebab)

	assertTree(t, root, "top-file.txt", "sub-dir/", "sub-dir/Inner File.txt")
}

func TestDirectoriesAreLeftAloneWithoutIncludeDirs(t *testing.T) {
	root := makeTree(t, "Sub Dir/Inner File.txt")
	apply(t, rename.Options{Root: root, Recursive: true}, casing.Kebab)

	assertTree(t, root, "Sub Dir/", "Sub Dir/inner-file.txt")
}

func TestTargetDirectoryIsNeverRenamed(t *testing.T) {
	parent := makeTree(t, "My Root/A File.txt")
	root := filepath.Join(parent, "My Root")
	apply(t, rename.Options{Root: root, Recursive: true, IncludeDirs: true}, casing.Kebab)

	assertTree(t, parent, "My Root/", "My Root/a-file.txt")
}

func TestHiddenEntriesAreSkippedByDefault(t *testing.T) {
	root := makeTree(t, ".Hidden Dir/Inner File.txt", ".Hidden File.txt", "Visible File.txt")
	apply(t, rename.Options{Root: root, Recursive: true, IncludeDirs: true}, casing.Kebab)

	assertTree(t, root,
		".Hidden Dir/", ".Hidden Dir/Inner File.txt", ".Hidden File.txt", "visible-file.txt",
	)
}

func TestHiddenEntriesAreIncludedOnRequest(t *testing.T) {
	root := makeTree(t, ".Hidden Dir/Inner File.txt", ".Hidden File.txt")
	apply(t, rename.Options{Root: root, Recursive: true, IncludeDirs: true, IncludeHidden: true}, casing.Kebab)

	assertTree(t, root, ".hidden-dir/", ".hidden-dir/inner-file.txt", ".hidden-file.txt")
}

func TestExcludeSkipsMatchingNamesAndSubtrees(t *testing.T) {
	tests := []struct {
		name    string
		exclude []string
		want    []string
	}{
		{
			name:    "base name glob",
			exclude: []string{"*.min.js"},
			want:    []string{"keep-me.txt", "Skip Me.min.js", "vendor/", "vendor/lib-file.js"},
		},
		{
			name:    "path glob skips the whole subtree",
			exclude: []string{"vendor"},
			want:    []string{"keep-me.txt", "skip-me-min.js", "vendor/", "vendor/Lib File.js"},
		},
		{
			name:    "slash pattern matches the relative path",
			exclude: []string{"vendor/*"},
			want:    []string{"keep-me.txt", "skip-me-min.js", "vendor/", "vendor/Lib File.js"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := makeTree(t, "Keep Me.txt", "Skip Me.min.js", "vendor/Lib File.js")
			apply(t, rename.Options{Root: root, Recursive: true, Exclude: tc.exclude}, casing.Kebab)
			assertTree(t, root, tc.want...)
		})
	}
}

func TestCollisionAbortsBeforeAnythingIsRenamed(t *testing.T) {
	root := makeTree(t, "My File.txt", "my_file.txt", "Untouched.txt")
	before := snapshot(t, root)

	_, err := rename.Build(t.Context(), rename.Options{Root: root}, casing.Kebab)

	var collision *rename.CollisionError
	if !errors.As(err, &collision) {
		t.Fatalf("Build error = %v, want *rename.CollisionError", err)
	}
	if len(collision.Conflicts) != 1 {
		t.Fatalf("got %d conflicts, want 1: %+v", len(collision.Conflicts), collision.Conflicts)
	}
	conflict := collision.Conflicts[0]
	if conflict.Name != "my-file.txt" {
		t.Errorf("conflict name = %q, want %q", conflict.Name, "my-file.txt")
	}
	if len(conflict.Sources) != 2 {
		t.Errorf("conflict sources = %v, want 2 entries", conflict.Sources)
	}
	assertTree(t, root, before...)
}

func TestCollisionWithAnEntryThatKeepsItsName(t *testing.T) {
	// converted sibling absent from plan, but target contested.
	root := makeTree(t, "My File.txt", "my-file.txt")
	_, err := rename.Build(t.Context(), rename.Options{Root: root}, casing.Kebab)

	var collision *rename.CollisionError
	if !errors.As(err, &collision) {
		t.Fatalf("Build error = %v, want *rename.CollisionError", err)
	}
	if !strings.Contains(collision.Error(), "my-file.txt") {
		t.Errorf("error %q does not name the contested target", collision)
	}
}

func TestCollisionWithASkippedSibling(t *testing.T) {
	// excluded entry retains name, claims target.
	root := makeTree(t, "My File.txt", "my-file.txt")
	_, err := rename.Build(t.Context(),
		rename.Options{Root: root, Exclude: []string{"my-file.txt"}}, casing.Kebab)

	if _, ok := errors.AsType[*rename.CollisionError](err); !ok {
		t.Fatalf("Build error = %v, want *rename.CollisionError", err)
	}
}

func TestCollisionIntermediateOverwriteHazard(t *testing.T) {
	// under Pascal casing, both entries claim "Ab.txt".
	root := makeTree(t, "A B.txt", "AB.txt")
	fileA := filepath.Join(root, "A B.txt")
	fileB := filepath.Join(root, "AB.txt")
	if err := os.WriteFile(fileA, []byte("ORIGINAL"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileB, []byte("PRECIOUS"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := rename.Build(t.Context(), rename.Options{Root: root}, casing.Pascal)

	var collision *rename.CollisionError
	if !errors.As(err, &collision) {
		t.Fatalf("Build error = %v, want *rename.CollisionError", err)
	}
	if len(collision.Conflicts) != 1 {
		t.Fatalf("got %d conflicts, want 1: %+v", len(collision.Conflicts), collision.Conflicts)
	}
	conflict := collision.Conflicts[0]
	if conflict.Name != "Ab.txt" {
		t.Errorf("conflict name = %q, want %q", conflict.Name, "Ab.txt")
	}
	if wantSources := []string{"A B.txt", "AB.txt"}; !slices.Equal(conflict.Sources, wantSources) {
		t.Errorf("conflict sources = %v, want %v", conflict.Sources, wantSources)
	}

	assertTree(t, root, "A B.txt", "AB.txt")
	assertContent(t, fileA, "ORIGINAL")
	assertContent(t, fileB, "PRECIOUS")
}

func assertContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("file %q content = %q, want %q", path, got, want)
	}
}

func TestCollisionIntermediateOverwriteHazardCamel(t *testing.T) {
	// under Camel casing, "A A A.txt" -> "aAA.txt", "aAA.txt" -> "aAa.txt".
	root := makeTree(t, "A A A.txt", "aAA.txt")
	_, err := rename.Build(t.Context(), rename.Options{Root: root}, casing.Camel)

	if _, ok := errors.AsType[*rename.CollisionError](err); !ok {
		t.Fatalf("Build error = %v, want *rename.CollisionError", err)
	}
	assertTree(t, root, "A A A.txt", "aAA.txt")
}

func TestCollisionsInDifferentDirectoriesDoNotInterfere(t *testing.T) {
	root := makeTree(t, "a/My File.txt", "b/My File.txt")
	apply(t, rename.Options{Root: root, Recursive: true}, casing.Kebab)

	assertTree(t, root, "a/", "a/my-file.txt", "b/", "b/my-file.txt")
}

func TestAlreadyConvertedTreeProducesAnEmptyPlan(t *testing.T) {
	root := makeTree(t, "my-file.txt", "sub-dir/inner-file.txt")
	plan := build(t, rename.Options{Root: root, Recursive: true, IncludeDirs: true}, casing.Kebab)

	if !plan.IsEmpty() {
		t.Errorf("plan = %v, want empty", plan.Renames)
	}
	done, err := plan.Apply(t.Context(), nil)
	if err != nil || done != 0 {
		t.Errorf("Apply on an empty plan = (%d, %v), want (0, nil)", done, err)
	}
}

func TestSymlinksAreRenamedButNotFollowed(t *testing.T) {
	root := makeTree(t, "Real Dir/Inner File.txt")
	link := filepath.Join(root, "My Link")
	if err := os.Symlink(filepath.Join(root, "Real Dir"), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	apply(t, rename.Options{Root: root, Recursive: true}, casing.Kebab)

	assertTree(t, root, "Real Dir/", "Real Dir/inner-file.txt", "my-link")
}

func TestBuildRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		opts func(t *testing.T) rename.Options
	}{
		{"empty root", func(*testing.T) rename.Options { return rename.Options{Root: " "} }},
		{"missing root", func(t *testing.T) rename.Options {
			t.Helper()
			return rename.Options{Root: filepath.Join(t.TempDir(), "nope")}
		}},
		{"root is a file", func(t *testing.T) rename.Options {
			t.Helper()
			return rename.Options{Root: filepath.Join(makeTree(t, "file.txt"), "file.txt")}
		}},
		{"bad exclude pattern", func(t *testing.T) rename.Options {
			t.Helper()
			return rename.Options{Root: t.TempDir(), Exclude: []string{"["}}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := rename.Build(t.Context(), tc.opts(t), casing.Kebab); err == nil {
				t.Error("Build succeeded, want an error")
			}
		})
	}
}

func TestBuildRespectsContextCancellation(t *testing.T) {
	root := makeTree(t, "A File.txt", "Sub Dir/B File.txt")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := rename.Build(ctx, rename.Options{Root: root, Recursive: true}, casing.Kebab); !errors.Is(err, context.Canceled) {
		t.Errorf("Build error = %v, want context.Canceled", err)
	}
}

func TestApplyRespectsContextCancellation(t *testing.T) {
	root := makeTree(t, "A File.txt")
	plan := build(t, rename.Options{Root: root}, casing.Kebab)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	done, err := plan.Apply(ctx, nil)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Apply error = %v, want context.Canceled", err)
	}
	if done != 0 {
		t.Errorf("Apply completed %d renames after cancellation, want 0", done)
	}
	assertTree(t, root, "A File.txt")
}

func BenchmarkBuild(b *testing.B) {
	root := b.TempDir()
	for dir := range 20 {
		sub := filepath.Join(root, "Some Directory "+string(rune('a'+dir)))
		if err := os.MkdirAll(sub, 0o755); err != nil {
			b.Fatal(err)
		}
		for file := range 50 {
			name := filepath.Join(sub, "My Test File "+string(rune('a'+file%26))+".txt")
			if err := os.WriteFile(name, nil, 0o644); err != nil {
				b.Fatal(err)
			}
		}
	}
	opts := rename.Options{Root: root, Recursive: true, IncludeDirs: true}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := rename.Build(b.Context(), opts, casing.Kebab); err != nil {
			b.Fatal(err)
		}
	}
}

// TestRelativeRootForms locks the behaviour that broke a prefix-slicing
// shortcut: filepath.Join cleans "./" away, so paths under root "." carry no
// fixed prefix. Exclude patterns are path-relative and must still match.
func TestRelativeRootForms(t *testing.T) {
	for _, root := range []string{".", "./", "./."} {
		t.Run(root, func(t *testing.T) {
			dir := makeTree(t, "Keep Me.txt", "vendor/Skip Me.js", "vendor/Nested Dir/Deep.js")
			t.Chdir(dir)

			apply(t, rename.Options{
				Root: root, Recursive: true, IncludeDirs: true, Exclude: []string{"vendor/*"},
			}, casing.Kebab)

			assertTree(t, dir,
				"keep-me.txt",
				"vendor/", "vendor/Skip Me.js", "vendor/Nested Dir/", "vendor/Nested Dir/Deep.js",
			)
		})
	}
}

func TestRelativeRootSingleChar(t *testing.T) {
	dir := makeTree(t, "A", "b", "D/F.txt", "e/f.txt")
	t.Chdir(dir)

	apply(t, rename.Options{
		Root:        ".",
		Recursive:   true,
		IncludeDirs: true,
	}, casing.Kebab)

	assertTree(t, dir, "a", "b", "d/", "d/f.txt", "e/", "e/f.txt")
}

func TestTrailingSeparatorInRoot(t *testing.T) {
	root := makeTree(t, "Sub Dir/My File.txt")
	apply(t, rename.Options{Root: root + string(filepath.Separator), Recursive: true, IncludeDirs: true}, casing.Kebab)

	assertTree(t, root, "sub-dir/", "sub-dir/my-file.txt")
}

func TestExcludePatternMatchesNestedRelativePath(t *testing.T) {
	root := makeTree(t, "a/b/Skip Me.txt", "a/b/Keep Me.log", "Keep Me.txt")
	apply(t, rename.Options{Root: root, Recursive: true, Exclude: []string{"a/b/*.txt"}}, casing.Kebab)

	assertTree(t, root,
		"a/", "a/b/", "a/b/Skip Me.txt", "a/b/keep-me.log", "keep-me.txt",
	)
}

// excludeCase is one Exclude pattern applied recursively to tree with Kebab.
type excludeCase struct {
	name    string
	tree    []string
	exclude string
	want    []string
}

func runExcludeCases(t *testing.T, tests []excludeCase) {
	t.Helper()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := makeTree(t, tc.tree...)
			apply(t, rename.Options{Root: root, Recursive: true, Exclude: []string{tc.exclude}}, casing.Kebab)
			assertTree(t, root, tc.want...)
		})
	}
}

// Exclude patterns read like .gitignore: a leading or inner slash anchors the
// pattern to the target.
func TestExcludeAnchoredPatterns(t *testing.T) {
	runExcludeCases(t, []excludeCase{
		{
			name:    "dot slash anchors a path",
			tree:    []string{"Keep Me.txt", "vendor/Lib File.js", "Sub/vendor/Deep File.js"},
			exclude: "./vendor/*",
			want:    []string{"keep-me.txt", "vendor/", "vendor/Lib File.js", "Sub/", "Sub/vendor/", "Sub/vendor/deep-file.js"},
		},
		{
			name:    "leading slash anchors a path",
			tree:    []string{"Keep Me.txt", "vendor/Lib File.js", "Sub/vendor/Deep File.js"},
			exclude: "/vendor/*",
			want:    []string{"keep-me.txt", "vendor/", "vendor/Lib File.js", "Sub/", "Sub/vendor/", "Sub/vendor/deep-file.js"},
		},
		{
			name:    "dot slash anchors a base name glob",
			tree:    []string{"Keep Me.txt", "vendor/Nested File.txt"},
			exclude: "./*.txt",
			want:    []string{"Keep Me.txt", "vendor/", "vendor/nested-file.txt"},
		},
		{
			name:    "leading slash anchors a base name glob",
			tree:    []string{"Keep Me.txt", "vendor/Nested File.txt"},
			exclude: "/*.txt",
			want:    []string{"Keep Me.txt", "vendor/", "vendor/nested-file.txt"},
		},
		{
			name:    "repeated leading prefixes",
			tree:    []string{"Keep Me.txt", "vendor/Nested File.txt"},
			exclude: ".//./*.txt",
			want:    []string{"Keep Me.txt", "vendor/", "vendor/nested-file.txt"},
		},
	})
}

// A trailing slash matches directories only, as in .gitignore.
func TestExcludeDirectoryOnlyPatterns(t *testing.T) {
	runExcludeCases(t, []excludeCase{
		{
			name:    "trailing slash matches directories at any depth",
			tree:    []string{"My File.txt", "vendor/Lib File.js", "Sub/vendor/Deep File.js"},
			exclude: "vendor/",
			want:    []string{"my-file.txt", "vendor/", "vendor/Lib File.js", "Sub/", "Sub/vendor/", "Sub/vendor/Deep File.js"},
		},
		{
			name:    "trailing slash skips files of that name",
			tree:    []string{"Build Out/Log File.txt", "Sub/Build Out"},
			exclude: "Build Out/",
			want:    []string{"Build Out/", "Build Out/Log File.txt", "Sub/", "Sub/build-out"},
		},
		{
			name:    "anchored directory",
			tree:    []string{"vendor/Lib File.js", "Sub/vendor/Deep File.js"},
			exclude: "/vendor/",
			want:    []string{"vendor/", "vendor/Lib File.js", "Sub/", "Sub/vendor/", "Sub/vendor/deep-file.js"},
		},
		{
			name:    "nested directory path",
			tree:    []string{"vendor/Lib File.js", "Sub/vendor/Deep File.js"},
			exclude: "Sub/vendor/",
			want:    []string{"vendor/", "vendor/lib-file.js", "Sub/", "Sub/vendor/", "Sub/vendor/Deep File.js"},
		},
	})
}

func TestApplyStopsAtTheFirstFailureAndReportsProgress(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	// "Locked Dir" sorts after "A Dir" at the same depth, so one rename lands
	// before the unwritable directory stops the run.
	root := makeTree(t, "A Dir/Deep File.txt", "Locked Dir/Blocked File.txt")
	locked := filepath.Join(root, "Locked Dir")

	plan := build(t, rename.Options{Root: root, Recursive: true}, casing.Kebab)
	if err := os.Chmod(locked, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(locked, 0o755); err != nil {
			t.Errorf("restore permissions: %v", err)
		}
	})

	done, err := plan.Apply(t.Context(), nil)
	if err == nil {
		t.Fatal("Apply succeeded, want a permission error")
	}
	if !strings.Contains(err.Error(), "rename ") {
		t.Errorf("error %q does not name the failed rename", err)
	}
	if done != 1 {
		t.Errorf("Apply completed %d renames, want 1", done)
	}
	assertTree(t, root,
		"A Dir/", "A Dir/deep-file.txt", "Locked Dir/", "Locked Dir/Blocked File.txt",
	)
}

// os.Stat follows symlinks but filepath.WalkDir lstats its root, so without
// resolving first a symlinked target walked nothing and reported success.
func TestSymlinkedTargetIsResolved(t *testing.T) {
	parent := makeTree(t, "real/My File.txt", "real/Sub Dir/Inner File.txt")
	link := filepath.Join(parent, "link")
	if err := os.Symlink(filepath.Join(parent, "real"), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	renamed := apply(t, rename.Options{Root: link, Recursive: true, IncludeDirs: true}, casing.Kebab)
	if renamed != 3 {
		t.Errorf("renamed %d entries through the symlink, want 3", renamed)
	}
	assertTree(t, filepath.Join(parent, "real"),
		"my-file.txt", "sub-dir/", "sub-dir/inner-file.txt",
	)
}

func TestUndecodableNamesAreRenamedIntact(t *testing.T) {
	root := makeTree(t, "Caf\xe9 Menu.txt", "Na\xefve Report.md")
	apply(t, rename.Options{Root: root}, casing.Kebab)

	assertTree(t, root, "caf\xe9-menu.txt", "na\xefve-report.md")
}

func TestApplyReportsEachRenameAsItLands(t *testing.T) {
	root := makeTree(t, "A File.txt", "B File.txt")
	plan := build(t, rename.Options{Root: root}, casing.Kebab)

	var reported []string
	done, err := plan.Apply(t.Context(), func(r rename.Rename) {
		reported = append(reported, filepath.Base(r.To))
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if done != len(reported) {
		t.Errorf("Apply reported %d renames but completed %d", len(reported), done)
	}
	slices.Sort(reported)
	if want := []string{"a-file.txt", "b-file.txt"}; !slices.Equal(reported, want) {
		t.Errorf("reported %v, want %v", reported, want)
	}
}

func TestApplyStopsReportingAtTheFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	root := makeTree(t, "A Dir/Deep File.txt", "Locked Dir/Blocked File.txt")
	locked := filepath.Join(root, "Locked Dir")

	plan := build(t, rename.Options{Root: root, Recursive: true}, casing.Kebab)
	if err := os.Chmod(locked, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(locked, 0o755); err != nil {
			t.Errorf("restore permissions: %v", err)
		}
	})

	var reported int
	done, err := plan.Apply(t.Context(), func(rename.Rename) { reported++ })
	if err == nil {
		t.Fatal("Apply succeeded, want a permission error")
	}
	if reported != done {
		t.Errorf("reported %d renames but completed %d", reported, done)
	}
}

func TestCaseOnlyRenameIsNotAConflict(t *testing.T) {
	root := makeTree(t, "README.txt")
	apply(t, rename.Options{Root: root}, casing.Kebab)

	assertTree(t, root, "readme.txt")
}

// "Foo Bar.txt" converts to "foo-bar.txt" while "foo-bar.TXT" keeps its
// verbatim extension. The two targets differ only by case: distinct on a
// case-sensitive filesystem, the same file on a case-folding one.
func TestTargetsDifferingOnlyByCase(t *testing.T) {
	root := makeTree(t, "Foo Bar.txt", "foo-bar.TXT")
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 2 {
		t.Skip("filesystem folds case; both fixtures are one file")
	}

	_, err := rename.Build(t.Context(), rename.Options{Root: root}, casing.Kebab)
	if err != nil {
		t.Errorf("Build reported a conflict on a case-sensitive filesystem: %v", err)
	}
}

func TestIgnoresCaseIsFalseOnACaseSensitiveFilesystem(t *testing.T) {
	root := makeTree(t, "Mixed Case.txt")
	if _, err := os.Lstat(filepath.Join(root, "mIXED cASE.TXT")); err == nil {
		t.Skip("filesystem folds case")
	}
	// conflict must be found when folded, exact names agree.
	root = makeTree(t, "My File.txt", "my_file.txt")
	if _, err := rename.Build(t.Context(), rename.Options{Root: root}, casing.Kebab); err == nil {
		t.Error("Build missed an exact-name collision")
	}
}

func FuzzNoSilentDataLoss(f *testing.F) {
	for _, seed := range [][2]string{
		{"A B.txt", "AB.txt"},
		{"A A A.txt", "aAA.txt"},
		{"foo.txt", "bar.txt"},
		{"My File.txt", "my_file.txt"},
		{"1. Something.txt", "2. Other.txt"},
		{"part2beta.txt", "PART2BETA.txt"},
		{"a.txt", "b.txt"},
	} {
		f.Add(seed[0], seed[1])
	}

	f.Fuzz(func(t *testing.T, name1, name2 string) {
		if !isValidFilename(name1) || !isValidFilename(name2) || name1 == name2 {
			t.Skip("invalid or duplicate filenames")
		}
		root := t.TempDir()
		for _, conv := range casing.Conventions() {
			assertNoDataLoss(t, filepath.Join(root, string(conv)), conv, name1, name2)
		}
	})
}

func isValidFilename(name string) bool {
	return name != "" && len(name) <= 255 &&
		!strings.ContainsAny(name, "/\x00") &&
		strings.TrimLeft(name, ".") != "" &&
		utf8.ValidString(name)
}

// assertNoDataLoss renames two files in dir and checks both bodies survive,
// whether the plan applied or was refused.
func assertNoDataLoss(t *testing.T, dir string, conv casing.Convention, name1, name2 string) {
	t.Helper()
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	want, ok := writeDistinctFiles(dir, name1, name2)
	if !ok {
		return
	}

	plan, buildErr := rename.Build(t.Context(), rename.Options{Root: dir, IncludeHidden: true}, conv)
	if buildErr == nil {
		if _, err := plan.Apply(t.Context(), nil); err != nil {
			t.Fatalf("convention %s Apply failed: %v", conv, err)
		}
	}

	if got := fileBodies(t, dir); !slices.Equal(got, want) {
		t.Fatalf("silent data loss with convention %s for %q and %q: got bodies %v, want %v (plan: %+v, build err: %v)",
			conv, name1, name2, got, want, plan.Renames, buildErr)
	}
}

// writeDistinctFiles writes one body per name and returns the sorted bodies;
// ok is false when the filesystem cannot hold both names as separate files.
func writeDistinctFiles(dir string, names ...string) (bodies []string, ok bool) {
	for i, name := range names {
		body := fmt.Sprintf("body%d:%s", i+1, name)
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			return nil, false
		}
		bodies = append(bodies, body)
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != len(names) {
		return nil, false
	}
	slices.Sort(bodies)
	return bodies, true
}

// fileBodies returns the sorted contents of the files directly in dir.
func fileBodies(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s after rename: %v", dir, err)
	}
	bodies := make([]string, 0, len(entries))
	for _, e := range entries {
		content, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s after rename: %v", e.Name(), err)
		}
		bodies = append(bodies, string(content))
	}
	slices.Sort(bodies)
	return bodies
}

func TestLowerExtensionRenamesExtensionOnlyChanges(t *testing.T) {
	root := makeTree(t, "My File.TXT", "notes.MD", "kept.txt", "Sub Dir.D/Inner.JSON")

	apply(t, rename.Options{Root: root, Recursive: true, IncludeDirs: true}, casing.LowerExtension{Convention: casing.Kebab})

	assertTree(t, root, "my-file.txt", "notes.md", "kept.txt", "sub-dir-d/", "sub-dir-d/inner.json")
}

func TestLowerExtensionCollisionAbortsBeforeAnythingIsRenamed(t *testing.T) {
	root := makeTree(t, "report.PDF", "report.pdf")
	before := snapshot(t, root)

	_, err := rename.Build(t.Context(), rename.Options{Root: root}, casing.LowerExtension{Convention: casing.Kebab})

	if _, ok := errors.AsType[*rename.CollisionError](err); !ok {
		t.Fatalf("Build error = %v, want *rename.CollisionError", err)
	}
	assertTree(t, root, before...)
}
