package rename

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// targetDir renames entries beneath the open target directory.
type targetDir struct {
	fd int
	// root serves kernels before 5.6, which lack openat2.
	root *os.Root
}

func openTargetDir(path string) (*targetDir, error) {
	fd, err := unix.Open(path, unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, errors.Join(err, unix.Close(fd))
	}
	return &targetDir{fd: fd, root: root}, nil
}

func (t *targetDir) close() error { return errors.Join(unix.Close(t.fd), t.root.Close()) }

// rename gives entry from the new name to inside dir. Dir resolves beneath the
// target in one openat2 call that refuses symlinks, so a link swapped in after
// planning cannot redirect the rename, and the cost does not grow with depth.
func (t *targetDir) rename(dir, from, to string) error {
	dirfd, err := unix.Openat2(t.fd, dir, &unix.OpenHow{
		Flags:   unix.O_PATH | unix.O_DIRECTORY | unix.O_CLOEXEC,
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS,
	})
	if errors.Is(err, unix.ENOSYS) {
		return t.renameThroughRoot(dir, from, to)
	}
	if err != nil {
		return err
	}
	//nolint:errcheck // O_PATH handle; closing it cannot lose data
	defer unix.Close(dirfd)
	return renameAt(dirfd, from, to)
}

// renameThroughRoot keeps renames inside the target, but follows symlinks
// that stay inside it, and resolves dir one component at a time.
func (t *targetDir) renameThroughRoot(dir, from, to string) error {
	handle, err := t.root.Open(dir)
	if err != nil {
		return err
	}
	//nolint:errcheck // read-only directory handle; closing it cannot lose data
	defer handle.Close()

	conn, err := handle.SyscallConn()
	if err != nil {
		return err
	}
	var renameErr error
	if err := conn.Control(func(fd uintptr) { renameErr = renameAt(int(fd), from, to) }); err != nil {
		return err
	}
	return renameErr
}

func renameAt(dirfd int, from, to string) error {
	err := unix.Renameat2(dirfd, from, dirfd, to, unix.RENAME_NOREPLACE)
	switch {
	case errors.Is(err, unix.EEXIST) && sameEntry(dirfd, from, to):
		return renameThroughTemporary(dirfd, from, to)
	case errors.Is(err, unix.EINVAL), errors.Is(err, unix.ENOSYS):
		return renameAfterCheck(dirfd, from, to)
	}
	return err
}

// renameThroughTemporary handles a case-only rename on a case-folding
// directory, where both names are one entry: rename(2) would succeed without
// changing the stored name, and RENAME_NOREPLACE refuses it.
func renameThroughTemporary(dirfd int, from, to string) error {
	temporary := ".casemv-" + rand.Text()
	if err := unix.Renameat2(dirfd, from, dirfd, temporary, unix.RENAME_NOREPLACE); err != nil {
		return err
	}
	err := unix.Renameat2(dirfd, temporary, dirfd, to, unix.RENAME_NOREPLACE)
	if err == nil {
		return nil
	}
	if undoErr := unix.Renameat2(dirfd, temporary, dirfd, from, unix.RENAME_NOREPLACE); undoErr != nil {
		return fmt.Errorf("%w; entry left as %s: %w", err, temporary, undoErr)
	}
	return err
}

// renameAfterCheck serves filesystems without RENAME_NOREPLACE; the check
// narrows the replace window to a few syscalls rather than closing it.
func renameAfterCheck(dirfd int, from, to string) error {
	var st unix.Stat_t
	if err := unix.Fstatat(dirfd, to, &st, unix.AT_SYMLINK_NOFOLLOW); err == nil && !sameEntry(dirfd, from, to) {
		return unix.EEXIST
	}
	return unix.Renameat(dirfd, from, dirfd, to)
}

func sameEntry(dirfd int, a, b string) bool {
	var stA, stB unix.Stat_t
	if unix.Fstatat(dirfd, a, &stA, unix.AT_SYMLINK_NOFOLLOW) != nil ||
		unix.Fstatat(dirfd, b, &stB, unix.AT_SYMLINK_NOFOLLOW) != nil {
		return false
	}
	return stA.Dev == stB.Dev && stA.Ino == stB.Ino
}
