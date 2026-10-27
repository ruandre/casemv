// Package rename plans and applies naming-convention renames over a directory tree.
package rename

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

// Options selects which entries a scan visits and which of them may be renamed.
type Options struct {
	Root          string
	Recursive     bool
	IncludeDirs   bool
	IncludeHidden bool
	// Exclude holds glob patterns in path.Match syntax. A pattern containing a
	// slash is matched against the entry's slash-separated path relative to
	// Root; otherwise it is matched against the base name.
	Exclude []string
}

func (o Options) validate() error {
	if strings.TrimSpace(o.Root) == "" {
		return errors.New("target directory is empty")
	}
	for _, pattern := range o.Exclude {
		if _, err := path.Match(normalizePattern(pattern), "x"); err != nil {
			return fmt.Errorf("invalid exclude pattern %q: %w", pattern, err)
		}
	}
	return nil
}

// isHidden reports Unix dot-prefix convention.
func isHidden(name string) bool { return strings.HasPrefix(name, ".") }

func (o Options) excludes(relPath, name string) bool {
	for _, pattern := range o.Exclude {
		pattern = normalizePattern(pattern)
		if pattern == "" {
			continue
		}
		subject := name
		if strings.ContainsRune(pattern, '/') {
			if relPath == "" {
				continue
			}
			subject = relPath
		}
		//nolint:errcheck // validate rejects malformed patterns before a scan starts
		if ok, _ := path.Match(pattern, subject); ok {
			return true
		}
	}
	return false
}

func normalizePattern(pattern string) string {
	pattern = strings.TrimPrefix(pattern, "./")
	return strings.TrimPrefix(pattern, "/")
}
