package casing_test

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/ruandre/casemv/internal/casing"
)

func TestLowerExtensionFormat(t *testing.T) {
	tests := []struct {
		name       string
		entry      string
		isDir      bool
		convention casing.Convention
		want       string
	}{
		{"extension is lowercased", "My File.TXT", false, casing.Kebab, "my-file.txt"},
		{"only the last extension is one", "Archive.TAR.GZ", false, casing.Kebab, "archive-tar.gz"},
		{"mixed-case extension", "Photo.JpEg", false, casing.Snake, "photo.jpeg"},
		{"converted stem, extension only changes", "notes.MD", false, casing.Kebab, "notes.md"},
		{"dotfile extension", ".Env.Local", false, casing.Snake, ".env.local"},
		{"dotfile without extension", ".Gitignore", false, casing.Kebab, ".gitignore"},
		{"stem without word runes keeps stem", "___.TXT", false, casing.Kebab, "___.txt"},
		{"spaced dot is not an extension", "1. Something", false, casing.Kebab, "1-something"},
		{"directory has no extension", "My Project.V2", true, casing.Pascal, "MyProjectV2"},
		{"screaming stem, lowercase extension", "My File.Txt", false, casing.ScreamingSnake, "MY_FILE.txt"},
		{"lower convention", "My_File.TXT", false, casing.Lower, "my_file.txt"},
		{"undecodable byte survives", "My File.T\xffXT", false, casing.Kebab, "my-file.t\xffxt"},
		{"non-ASCII extension", "Datei.ÄÖÜ", false, casing.Kebab, "datei.äöü"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			namer := casing.LowerExtension{Convention: tc.convention}
			if got := namer.Format(tc.entry, tc.isDir); got != tc.want {
				t.Errorf("LowerExtension{%s}.Format(%q, isDir=%v) = %q, want %q",
					tc.convention, tc.entry, tc.isDir, got, tc.want)
			}
		})
	}
}

// FuzzLowerExtension asserts that lowering touches nothing but the extension,
// and that a later run, with or without lowering, finds nothing left to change.
func FuzzLowerExtension(f *testing.F) {
	for _, seed := range []string{"My File.TXT", "1. Something", ".Env.Local", "___.TXT", "a.T\xffXT", "ϒ.ϒ"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, name string) {
		if name == "" || strings.ContainsAny(name, "/\x00") || strings.TrimLeft(name, ".") == "" {
			t.Skip("not a renamable base name")
		}
		eachFormatVariant(func(convention casing.Convention, isDir bool) {
			assertLowerExtensionProperties(t, convention, name, isDir)
		})
	})
}

func assertLowerExtensionProperties(t *testing.T, convention casing.Convention, name string, isDir bool) {
	t.Helper()
	namer := casing.LowerExtension{Convention: convention}
	plain := convention.Format(name, isDir)
	lowered := namer.Format(name, isDir)

	if isDir && lowered != plain {
		t.Fatalf("%s: directory %q became %q, want %q as without lowering", convention, name, lowered, plain)
	}
	if lowerRunes(lowered) != lowerRunes(plain) {
		t.Fatalf("%s: %q lowered to %q, which differs from %q beyond letter case", convention, name, lowered, plain)
	}
	// Joined undecodable bytes can form a valid rune, as FuzzFormatIsIdempotent notes.
	if utf8.ValidString(name) {
		if again := namer.Format(lowered, isDir); again != lowered {
			t.Fatalf("%s: not idempotent on %q: %q then %q", convention, name, lowered, again)
		}
		if again := convention.Format(lowered, isDir); again != lowered {
			t.Fatalf("%s: plain Format changes lowered %q to %q", convention, lowered, again)
		}
	}
	if strings.HasPrefix(name, ".") != strings.HasPrefix(lowered, ".") {
		t.Fatalf("%s: %q lowered to %q changed hidden status", convention, name, lowered)
	}
	for b, want := range opaqueBytes(name) {
		if got := strings.Count(lowered, string([]byte{b})); got < want {
			t.Fatalf("%s: %q lowered to %q keeps byte %#x %d times, want at least %d", convention, name, lowered, b, got, want)
		}
	}
}

// lowerRunes lowercases decodable runes and keeps undecodable bytes, as lowering does.
func lowerRunes(s string) string {
	var b strings.Builder
	for i, r := range s {
		if r == unicode.ReplacementChar && !strings.HasPrefix(s[i:], string(unicode.ReplacementChar)) {
			b.WriteByte(s[i])
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}
