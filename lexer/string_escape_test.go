package lexer

import (
	"testing"

	"github.com/go-ruby-parser/parser/token"
)

// firstStringLit tokenizes src and returns the literal of the first STRING,
// STRBEG, STRMID, or STREND token (the interpreted body of a double-quoted-style
// string), or ("", false) if none is found.
func firstStringLit(src string) (string, bool) {
	for _, t := range New(src).Tokenize() {
		switch t.Type {
		case token.STRING, token.STRBEG, token.STRMID, token.STREND:
			return t.Lit, true
		}
	}
	return "", false
}

// stringLits returns the literals of every STRING/STRBEG/STRMID/STREND token in
// src, in order — used to inspect a string split across interpolations.
func stringLits(src string) []string {
	var out []string
	for _, t := range New(src).Tokenize() {
		switch t.Type {
		case token.STRING, token.STRBEG, token.STRMID, token.STREND:
			out = append(out, t.Lit)
		}
	}
	return out
}

// TestStringHexEscape: `\xHH` consumes one or two hex digits (greedy) and yields
// the corresponding byte, matching MRI 4.0.5.
func TestStringHexEscape(t *testing.T) {
	cases := map[string]string{
		`"\x41"`:     "A",          // two digits
		`"\x4"`:      "\x04",       // a single digit is enough
		`"\xff"`:     "\xff",       // full byte, upper half
		`"\xFF"`:     "\xff",       // uppercase hex digits
		`"\xfff"`:    "\xff" + "f", // greedy stops after two digits
		`"\x41\x42"`: "AB",         // adjacent escapes
		`"a\x41b"`:   "aAb",        // surrounded by literal text
	}
	for src, want := range cases {
		got, ok := firstStringLit(src)
		if !ok || got != want {
			t.Errorf("%s: got %q (ok=%v), want %q", src, got, ok, want)
		}
	}
}

// TestStringHexEscapeNoDigit: `\x` with no following hex digit has no valid
// interpretation (MRI raises a SyntaxError). Lacking an error channel here the
// lexer degrades gracefully to a literal `x`, keeping the byte stream defined.
func TestStringHexEscapeNoDigit(t *testing.T) {
	for src, want := range map[string]string{
		`"\xZ"`: "xZ", // non-hex follower
		`"\x"`:  "x",  // end of string
	} {
		got, ok := firstStringLit(src)
		if !ok || got != want {
			t.Errorf("%s: got %q (ok=%v), want %q", src, got, ok, want)
		}
	}
}

// TestStringOctalEscape: `\NNN` consumes one to three octal digits (greedy),
// masking the value to a single byte, matching MRI 4.0.5.
func TestStringOctalEscape(t *testing.T) {
	cases := map[string]string{
		`"\101"`:  "A",          // three digits -> 0o101 = 65
		`"\12"`:   "\n",         // two digits -> 0o12 = 10 (newline)
		`"\1"`:    "\x01",       // a single digit
		`"\0"`:    "\x00",       // NUL (the previously-working single case)
		`"\377"`:  "\xff",       // 0o377 = 255
		`"\400"`:  "\x00",       // overflow: 0o400 & 0xFF = 0
		`"\1010"`: "A0",         // greedy stops after three digits
		`"\08"`:   "\x00" + "8", // non-octal digit ends the run
		`"\779"`:  "?9",         // 0o77 = 63 ('?'), then literal 9
		`"a\101"`: "aA",         // after literal text
	}
	for src, want := range cases {
		got, ok := firstStringLit(src)
		if !ok || got != want {
			t.Errorf("%s: got %q (ok=%v), want %q", src, got, ok, want)
		}
	}
}

// TestStringUnicodeEscape: `\uHHHH` (exactly four hex digits) and `\u{...}`
// (whitespace-separated codepoints) both expand to UTF-8 bytes, matching MRI.
func TestStringUnicodeEscape(t *testing.T) {
	// bs is a single literal backslash; building the sources this way keeps the
	// `\u` sequences intact (a bare `A` in Go source would be the compiler's
	// own escape, not the two bytes the lexer must see).
	bs := "\\"
	cases := map[string]string{
		`"` + bs + `u0041"`:       "A",          // four-digit form -> 'A'
		`"` + bs + `u00e9"`:       "é",          // four-digit, two-byte UTF-8
		`"a` + bs + `u0041b"`:     "aAb",        // four-digit among literal text
		`"` + bs + `u{41}"`:       "A",          // brace form, single codepoint
		`"` + bs + `u{41 42 43}"`: "ABC",        // multiple codepoints
		`"` + bs + `u{1F600}"`:    "\U0001F600", // codepoint beyond the BMP
		`"` + bs + `u{}"`:         "",           // empty braces -> nothing
		`"` + bs + `u{ 41 }"`:     "A",          // surrounding whitespace tolerated
	}
	for src, want := range cases {
		got, ok := firstStringLit(src)
		if !ok || got != want {
			t.Errorf("%s: got %q (ok=%v), want %q", src, got, ok, want)
		}
	}
}

// TestStringUnicodeEscapeInvalid: out-of-range / surrogate codepoints fall back
// to U+FFFD, and a `\u` with no hex digit contributes nothing.
func TestStringUnicodeEscapeInvalid(t *testing.T) {
	cases := map[string]string{
		`"\u{110000}"`: "�", // beyond U+10FFFF
		`"\u{D800}"`:   "�", // a surrogate half
		`"\uZ"`:        "Z", // no hex digit after \u: the Z stays as literal text
		`"\u{ZZ}"`:     "",  // no hex digit inside braces
		`"\u{41ZZ}"`:   "A", // trailing junk before the close brace is skipped
		`"\u{41`:       "A", // unterminated brace form runs into end of input
	}
	for src, want := range cases {
		got, ok := firstStringLit(src)
		if !ok || got != want {
			t.Errorf("%s: got %q (ok=%v), want %q", src, got, ok, want)
		}
	}
}

// TestStringBasicEscapesStillWork guards the pre-existing escapes the new code
// sits beside, so the regression covers the whole switch.
func TestStringBasicEscapesStillWork(t *testing.T) {
	cases := map[string]string{
		`"\n"`: "\n",
		`"\t"`: "\t",
		`"\r"`: "\r",
		`"\a"`: "\a",
		`"\b"`: "\b",
		`"\v"`: "\v",
		`"\f"`: "\f",
		`"\e"`: "\x1b",
		`"\s"`: " ",
		`"\\"`: "\\",
		`"\""`: "\"",
		`"\q"`: "q", // unknown escape drops the backslash
	}
	for src, want := range cases {
		got, ok := firstStringLit(src)
		if !ok || got != want {
			t.Errorf("%s: got %q (ok=%v), want %q", src, got, ok, want)
		}
	}
}

// TestNumericEscapesInPercentQAndInterp: the new escapes apply to every
// interpolating string form (%Q, %{}, and the segment after an interpolation),
// since they all funnel through the same scanner.
func TestNumericEscapesInPercentQAndInterp(t *testing.T) {
	if got, _ := firstStringLit(`%Q{\x41\101}`); got != "AA" {
		t.Errorf(`%%Q{\x41\101}: got %q, want "AA"`, got)
	}
	if got, _ := firstStringLit(`%{A}`); got != "A" {
		t.Errorf(`%%{A}: got %q, want "A"`, got)
	}
	// The trailing segment after an interpolation must interpret escapes too.
	lits := stringLits("\"a#{x}\\x42\"")
	if len(lits) < 2 || lits[len(lits)-1] != "B" {
		t.Errorf("interpolation tail: lits=%q, want last segment %q", lits, "B")
	}
}

// TestNumericEscapesNotInSingleQuote: single-quoted strings keep `\x`/`\NNN`
// verbatim (only `\\` and `\'` are special), unchanged by this fix.
func TestNumericEscapesNotInSingleQuote(t *testing.T) {
	for src, want := range map[string]string{
		`'\x41'`: `\x41`,
		`'\101'`: `\101`,
		`'A'`:    `A`,
	} {
		got, ok := firstStringLit(src)
		if !ok || got != want {
			t.Errorf("%s: got %q (ok=%v), want %q", src, got, ok, want)
		}
	}
}
