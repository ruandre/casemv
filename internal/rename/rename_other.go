//go:build !linux

package rename

import (
	"os"
	"path/filepath"
)

// targetDir renames entries beneath the target through rootRenamer: only Linux
// gets the atomic RENAME_NOREPLACE and openat2 guarantees.
type targetDir struct {
	renamer rootRenamer
}

func openTargetDir(path string) (*targetDir, error) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	return &targetDir{renamer: rootRenamer{root: root}}, nil
}

func (t *targetDir) close() error { return t.renamer.root.Close() }

func (t *targetDir) rename(dir, from, to string) error {
	return t.renamer.rename(filepath.ToSlash(dir), from, to)
}
