package casing

import (
	"strings"
	"unicode"
)

// splitBase splits a base name into its leading dots, stem, and extension.
//
// A directory never has an extension, so "1. Something" is entirely stem. For
// files, a candidate extension containing whitespace is not one either: file
// "1. Something" splits the same way as the directory, while "archive.tar.gz"
// still yields ".gz".
func splitBase(name string, isDir bool) (dots, stem, ext string) {
	leadingDots := 0
	for leadingDots < len(name) && name[leadingDots] == '.' {
		leadingDots++
	}
	dots, rest := name[:leadingDots], name[leadingDots:]
	if isDir {
		return dots, rest, ""
	}
	if i := strings.LastIndexByte(rest, '.'); i > 0 && !containsSpace(rest[i:]) {
		return dots, rest[:i], rest[i:]
	}
	return dots, rest, ""
}

func containsSpace(s string) bool { return strings.ContainsFunc(s, unicode.IsSpace) }
