package rename

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// Conflict is a name in Dir that more than one entry would claim.
type Conflict struct {
	Dir     string
	Name    string
	Sources []string
}

// CollisionError reports conflicts found during planning. It is returned before
// any rename is attempted, so the tree is untouched.
type CollisionError struct {
	Conflicts []Conflict
}

func (e *CollisionError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d name collision(s) would be created; nothing was renamed:", len(e.Conflicts))
	for _, c := range e.Conflicts {
		quoted := make([]string, len(c.Sources))
		for i, source := range c.Sources {
			quoted[i] = strconv.Quote(source)
		}
		fmt.Fprintf(&b, "\n  %s: %q claimed by %s", c.Dir, c.Name, strings.Join(quoted, ", "))
	}
	return b.String()
}

// claim tracks entries claiming target name.
type claim struct {
	name    string
	sources []string
}

// checkCollisions compares the post-rename name of every child of every
// affected directory, including children that keep their current name.
func checkCollisions(t tree, changed map[string]string) error {
	if len(changed) == 0 {
		return nil
	}

	affectedDirs := make(map[string]struct{}, len(changed))
	for p := range changed {
		affectedDirs[filepath.Dir(p)] = struct{}{}
	}

	var conflicts []Conflict
	for dir := range affectedDirs {
		names, ok := t.siblings[dir]
		if !ok {
			continue
		}
		// Detected per directory, not per platform: a Linux host still mounts
		// case-folding filesystems (vfat, exfat, ntfs, cifs, ext4 casefold),
		// and there os.Rename replaces a destination differing only by case.
		found := conflictsIn(dir, names, changed, ignoresCase(dir, names))
		if len(found) == 0 {
			found = occupiedTargets(dir, names, changed)
		}
		conflicts = append(conflicts, found...)
	}
	if len(conflicts) == 0 {
		return nil
	}
	slices.SortFunc(conflicts, func(a, b Conflict) int {
		if d := cmp.Compare(a.Dir, b.Dir); d != 0 {
			return d
		}
		return cmp.Compare(a.Name, b.Name)
	})
	return &CollisionError{Conflicts: conflicts}
}

func conflictsIn(dir string, names []string, changed map[string]string, foldCase bool) []Conflict {
	claims := newDirClaims(len(names), foldCase)
	affected := false
	for _, name := range names {
		newName, isChanged := changed[filepath.Join(dir, name)]
		affected = affected || isChanged
		final := name
		if isChanged {
			final = newName
		}
		claims.add(name, final)
	}
	if !affected {
		return nil
	}
	for _, name := range names {
		if newName, ok := changed[filepath.Join(dir, name)]; ok && newName != name {
			claims.addOccupants(name, newName)
		}
	}
	return claims.conflicts(dir)
}

// dirClaims groups one directory's entries by the name each ends up with,
// compared under key.
type dirClaims struct {
	key    func(string) string
	byName map[string]*claim
	// occupants holds current names by key, to find entries a rename lands on.
	occupants map[string][]string
}

func newDirClaims(size int, foldCase bool) dirClaims {
	key := func(name string) string { return name }
	if foldCase {
		key = foldKey
	}
	return dirClaims{
		key:       key,
		byName:    make(map[string]*claim, size),
		occupants: make(map[string][]string, size),
	}
}

func (d dirClaims) add(name, final string) {
	current := d.key(name)
	d.occupants[current] = append(d.occupants[current], name)

	target := d.key(final)
	if c, ok := d.byName[target]; ok {
		c.sources = append(c.sources, name)
		return
	}
	d.byName[target] = &claim{name: final, sources: []string{name}}
}

// addOccupants makes every other entry currently holding newName a claimant:
// renaming name onto it would replace it even if it moves away later.
func (d dirClaims) addOccupants(name, newName string) {
	target := d.key(newName)
	c := d.byName[target]
	for _, occupant := range d.occupants[target] {
		if occupant != name && !slices.Contains(c.sources, occupant) {
			c.sources = append(c.sources, occupant)
		}
	}
}

func (d dirClaims) conflicts(dir string) []Conflict {
	var conflicts []Conflict
	for _, c := range d.byName {
		if len(c.sources) == 1 {
			continue
		}
		slices.Sort(c.sources)
		conflicts = append(conflicts, Conflict{Dir: dir, Name: c.name, Sources: c.sources})
	}
	slices.SortFunc(conflicts, func(a, b Conflict) int {
		return cmp.Compare(a.Name, b.Name)
	})
	return conflicts
}

// foldKey approximates the kernel's casefold: NFD plus full case folding.
// Undecodable names compare byte for byte there, so they stay as they are.
func foldKey(name string) string {
	if !utf8.ValidString(name) {
		return name
	}
	return norm.NFD.String(cases.Fold().String(norm.NFD.String(name)))
}

// occupiedTargets asks the filesystem whether a target name already resolves
// to another entry, catching folding rules foldKey does not model.
func occupiedTargets(dir string, names []string, changed map[string]string) []Conflict {
	var conflicts []Conflict
	for _, name := range names {
		newName, ok := changed[filepath.Join(dir, name)]
		if !ok {
			continue
		}
		occupant, ok := occupantOf(dir, name, newName, names)
		if !ok {
			continue
		}
		sources := []string{name, occupant}
		slices.Sort(sources)
		conflicts = append(conflicts, Conflict{Dir: dir, Name: newName, Sources: sources})
	}
	return conflicts
}

// occupantOf returns the sibling that newName resolves to, when that is not
// name itself; a case-only rename on a folding directory resolves to itself.
func occupantOf(dir, name, newName string, names []string) (string, bool) {
	target, err := os.Lstat(filepath.Join(dir, newName))
	if err != nil {
		return "", false
	}
	source, err := os.Lstat(filepath.Join(dir, name))
	if err != nil || os.SameFile(source, target) {
		return "", false
	}
	for _, sibling := range names {
		if info, err := os.Lstat(filepath.Join(dir, sibling)); err == nil && os.SameFile(info, target) {
			return sibling, true
		}
	}
	return newName, true
}

// ignoresCase probes dir's filesystem with entries that already exist: if a
// case-flipped name resolves to the same file, the filesystem folds case.
func ignoresCase(dir string, names []string) bool {
	for _, name := range names {
		// strings.Map would turn invalid bytes into U+FFFD: a name that cannot exist.
		if !utf8.ValidString(name) {
			continue
		}
		if folds, known := probeCase(dir, name); known {
			return folds
		}
	}
	return false
}

// probeCase flips only ASCII letters when name has any: every case-folding
// filesystem folds those, so a missing flipped name proves case sensitivity.
// Folding beyond ASCII varies (vfat folds no accents, the kernel's casefold
// tables lag Unicode), so a missing non-ASCII flip proves nothing.
func probeCase(dir, name string) (folds, known bool) {
	flipped, isASCIIProbe := flipASCIICase(name), true
	if flipped == name {
		flipped, isASCIIProbe = flipCase(name), false
	}
	if flipped == name {
		return false, false
	}
	original, err := os.Lstat(filepath.Join(dir, name))
	if err != nil {
		return false, false
	}
	other, err := os.Lstat(filepath.Join(dir, flipped))
	if err != nil {
		return false, isASCIIProbe && errors.Is(err, os.ErrNotExist)
	}
	return os.SameFile(original, other), true
}

func flipASCIICase(name string) string {
	b := []byte(name)
	for i, c := range b {
		switch {
		case 'a' <= c && c <= 'z':
			b[i] = c - 'a' + 'A'
		case 'A' <= c && c <= 'Z':
			b[i] = c - 'A' + 'a'
		}
	}
	return string(b)
}

// flipCase treats a rune as uppercase when lowercasing changes it, which
// covers titlecase ǅ and Other_Uppercase Ⅶ that unicode.IsUpper misses.
func flipCase(name string) string {
	return strings.Map(func(r rune) rune {
		if lower := unicode.ToLower(r); lower != r {
			return lower
		}
		return unicode.ToUpper(r)
	}, name)
}
