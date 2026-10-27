package cli

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
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

// terminalSafe escapes what displayPath would quote, keeping the newlines
// that structure multi-line errors.
func terminalSafe(text string) string {
	var b strings.Builder
	for i := 0; i < len(text); {
		r, width := utf8.DecodeRuneInString(text[i:])
		switch {
		case r == utf8.RuneError && width == 1:
			fmt.Fprintf(&b, `\x%02x`, text[i])
		case r == '\n' || strconv.IsPrint(r):
			b.WriteString(text[i : i+width])
		default:
			b.WriteString(strings.Trim(strconv.QuoteRune(r), "'"))
		}
		i += width
	}
	return b.String()
}
