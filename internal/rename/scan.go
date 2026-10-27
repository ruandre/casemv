package rename

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// entry is walked filesystem entry eligible for rename.
type entry struct {
	path  string
	dir   string
	name  string
	isDir bool
	depth int
}

// tree is the result of walking the target directory: the entries that may be
// renamed, plus every child name of each directory that contains one. The
// sibling names are what collision detection compares against, so they include
// entries that were skipped and will keep their current name.
type tree struct {
	root     string
	entries  []entry
	siblings map[string][]string
}

func scan(ctx context.Context, opts Options) (tree, error) {
	root, err := resolveRoot(opts.Root)
	if err != nil {
		return tree{}, err
	}
	w := walker{
		opts:         opts,
		matchesPaths: slices.ContainsFunc(opts.Exclude, func(p string) bool { return strings.ContainsRune(p, '/') }),
		tree:         tree{root: root, siblings: make(map[string][]string)},
	}
	walk := func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", p, err)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return w.visit(p, d)
	}
	if err := filepath.WalkDir(root, walk); err != nil {
		return tree{}, err
	}
	return w.tree, nil
}

// resolveRoot checks target is a directory and resolves symlinks: os.Stat
// follows them but filepath.WalkDir lstats its root, so a symlinked target
// would walk nothing at all.
func resolveRoot(target string) (string, error) {
	info, err := os.Stat(target)
	if err != nil {
		return "", fmt.Errorf("read target directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("target %s is not a directory", target)
	}
	root, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", fmt.Errorf("resolve target directory %s: %w", target, err)
	}
	return root, nil
}

// walker collects the tree while filepath.WalkDir visits each entry.
type walker struct {
	opts Options
	// matchesPaths is set when an exclude pattern holds a slash, so relative
	// paths are worth computing for every entry.
	matchesPaths bool
	tree         tree
}

func (w *walker) visit(p string, d fs.DirEntry) error {
	if p == w.tree.root {
		return nil
	}
	name, dir := d.Name(), filepath.Dir(p)
	w.tree.siblings[dir] = append(w.tree.siblings[dir], name)

	if isHidden(name) && !w.opts.IncludeHidden {
		return skipSubtree(d)
	}
	rel, err := filepath.Rel(w.tree.root, p)
	if err != nil {
		return fmt.Errorf("resolve %s relative to %s: %w", p, w.tree.root, err)
	}
	if w.excludes(rel, name) {
		return skipSubtree(d)
	}

	if !d.IsDir() || w.opts.IncludeDirs {
		w.tree.entries = append(w.tree.entries, entry{
			path:  p,
			dir:   dir,
			name:  name,
			isDir: d.IsDir(),
			depth: strings.Count(rel, string(filepath.Separator)),
		})
	}
	if d.IsDir() && !w.opts.Recursive {
		return fs.SkipDir
	}
	return nil
}

func (w *walker) excludes(rel, name string) bool {
	if len(w.opts.Exclude) == 0 {
		return false
	}
	var matchRel string
	if w.matchesPaths {
		matchRel = filepath.ToSlash(rel)
	}
	return w.opts.excludes(matchRel, name)
}

// skipSubtree stops walk from descending into skipped directory.
func skipSubtree(d fs.DirEntry) error {
	if d.IsDir() {
		return fs.SkipDir
	}
	return nil
}
