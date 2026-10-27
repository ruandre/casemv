// Package casing converts file and directory names between naming conventions.
package casing

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Convention is a naming convention that can be applied to a name.
type Convention string

// The supported conventions, shown with what "foo bar baz" becomes.
const (
	Camel          Convention = "camel"     // Renders fooBarBaz.
	Kebab          Convention = "kebab"     // Renders foo-bar-baz.
	Lower          Convention = "lower"     // Renders foo_bar baz: lowercased, words left intact.
	Pascal         Convention = "pascal"    // Renders FooBarBaz.
	ScreamingSnake Convention = "screaming" // Renders FOO_BAR_BAZ.
	Sentence       Convention = "sentence"  // Renders Foo bar baz.
	Snake          Convention = "snake"     // Renders foo_bar_baz.
	Title          Convention = "title"     // Renders Foo Bar Baz.
	Train          Convention = "train"     // Renders Foo-Bar-Baz.
)

// canonical order used in help text, completion.
var conventions = [...]Convention{
	Camel, Kebab, Lower, Pascal, ScreamingSnake, Sentence, Snake, Title, Train,
}

// Conventions returns every supported convention in the order shown in help text.
func Conventions() []Convention { return conventions[:len(conventions):len(conventions)] }

// ParseConvention resolves a convention name, case-insensitively.
func ParseConvention(name string) (Convention, error) {
	candidate := Convention(strings.ToLower(strings.TrimSpace(name)))
	for _, c := range conventions {
		if c == candidate {
			return c, nil
		}
	}
	return "", fmt.Errorf("unknown naming convention %q (want one of: %s)", name, Names())
}

// Names returns the supported convention names, comma separated.
func Names() string {
	names := make([]string, len(conventions))
	for i, c := range conventions {
		names[i] = string(c)
	}
	return strings.Join(names, ", ")
}

// style describes word rendering, joining per convention.
type style struct {
	separator string
	first     func(string) string
	rest      func(string) string
}

func styleFor(c Convention) style {
	switch c {
	case Camel:
		return style{"", lowerWord, titleWord}
	case Kebab:
		return style{"-", lowerWord, lowerWord}
	case Pascal:
		return style{"", titleWord, titleWord}
	case ScreamingSnake:
		return style{"_", upperWord, upperWord}
	case Sentence:
		return style{" ", titleWord, lowerWord}
	case Snake:
		return style{"_", lowerWord, lowerWord}
	case Title:
		return style{" ", titleWord, titleWord}
	case Train:
		return style{"-", titleWord, titleWord}
	default:
		// Lower never reaches here: formatStem returns before tokenizing.
		panic("casing: no style defined for convention " + string(c))
	}
}

// Format converts a base name to the convention. It panics if c is not one of
// the values ParseConvention returns.
//
// A file's extension is preserved verbatim; a directory has no extension, so
// "1. Something" becomes "1-something" under Kebab instead of stopping at the
// dot. Leading dots are kept, so ".gitignore" stays hidden. Names whose stem
// holds no word runes ("___.txt") are returned unchanged rather than reduced to
// a bare extension.
func (c Convention) Format(name string, isDir bool) string {
	dots, stem, ext := splitBase(name, isDir)
	formatted := c.formatStem(stem)
	if formatted == "" && stem != "" {
		return name
	}
	return dots + formatted + ext
}

// FormatText converts a free-standing string, with no extension handling. It
// panics if c is not one of the values ParseConvention returns.
func (c Convention) FormatText(text string) string { return c.formatStem(text) }

func (c Convention) formatStem(stem string) string {
	if c == Lower {
		return lowerWord(stem)
	}
	words := splitWords(stem)
	if len(words) == 0 {
		return ""
	}
	s := styleFor(c)

	var b strings.Builder
	b.Grow(len(stem) + len(s.separator)*(len(words)-1))
	prev := s.first(words[0])
	b.WriteString(prev)
	for _, w := range words[1:] {
		b.WriteString(s.separator)
		rendered := s.rest(w)
		if s.separator == "" && needsLower(prev, rendered) {
			rendered = lowerWord(w)
		}
		b.WriteString(rendered)
		prev = rendered
	}
	return b.String()
}

func needsLower(prev, rendered string) bool {
	r, width := utf8.DecodeLastRuneInString(prev)
	switch classify(r, width) {
	case classUpper, classDigit, classCaseless:
		return !secondRuneIsLower(rendered)
	case classOpaque:
		return true
	default:
		return false
	}
}

func secondRuneIsLower(s string) bool {
	_, w := utf8.DecodeRuneInString(s)
	if w == 0 || w >= len(s) {
		return false
	}
	r, width := utf8.DecodeRuneInString(s[w:])
	return classify(r, width) == classLower
}
