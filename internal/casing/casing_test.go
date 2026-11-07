package casing_test

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/ruandre/casemv/internal/casing"
)

func TestConventionsCoverEveryStyle(t *testing.T) {
	for _, c := range casing.Conventions() {
		got := c.FormatText("foo bar baz")
		if got == "" {
			t.Errorf("convention %q produced an empty result; styleFor is likely missing a case", c)
		}
	}
}

func TestFormatTextByConvention(t *testing.T) {
	const input = "my FILE name-2 HTTPServer"
	tests := map[casing.Convention]string{
		casing.Camel:          "myFileName2HttpServer",
		casing.Kebab:          "my-file-name-2-http-server",
		casing.Lower:          "my file name-2 httpserver",
		casing.Pascal:         "MyFileName2HttpServer",
		casing.ScreamingSnake: "MY_FILE_NAME_2_HTTP_SERVER",
		casing.Sentence:       "My file name 2 http server",
		casing.Snake:          "my_file_name_2_http_server",
		casing.Title:          "My File Name 2 Http Server",
		casing.Train:          "My-File-Name-2-Http-Server",
	}
	for convention, want := range tests {
		t.Run(string(convention), func(t *testing.T) {
			if got := convention.FormatText(input); got != want {
				t.Errorf("FormatText(%q) = %q, want %q", input, got, want)
			}
		})
	}
}

func TestFormatTextWordSplitting(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"fooBar", "foo-bar"},
		{"FooBar", "foo-bar"},
		{"foo_bar", "foo-bar"},
		{"foo-bar", "foo-bar"},
		{"foo bar", "foo-bar"},
		{"foo.bar", "foo-bar"},
		{"foo___bar", "foo-bar"},
		{"  foo  bar  ", "foo-bar"},
		{"HTTPServer", "http-server"},
		{"XMLHttpRequest", "xml-http-request"},
		{"ABC", "abc"},
		{"iOS", "i-os"},
		{"v2", "v2"},
		{"foo2Bar", "foo2-bar"},
		{"1. Something", "1-something"},
		{"1.Something", "1-something"},
		{"日本語 Text", "日本語-text"},
		{"Cafe\u0301s", "cafe\u0301s"},
		{"e\u0301te\u0301", "e\u0301te\u0301"},
		{"Re\u0301sume\u0301", "re\u0301sume\u0301"},
		{"", ""},
		{"___", ""},
		{"a", "a"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			if got := casing.Kebab.FormatText(tc.input); got != tc.want {
				t.Errorf("Kebab.FormatText(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestFormatExtensionHandling(t *testing.T) {
	tests := []struct {
		name       string
		entry      string
		isDir      bool
		convention casing.Convention
		want       string
	}{
		{"dir with dot is not an extension", "1. Something", true, casing.Kebab, "1-something"},
		{"file with spaced dot is not an extension", "1. Something", false, casing.Kebab, "1-something"},
		{"file keeps a real extension verbatim", "My File.TXT", false, casing.Kebab, "my-file.TXT"},
		{"only the last dot is an extension", "archive.tar.gz", false, casing.Kebab, "archive-tar.gz"},
		{"numbered file with extension", "1. Introduction.md", false, casing.Kebab, "1-introduction.md"},
		{"dir with an extension-like suffix", "My Project.v2", true, casing.Kebab, "my-project-v2"},
		{"dotfile stays hidden", ".gitignore", false, casing.Kebab, ".gitignore"},
		{"dotfile is converted", ".My Config", false, casing.Kebab, ".my-config"},
		{"dotfile with extension", ".Env.Local", false, casing.Snake, ".env.Local"},
		{"stem without word runes is untouched", "___.txt", false, casing.Kebab, "___.txt"},
		{"separator-only name is untouched", "--", false, casing.Kebab, "--"},
		{"no extension", "My File", false, casing.Snake, "my_file"},
		{"already converted name is stable", "my-file.txt", false, casing.Kebab, "my-file.txt"},
		{"lower does not resplit words", "My_File.TXT", false, casing.Lower, "my_file.TXT"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.convention.Format(tc.entry, tc.isDir); got != tc.want {
				t.Errorf("%s.Format(%q, isDir=%v) = %q, want %q", tc.convention, tc.entry, tc.isDir, got, tc.want)
			}
		})
	}
}

func TestFormatIsIdempotent(t *testing.T) {
	names := []string{
		"My File.TXT", "1. Something", "HTTPServer.go", ".gitignore",
		"archive.tar.gz", "foo_bar baz", "日本語 Text",
		"A B.txt", "I D.md", "A A A.txt", "2x.txt", "part2beta.txt", "x9y.txt",
	}
	for _, convention := range casing.Conventions() {
		for _, name := range names {
			for _, isDir := range []bool{true, false} {
				once := convention.Format(name, isDir)
				if twice := convention.Format(once, isDir); twice != once {
					t.Errorf("%s.Format(%q, isDir=%v): not idempotent, %q then %q",
						convention, name, isDir, once, twice)
				}
			}
		}
	}
}

func TestFormatCounterexamples(t *testing.T) {
	tests := []struct {
		name       string
		convention casing.Convention
		input      string
		want       string
	}{
		{"pascal with A B.txt", casing.Pascal, "A B.txt", "Ab.txt"},
		{"pascal with I D.md", casing.Pascal, "I D.md", "Id.md"},
		{"camel with A A A.txt", casing.Camel, "A A A.txt", "aAa.txt"},
		{"screaming with 2x.txt", casing.ScreamingSnake, "2x.txt", "2X.txt"},
		{"screaming with part2beta.txt", casing.ScreamingSnake, "part2beta.txt", "PART2BETA.txt"},
		{"screaming with x9y.txt", casing.ScreamingSnake, "x9y.txt", "X9Y.txt"},
		// Titlecase digraphs (ǅ ǈ ǋ ǲ) start a word like an uppercase letter.
		{"camel with titlecase digraph", casing.Camel, "aǇ.txt", "aǈ.txt"},
		{"pascal with titlecase digraphs", casing.Pascal, "Ǉ Ǉ.txt", "ǈǉ.txt"},
		// ʡ is Ll yet has no case mapping, so word splitting treats it as caseless.
		{"pascal with caseless Ll rune", casing.Pascal, "ʒ ӡʡ.txt", "Ʒӡʡ.txt"},
		// Ⅶ is Nl, not Lu, yet lowercases to ⅶ.
		{"screaming with roman numeral", casing.ScreamingSnake, "Ⅶa.txt", "ⅦA.txt"},
		// U+0345 is a combining mark with case mappings, so it cannot join the rune before it.
		{"camel with cased combining mark", casing.Camel, "0 \u0345A\u0345.txt", "0\u0345A\u0345.txt"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			once := tc.convention.Format(tc.input, false)
			if once != tc.want {
				t.Errorf("%s.Format(%q) = %q, want %q", tc.convention, tc.input, once, tc.want)
			}
			twice := tc.convention.Format(once, false)
			if twice != once {
				t.Errorf("%s.Format(%q) not idempotent: once %q, twice %q", tc.convention, tc.input, once, twice)
			}
		})
	}
}

func FuzzFormatIsIdempotent(f *testing.F) {
	for _, seed := range []string{
		"My File.txt", "1. Something", ".gitignore", "a", "___",
		"A B.txt", "I D.md", "A A A.txt", "2x.txt", "part2beta.txt", "x9y.txt",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, name string) {
		if name == "" || strings.ContainsAny(name, "/\x00") || strings.TrimLeft(name, ".") == "" {
			t.Skip("not a renamable base name")
		}
		if !utf8.ValidString(name) {
			t.Skip("malformed utf-8 byte sequence")
		}
		eachFormatVariant(func(convention casing.Convention, isDir bool) {
			once := convention.Format(name, isDir)
			if twice := convention.Format(once, isDir); twice != once {
				t.Fatalf("%s.Format is not idempotent for %q (isDir=%v): once %q, twice %q",
					convention, name, isDir, once, twice)
			}
		})
	})
}

func TestParseConvention(t *testing.T) {
	for _, c := range casing.Conventions() {
		for _, input := range []string{string(c), strings.ToUpper(string(c)), " " + string(c) + " "} {
			got, err := casing.ParseConvention(input)
			if err != nil {
				t.Errorf("ParseConvention(%q): %v", input, err)
				continue
			}
			if got != c {
				t.Errorf("ParseConvention(%q) = %q, want %q", input, got, c)
			}
		}
	}
	for _, input := range []string{"", "kebab-case", "KebabCase", "nope"} {
		if _, err := casing.ParseConvention(input); err == nil {
			t.Errorf("ParseConvention(%q) succeeded, want an error", input)
		}
	}
}

func TestParseConventionErrorListsChoices(t *testing.T) {
	_, err := casing.ParseConvention("nope")
	if err == nil {
		t.Fatal("want an error")
	}
	for _, c := range casing.Conventions() {
		if !strings.Contains(err.Error(), string(c)) {
			t.Errorf("error %q does not mention convention %q", err, c)
		}
	}
}

// FuzzFormat asserts the invariants every convention must hold for any name:
// the result is never empty, keeps the hidden prefix, and is stable.
func FuzzFormat(f *testing.F) {
	for _, seed := range []string{"My File.txt", "1. Something", ".gitignore", "a", "___", "日本語"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, name string) {
		if name == "" || strings.ContainsAny(name, "/\x00") || strings.TrimLeft(name, ".") == "" {
			t.Skip("not a renamable base name")
		}
		eachFormatVariant(func(convention casing.Convention, isDir bool) {
			assertFormatInvariants(t, convention, name, isDir)
		})
	})
}

// eachFormatVariant calls check for every convention, as file and as directory.
func eachFormatVariant(check func(convention casing.Convention, isDir bool)) {
	for _, convention := range casing.Conventions() {
		for _, isDir := range []bool{true, false} {
			check(convention, isDir)
		}
	}
}

func assertFormatInvariants(t *testing.T, convention casing.Convention, name string, isDir bool) {
	t.Helper()
	got := convention.Format(name, isDir)
	if got == "" {
		t.Fatalf("%s.Format(%q, %v) returned an empty name", convention, name, isDir)
	}
	if strings.HasPrefix(name, ".") != strings.HasPrefix(got, ".") {
		t.Fatalf("%s.Format(%q, %v) = %q changed hidden status", convention, name, isDir, got)
	}
	if strings.ContainsRune(got, '/') || strings.ContainsRune(got, 0) {
		t.Fatalf("%s.Format(%q, %v) = %q introduced a path separator", convention, name, isDir, got)
	}
}

func TestTitleWordUsesUnicodeTitleCase(t *testing.T) {
	// ǳ has distinct title-case form (ǲ) from upper-case form (Ǳ).
	got := casing.Pascal.FormatText("ǳ foo")
	if r, _ := utf8.DecodeRuneInString(got); !unicode.IsTitle(r) && !unicode.IsUpper(r) {
		t.Errorf("Pascal.FormatText(%q) = %q, want a title-cased first rune", "ǳ foo", got)
	}
}

func BenchmarkFormat(b *testing.B) {
	for b.Loop() {
		casing.Kebab.Format("1. Some Long_DirectoryName With HTTPServer Words.txt", false)
	}
}

// Unix filenames are byte strings, not necessarily UTF-8. Legacy-encoded names
// must survive conversion byte for byte; routing them through []rune or
// strings.ToLower would collapse each undecodable byte to U+FFFD, and the
// tokenizer would then drop it as a separator.
func TestFormatPreservesUndecodableBytes(t *testing.T) {
	tests := []struct {
		name       string
		entry      string
		convention casing.Convention
		want       string
	}{
		{"latin-1 e-acute mid word", "Caf\xe9 Menu.txt", casing.Kebab, "caf\xe9-menu.txt"},
		{"latin-1 i-diaeresis", "Na\xefve Report.md", casing.Kebab, "na\xefve-report.md"},
		{"leading opaque byte", "\xe9diteur File.txt", casing.Snake, "\xe9diteur_file.txt"},
		{"opaque byte only", "\xff\xfe.txt", casing.Kebab, "\xff\xfe.txt"},
		{"screaming keeps the byte", "Caf\xe9 Menu.txt", casing.ScreamingSnake, "CAF\xe9_MENU.txt"},
		{"lower keeps the byte", "Caf\xe9_Menu.TXT", casing.Lower, "caf\xe9_menu.TXT"},
		{"title keeps the byte", "caf\xe9 menu.txt", casing.Title, "Caf\xe9 Menu.txt"},
		{"valid utf-8 is unaffected", "Café Menu.txt", casing.Kebab, "café-menu.txt"},
		{"real replacement char is still a separator", "Caf� Menu.txt", casing.Kebab, "caf-menu.txt"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.convention.Format(tc.entry, false); got != tc.want {
				t.Errorf("%s.Format(%q) = %q, want %q", tc.convention, tc.entry, got, tc.want)
			}
		})
	}
}

// FuzzFormatKeepsOpaqueBytes asserts the property the first fuzz target missed:
// separators may be dropped and case may change, but a byte that is not valid
// UTF-8 must survive, because it is part of the filename on disk.
func FuzzFormatKeepsOpaqueBytes(f *testing.F) {
	for _, seed := range []string{"Caf\xe9 Menu.txt", "My File.txt", "1. Something", "\xff\xfe", "日本語 File"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, name string) {
		if name == "" || strings.ContainsAny(name, "/\x00") {
			t.Skip("not a renamable base name")
		}
		opaque := opaqueBytes(name)
		eachFormatVariant(func(convention casing.Convention, isDir bool) {
			got := convention.Format(name, isDir)
			for b, want := range opaque {
				if n := strings.Count(got, string([]byte{b})); n < want {
					t.Fatalf("%s.Format(%q, %v) = %q keeps byte %#x %d times, want at least %d",
						convention, name, isDir, got, b, n, want)
				}
			}
		})
	})
}

// opaqueBytes counts undecodable UTF-8 bytes in s.
func opaqueBytes(s string) map[byte]int {
	counts := make(map[byte]int)
	for offset := 0; offset < len(s); {
		r, width := utf8.DecodeRuneInString(s[offset:])
		if r == utf8.RuneError && width == 1 {
			counts[s[offset]]++
		}
		offset += width
	}
	return counts
}

func TestFormatPreservesCombiningMarks(t *testing.T) {
	tests := []struct {
		name       string
		entry      string
		isDir      bool
		convention casing.Convention
		want       string
	}{
		{"kebab preserves combining mark in Cafe\u0301s.txt", "Cafe\u0301s.txt", false, casing.Kebab, "cafe\u0301s.txt"},
		{"snake preserves combining mark in Cafe\u0301s.txt", "Cafe\u0301s.txt", false, casing.Snake, "cafe\u0301s.txt"},
		{"pascal preserves combining mark in Cafe\u0301s.txt", "Cafe\u0301s.txt", false, casing.Pascal, "Cafe\u0301s.txt"},
		{"title preserves combining mark in Cafe\u0301s.txt", "Cafe\u0301s.txt", false, casing.Title, "Cafe\u0301s.txt"},
		{"camel preserves combining mark in Cafe\u0301s.txt", "Cafe\u0301s.txt", false, casing.Camel, "cafe\u0301s.txt"},
		{"screaming preserves combining mark in Cafe\u0301s.txt", "Cafe\u0301s.txt", false, casing.ScreamingSnake, "CAFE\u0301S.txt"},

		{"kebab preserves combining marks in e\u0301te\u0301", "e\u0301te\u0301", false, casing.Kebab, "e\u0301te\u0301"},
		{"snake preserves combining marks in e\u0301te\u0301", "e\u0301te\u0301", false, casing.Snake, "e\u0301te\u0301"},
		{"pascal title-cases first rune and preserves combining marks in e\u0301te\u0301", "e\u0301te\u0301", false, casing.Pascal, "E\u0301te\u0301"},
		{"title title-cases first rune and preserves combining marks in e\u0301te\u0301", "e\u0301te\u0301", false, casing.Title, "E\u0301te\u0301"},
		{"screaming upper-cases and preserves combining marks in e\u0301te\u0301", "e\u0301te\u0301", false, casing.ScreamingSnake, "E\u0301TE\u0301"},

		{"kebab preserves combining marks in Re\u0301sume\u0301.pdf", "Re\u0301sume\u0301.pdf", false, casing.Kebab, "re\u0301sume\u0301.pdf"},
		{"snake preserves combining marks in Re\u0301sume\u0301.pdf", "Re\u0301sume\u0301.pdf", false, casing.Snake, "re\u0301sume\u0301.pdf"},
		{"pascal preserves combining marks in Re\u0301sume\u0301.pdf", "Re\u0301sume\u0301.pdf", false, casing.Pascal, "Re\u0301sume\u0301.pdf"},
		{"title preserves combining marks in Re\u0301sume\u0301.pdf", "Re\u0301sume\u0301.pdf", false, casing.Title, "Re\u0301sume\u0301.pdf"},

		{"kebab splits words and preserves combining marks", "cafe\u0301 noir.txt", false, casing.Kebab, "cafe\u0301-noir.txt"},
		{"snake splits words and preserves combining marks", "cafe\u0301 noir.txt", false, casing.Snake, "cafe\u0301_noir.txt"},
		{"pascal splits words and preserves combining marks", "cafe\u0301 noir.txt", false, casing.Pascal, "Cafe\u0301Noir.txt"},
		{"title splits words and preserves combining marks", "cafe\u0301 noir.txt", false, casing.Title, "Cafe\u0301 Noir.txt"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.convention.Format(tc.entry, tc.isDir); got != tc.want {
				t.Errorf("%s.Format(%q, isDir=%v) = %q, want %q", tc.convention, tc.entry, tc.isDir, got, tc.want)
			}
		})
	}
}

// Names may arrive in NFD (HFS+ stores them so), spelling É as E plus U+0301;
// the result must not depend on which normal form a name uses.
func TestFormatTextTreatsDecomposedLikeComposed(t *testing.T) {
	inputs := []string{"A Été", "HTTPSÉté", "caféNOIR", "Été A B", "ÉTÉ Ab", "Ǘ Ǘ", "日本ÉTÉé"}
	for _, convention := range casing.Conventions() {
		for _, input := range inputs {
			assertNormalFormsAgree(t, convention, input)
		}
	}
}

// normalFormAlphabet mixes plain, precomposed and doubly accented letters of
// both cases with digits, separators and caseless letters. Every entry's case
// mappings commute with canonical decomposition, which the property needs.
var normalFormAlphabet = []string{"A", "a", "É", "é", "Ǘ", "ǘ", "N", "n", "Ñ", "ñ", "1", " ", "_", "日"}

func FuzzFormatTextAgreesAcrossNormalForms(f *testing.F) {
	for _, seed := range [][]byte{{0, 2, 3}, {0, 12, 2, 3, 2, 3}, {2, 2, 2, 3, 9}} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, picks []byte) {
		var text strings.Builder
		for _, pick := range picks {
			text.WriteString(normalFormAlphabet[int(pick)%len(normalFormAlphabet)])
		}
		for _, convention := range casing.Conventions() {
			assertNormalFormsAgree(t, convention, text.String())
		}
	})
}

func assertNormalFormsAgree(t *testing.T, convention casing.Convention, text string) {
	t.Helper()
	want := norm.NFD.String(convention.FormatText(norm.NFC.String(text)))
	if got := convention.FormatText(norm.NFD.String(text)); got != want {
		t.Errorf("%s.FormatText(NFD %+q) = %+q, want %+q as for NFC", convention, text, got, want)
	}
}

// Format and FormatText document a panic for an invalid convention; it must
// not depend on the input holding a word.
func TestInvalidConventionPanicsWhateverTheInput(t *testing.T) {
	calls := map[string]func(){
		"Format with words":    func() { casing.Convention("invalid").Format("My File.txt", false) },
		"Format without words": func() { casing.Convention("invalid").Format("___.txt", false) },
		"FormatText empty":     func() { casing.Convention("").FormatText("") },
		"LowerExtension":       func() { casing.LowerExtension{Convention: "invalid"}.Format("___.TXT", false) },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("no panic, want one for an invalid convention")
				}
			}()
			call()
		})
	}
}
