// Package rename plans and applies naming-convention renames over a directory tree.
package rename

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// Options selects which entries a scan visits and which of them may be renamed.
type Options struct {
	Root          string
	Recursive     bool
	IncludeDirs   bool
	IncludeHidden bool
	// Exclude holds glob patterns in path.Match syntax, anchored as .gitignore
	// anchors lines. A pattern with a leading or inner slash matches the
	// entry's slash-separated path relative to Root; otherwise it matches the
	// base name at any depth. A trailing slash matches directories only.
	// Unlike .gitignore, "**" is rejected and "!" negates nothing.
	Exclude []string
}

func (o Options) validate() error {
	if strings.TrimSpace(o.Root) == "" {
		return errors.New("target directory is empty")
	}
	for _, pattern := range o.Exclude {
		glob := parseExclude(pattern).glob
		if strings.Contains(glob, "**") {
			// Not %q: the CLI escapes error text for the terminal, and would escape again.
			return fmt.Errorf(`invalid exclude pattern "%s": ** is not supported; * matches within one path segment`, pattern)
		}
		if _, err := path.Match(glob, "x"); err != nil {
			return fmt.Errorf(`invalid exclude pattern "%s": %w`, pattern, err)
		}
	}
	return nil
}

// isHidden reports Unix dot-prefix convention.
func isHidden(name string) bool { return strings.HasPrefix(name, ".") }

// excludePattern is one Exclude entry, parsed.
type excludePattern struct {
	glob     string
	anchored bool
	dirOnly  bool
}

// parseExclude decides anchoring before it strips leading "./" and "/": those
// prefixes are what anchor "/*.txt" to the target root. On Windows a
// backslash separates; elsewhere path.Match reads it as an escape.
func parseExclude(pattern string) excludePattern {
	pattern = filepath.ToSlash(pattern)
	dirOnly := strings.HasSuffix(pattern, "/")
	glob := strings.TrimRight(pattern, "/")
	anchored := strings.Contains(glob, "/")
	for {
		stripped := strings.TrimPrefix(strings.TrimPrefix(glob, "./"), "/")
		if stripped == glob {
			break
		}
		glob = stripped
	}
	return excludePattern{glob: glob, anchored: anchored, dirOnly: dirOnly}
}

func parseExcludes(patterns []string) []excludePattern {
	parsed := make([]excludePattern, len(patterns))
	for i, pattern := range patterns {
		parsed[i] = parseExclude(pattern)
	}
	return parsed
}

// matches takes relPath slash-separated.
func (p excludePattern) matches(relPath, name string, isDir bool) bool {
	if p.glob == "" || (p.dirOnly && !isDir) {
		return false
	}
	subject := name
	if p.anchored {
		subject = relPath
	}
	//nolint:errcheck // validate rejects malformed patterns before a scan starts
	ok, _ := path.Match(p.glob, subject)
	return ok
}
