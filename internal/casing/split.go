package casing

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// class groups runes by word boundary effect.
type class uint8

const (
	classSeparator class = iota
	classLower
	classUpper
	classCaseless
	classDigit
	// classOpaque is a byte that is not valid UTF-8. Filenames on Unix are byte
	// strings, so legacy encodings reach us intact; such a byte joins the
	// surrounding word and never starts or ends one.
	classOpaque
)

// typicalWordCount sizes the word slice for most filenames; longer ones grow it.
const typicalWordCount = 4

// classify treats caseless letters (CJK, Hebrew, …) and combining marks as
// classCaseless so they form words rather than being dropped as separators. width
// is the rune's encoded length, which distinguishes a real U+FFFD from an
// undecodable byte.
func classify(r rune, width int) class {
	switch {
	case r == utf8.RuneError && width == 1:
		return classOpaque
	case unicode.ToUpper(r) != unicode.ToLower(r):
		// Uppercase means lowercasing changes it: Lu, titlecase ǈ, and Other_Uppercase Ⅶ,
		// which unicode.IsUpper misses.
		if unicode.ToLower(r) != r {
			return classUpper
		}
		return classLower
	case unicode.IsLetter(r), unicode.IsMark(r):
		return classCaseless
	case unicode.IsNumber(r):
		return classDigit
	default:
		return classSeparator
	}
}

// glyph is one decoded rune, with any caseless combining marks after it,
// located in the source string. Words are cut from the source by byte offset so
// undecodable bytes survive verbatim; converting through []rune would replace
// each of them with U+FFFD.
type glyph struct {
	class  class
	offset int
}

func scanGlyphs(name string) []glyph {
	glyphs := make([]glyph, 0, len(name))
	for offset := 0; offset < len(name); {
		r, width := utf8.DecodeRuneInString(name[offset:])
		c := classify(r, width)
		if !modifiesPrevious(glyphs, r, c) {
			glyphs = append(glyphs, glyph{class: c, offset: offset})
		}
		offset += width
	}
	return glyphs
}

// modifiesPrevious reports whether r joins the glyph before it, so decomposed
// E+U+0301 splits like precomposed U+00C9. A cased mark (U+0345 uppercases to
// U+0399) keeps a glyph of its own, as does a mark with no word rune before it.
func modifiesPrevious(glyphs []glyph, r rune, c class) bool {
	followsWordRune := len(glyphs) > 0 && glyphs[len(glyphs)-1].class != classSeparator
	return followsWordRune && c == classCaseless && unicode.IsMark(r)
}

// splitWords breaks a name into words, dropping separator runes.
// Boundaries: separator runs, lower|digit→upper ("fooBar"), and the tail of an
// acronym run ("HTTPServer" → "HTTP", "Server").
func splitWords(name string) []string {
	if name == "" {
		return nil
	}
	glyphs := scanGlyphs(name)
	words := make([]string, 0, typicalWordCount)
	start := -1
	for i, g := range glyphs {
		if g.class == classSeparator {
			if start >= 0 {
				words = append(words, name[glyphs[start].offset:g.offset])
				start = -1
			}
			continue
		}
		switch {
		case start < 0:
			start = i
		case startsWord(glyphs, i):
			words = append(words, name[glyphs[start].offset:g.offset])
			start = i
		}
	}
	if start >= 0 {
		words = append(words, name[glyphs[start].offset:])
	}
	return words
}

// startsWord reports whether glyphs[i] begins a new word, given glyphs[i-1] is
// also a word rune.
func startsWord(glyphs []glyph, i int) bool {
	prev, cur := glyphs[i-1].class, glyphs[i].class
	if cur != classUpper {
		return false
	}
	if prev == classLower {
		return true
	}
	endsAcronym := prev == classUpper || prev == classDigit || prev == classCaseless
	nextIsLower := i+1 < len(glyphs) && glyphs[i+1].class == classLower
	return endsAcronym && nextIsLower
}

// mapRunes applies f to each decodable rune and copies undecodable bytes
// through unchanged. strings.ToLower and friends replace those bytes with
// U+FFFD, which would rewrite the name being renamed.
func mapRunes(s string, f func(rune) rune) string {
	var b strings.Builder
	b.Grow(len(s))
	for offset := 0; offset < len(s); {
		r, width := utf8.DecodeRuneInString(s[offset:])
		if r == utf8.RuneError && width == 1 {
			b.WriteByte(s[offset])
		} else {
			b.WriteRune(f(r))
		}
		offset += width
	}
	return b.String()
}

func lowerWord(w string) string { return mapRunes(w, unicode.ToLower) }

func upperWord(w string) string { return mapRunes(w, unicode.ToUpper) }

func titleWord(w string) string {
	r, width := utf8.DecodeRuneInString(w)
	switch {
	case width == 0:
		return w
	case r == utf8.RuneError && width == 1:
		return w[:1] + lowerWord(w[1:])
	default:
		return string(unicode.ToTitle(r)) + lowerWord(w[width:])
	}
}
