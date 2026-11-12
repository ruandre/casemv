package rename

import (
	"cmp"
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"unicode/utf8"
)

// maxNameRunes is a limit every common filesystem enforces: most Linux ones
// allow 255 bytes, Windows and macOS ones 255 UTF-16 code units, and a name of
// more than 255 runes exceeds both. A shorter name can still be too long in
// bytes; its rename then fails when it runs.
const maxNameRunes = 255

// Namer gives the name an entry should have; casing.Convention and
// casing.LowerExtension implement it.
type Namer interface {
	Format(name string, isDir bool) string
}

// Rename is one planned rename. Both paths share a parent directory.
type Rename struct {
	From string
	To   string
}

// Plan is an ordered set of renames under Root, the resolved target directory.
type Plan struct {
	Root    string
	Renames []Rename
}

// IsEmpty reports whether the plan would rename nothing.
func (p Plan) IsEmpty() bool { return len(p.Renames) == 0 }

// Build scans the target tree and returns the renames needed to bring every
// eligible name to the one namer gives. Deeper entries are ordered first, so a directory is
// renamed only after its contents and every recorded source path is still valid
// when its turn comes.
//
// Target-name collisions are detected up front: Build returns *CollisionError
// and an empty plan rather than renaming part of the tree. A new name longer
// than maxNameRunes fails the plan the same way.
func Build(ctx context.Context, opts Options, namer Namer) (Plan, error) {
	if err := opts.validate(); err != nil {
		return Plan{}, err
	}
	t, err := scan(ctx, opts)
	if err != nil {
		return Plan{}, err
	}

	changed := make(map[string]string, len(t.entries))
	pending := make([]entry, 0, len(t.entries))
	for _, e := range t.entries {
		newName := namer.Format(e.name, e.isDir)
		if newName == e.name {
			continue
		}
		changed[e.path] = newName
		pending = append(pending, e)
	}
	if err := checkNameLengths(pending, changed); err != nil {
		return Plan{}, err
	}
	if err := checkCollisions(t, changed); err != nil {
		return Plan{}, err
	}

	slices.SortFunc(pending, deepestFirst)
	renames := make([]Rename, len(pending))
	for i, e := range pending {
		renames[i] = Rename{From: e.path, To: filepath.Join(e.dir, changed[e.path])}
	}
	return Plan{Root: t.root, Renames: renames}, nil
}

func checkNameLengths(pending []entry, changed map[string]string) error {
	for _, e := range pending {
		if n := utf8.RuneCountInString(changed[e.path]); n > maxNameRunes {
			return fmt.Errorf("new name for %s would be %d characters, over the limit of %d; nothing was renamed",
				e.path, n, maxNameRunes)
		}
	}
	return nil
}

func deepestFirst(a, b entry) int {
	if d := cmp.Compare(b.depth, a.depth); d != 0 {
		return d
	}
	if d := cmp.Compare(a.dir, b.dir); d != 0 {
		return d
	}
	return cmp.Compare(a.name, b.name)
}
