package rename

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

const dirHandleFlags = unix.O_PATH | unix.O_DIRECTORY | unix.O_CLOEXEC

// targetDir renames entries beneath the open target directory.
type targetDir struct {
	fd int
}

func openTargetDir(path string) (*targetDir, error) {
	fd, err := unix.Open(path, dirHandleFlags, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return &targetDir{fd: fd}, nil
}

func (t *targetDir) close() error { return unix.Close(t.fd) }

// rename gives entry from the new name to inside dir. Dir resolves beneath the
// target in one openat2 call that refuses symlinks, so a link swapped in after
// planning cannot redirect the rename, and the cost does not grow with depth.
func (t *targetDir) rename(dir, from, to string) error {
	dirfd, err := unix.Openat2(t.fd, dir, &unix.OpenHow{
		Flags:   dirHandleFlags,
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS,
	})
	if openat2Unavailable(err) {
		return t.renameWithoutOpenat2(dir, from, to)
	}
	if err != nil {
		return err
	}
	//nolint:errcheck // O_PATH handle; closing it cannot lose data
	defer unix.Close(dirfd)
	return renameAt(dirfd, from, to)
}

// openat2Unavailable reports kernels before 5.6 (ENOSYS) and seccomp filters
// that deny unknown syscalls with EPERM, as older container runtimes do. A
// genuine EPERM recurs on the fallback path, so it is still reported.
func openat2Unavailable(err error) bool {
	return errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EPERM)
}

// renameWithoutOpenat2 gives the openat2 guarantee by resolving dir one
// component at a time, each opened with O_NOFOLLOW; the cost grows with depth.
func (t *targetDir) renameWithoutOpenat2(dir, from, to string) error {
	dirfd, err := openEachComponent(t.fd, dir)
	if err != nil {
		return err
	}
	//nolint:errcheck // O_PATH handle; closing it cannot lose data
	defer unix.Close(dirfd)
	return renameAt(dirfd, from, to)
}

// openEachComponent opens dir, a clean path relative to rootfd, refusing ".."
// with EXDEV as RESOLVE_BENEATH does.
func openEachComponent(rootfd int, dir string) (int, error) {
	fd, err := unix.Openat(rootfd, ".", dirHandleFlags, 0)
	if err != nil || dir == "." {
		return fd, err
	}
	for component := range strings.SplitSeq(dir, "/") {
		next := -1
		if component == ".." {
			err = unix.EXDEV
		} else {
			next, err = unix.Openat(fd, component, dirHandleFlags|unix.O_NOFOLLOW, 0)
		}
		//nolint:errcheck,gosec // O_PATH handle; closing it cannot lose data
		unix.Close(fd)
		if err != nil {
			return -1, err
		}
		fd = next
	}
	return fd, nil
}

func renameAt(dirfd int, from, to string) error {
	err := renameWith(dirfd, from, to, renameNoReplace)
	if errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOSYS) {
		return renameAfterCheck(dirfd, from, to)
	}
	return err
}

// renameAfterCheck serves filesystems without RENAME_NOREPLACE.
func renameAfterCheck(dirfd int, from, to string) error {
	return renameWith(dirfd, from, to, renameIfAbsent)
}

// move renames an entry within dirfd, failing with EEXIST when the new name exists.
type move func(dirfd int, from, to string) error

func renameNoReplace(dirfd int, from, to string) error {
	return unix.Renameat2(dirfd, from, dirfd, to, unix.RENAME_NOREPLACE)
}

// renameIfAbsent narrows the replace window to a few syscalls rather than
// closing it. Only ENOENT shows that to is absent: after any other stat
// failure, such as EIO, rename(2) could still replace an entry that is there.
func renameIfAbsent(dirfd int, from, to string) error {
	var st unix.Stat_t
	err := unix.Fstatat(dirfd, to, &st, unix.AT_SYMLINK_NOFOLLOW)
	if err == nil {
		return unix.EEXIST
	}
	if !errors.Is(err, unix.ENOENT) {
		return err
	}
	return unix.Renameat(dirfd, from, dirfd, to)
}

// renameWith renames through a temporary name when from and to are one entry:
// rename(2) between them succeeds without changing anything.
func renameWith(dirfd int, from, to string, m move) error {
	err := m(dirfd, from, to)
	if errors.Is(err, unix.EEXIST) && sameEntry(dirfd, from, to) {
		return renameThroughTemporary(dirfd, from, to, m)
	}
	return err
}

// renameThroughTemporary changes case on a case-folding directory, where both
// names are one entry. Once from moves away, to is free unless it is a second
// hard link to the same file, which m then refuses.
func renameThroughTemporary(dirfd int, from, to string, m move) error {
	temporary := ".casemv-" + rand.Text()
	if err := m(dirfd, from, temporary); err != nil {
		return err
	}
	err := m(dirfd, temporary, to)
	if err == nil {
		return nil
	}
	if undoErr := m(dirfd, temporary, from); undoErr != nil {
		return fmt.Errorf("%w; entry left as %s: %w", err, temporary, undoErr)
	}
	return err
}

func sameEntry(dirfd int, a, b string) bool {
	var stA, stB unix.Stat_t
	if unix.Fstatat(dirfd, a, &stA, unix.AT_SYMLINK_NOFOLLOW) != nil ||
		unix.Fstatat(dirfd, b, &stB, unix.AT_SYMLINK_NOFOLLOW) != nil {
		return false
	}
	return stA.Dev == stB.Dev && stA.Ino == stB.Ino
}
