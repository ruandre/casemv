package rename

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"
)

// errNotDirectory reports a directory replaced after planning, for instance by
// a symlink or a Windows junction.
var errNotDirectory = errors.New("not a directory")

// rootRenamer renames beneath an os.Root by checking, then renaming: unlike
// RENAME_NOREPLACE and openat2 on Linux, a change racing between the two
// steps can still replace an entry or redirect through a symlink.
type rootRenamer struct {
	root *os.Root
}

// rename gives entry from the name to inside dir, a clean slash-separated
// path relative to the root.
func (r rootRenamer) rename(dir, from, to string) error {
	if err := r.checkRealDirs(dir); err != nil {
		return err
	}
	from, to = path.Join(dir, from), path.Join(dir, to)
	target, err := r.root.Lstat(to)
	if errors.Is(err, fs.ErrNotExist) {
		return r.root.Rename(from, to)
	}
	if err != nil {
		return err
	}
	source, err := r.root.Lstat(from)
	if err != nil {
		return err
	}
	if !os.SameFile(source, target) {
		return fs.ErrExist
	}
	return r.renameThroughTemporary(from, to)
}

// checkRealDirs requires every component of dir to be a directory, not a link
// to one: os.Root keeps dir beneath the root, but allows ".." and follows
// symlinks that stay inside it.
func (r rootRenamer) checkRealDirs(dir string) error {
	if dir == "." {
		return nil
	}
	prefix := ""
	for component := range strings.SplitSeq(dir, "/") {
		if component == ".." {
			return fmt.Errorf("%s: parent component in path", dir)
		}
		prefix = path.Join(prefix, component)
		info, err := r.root.Lstat(prefix)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("%s: %w", prefix, errNotDirectory)
		}
	}
	return nil
}

// renameThroughTemporary changes case on a case-folding directory, where both
// names are one entry and rename(2) between them may change nothing. Once from
// moves away, to is free unless it is a second hard link to the same file.
func (r rootRenamer) renameThroughTemporary(from, to string) error {
	temporary := path.Join(path.Dir(from), ".casemv-"+rand.Text())
	if err := r.root.Rename(from, temporary); err != nil {
		return err
	}
	err := r.renameIfAbsent(temporary, to)
	if err == nil {
		return nil
	}
	if undoErr := r.root.Rename(temporary, from); undoErr != nil {
		return fmt.Errorf("%w; entry left as %s: %w", err, temporary, undoErr)
	}
	return err
}

// renameIfAbsent renames from only when Lstat reports the new name as missing,
// and returns any other Lstat failure as is.
func (r rootRenamer) renameIfAbsent(from, to string) error {
	_, err := r.root.Lstat(to)
	if err == nil {
		return fs.ErrExist
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return r.root.Rename(from, to)
}
