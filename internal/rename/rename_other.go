//go:build !linux

package rename

import (
	"io/fs"
	"os"
	"path/filepath"
)

// targetDir renames entries beneath the target with check-then-rename: only
// Linux gets the atomic RENAME_NOREPLACE guarantee.
type targetDir struct {
	root *os.Root
}

func openTargetDir(path string) (*targetDir, error) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	return &targetDir{root: root}, nil
}

func (t *targetDir) close() error { return t.root.Close() }

func (t *targetDir) rename(dir, from, to string) error {
	from, to = filepath.Join(dir, from), filepath.Join(dir, to)
	if target, err := t.root.Lstat(to); err == nil {
		source, err := t.root.Lstat(from)
		if err != nil {
			return err
		}
		if !os.SameFile(source, target) {
			return fs.ErrExist
		}
	}
	return t.root.Rename(from, to)
}
