package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ruandre/casemv/internal/rename"
)

// displayPath quotes a name a terminal could misread: control runes would
// drive it, newlines would fake output lines, and invalid bytes would blur.
func displayPath(path string) string {
	for _, r := range path {
		if r == utf8.RuneError || !strconv.IsPrint(r) {
			return strconv.Quote(path)
		}
	}
	return path
}

// errorText renders err for stderr. Only an unwrapped collision report keeps
// its line breaks, and is printed as is: CollisionError.Error quotes every
// variable field with %q, so its newlines only separate conflicts, and
// escaping again would double its backslashes. Text wrapping it may hold raw
// paths, so a wrapped one is escaped whole.
func errorText(err error) string {
	collision, ok := errors.AsType[*rename.CollisionError](err)
	if !ok || err.Error() != collision.Error() {
		return terminalSafe(err.Error())
	}
	return collision.Error()
}

// terminalSafe escapes what displayPath would quote, and backslashes, so an
// escape it writes cannot be confused with one already in the text.
func terminalSafe(text string) string {
	var b strings.Builder
	for i := 0; i < len(text); {
		r, width := utf8.DecodeRuneInString(text[i:])
		switch {
		case r == utf8.RuneError && width == 1:
			fmt.Fprintf(&b, `\x%02x`, text[i])
		case r == '\\':
			b.WriteString(`\\`)
		case strconv.IsPrint(r):
			b.WriteString(text[i : i+width])
		default:
			b.WriteString(strings.Trim(strconv.QuoteRune(r), "'"))
		}
		i += width
	}
	return b.String()
}
