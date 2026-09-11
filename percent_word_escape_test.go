package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
)

// The tables below pin the escape rules of the four word-list forms. Every
// `want` is the value the installed MRI prints for the same source — each case
// was run through `ruby -e 'p <src>'` and compared byte for byte — so a
// disagreement here is a divergence from Ruby, not a matter of taste.
//
// The rules, from MRI's parse.y (tokadd_string): a backslash escapes the
// character after it, which therefore neither separates two words nor opens or
// closes the list. In a NON-interpolating %w/%i the backslash is otherwise KEPT
// literally — it escapes a character only before whitespace, a delimiter, or
// another backslash. In an interpolating %W/%I the full double-quote escape set
// applies instead, except that a backslash before a newline is a literal
// newline rather than a line continuation.

// TestPercentWordListEscapes covers %w (non-interpolating word list).
func TestPercentWordListEscapes(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want []string
	}{
		// An escaped space is a literal space inside the word, never a
		// separator. This is the case ruby/spec's Dir fixtures depend on: a
		// directory literally named `special/test +()[]{}`.
		{`x = %w[a\ b c]`, []string{"a b", "c"}},                                           // ["a b", "c"]
		{`x = %w[special/test\ +()[]{} other]`, []string{"special/test +()[]{}", "other"}}, // ["special/test +()[]{}", "other"]
		{`x = %w|a\ b c|`, []string{"a b", "c"}},                                           // ["a b", "c"]
		{`x = %w[  a\ b   c  ]`, []string{"a b", "c"}},                                     // ["a b", "c"]
		{`x = %w[a\ b\ c]`, []string{"a b c"}},                                             // ["a b c"]
		{`x = %w[a\  b]`, []string{"a ", "b"}},                                             // ["a ", "b"]
		{`x = %w[\ ]`, []string{" "}},                                                      // [" "]
		{"x = %w[a\\\tb]", []string{"a\tb"}},                                               // ["a\tb"] (escaped TAB)
		{"x = %w[a\\\nb]", []string{"a\nb"}},                                               // ["a\nb"] (escaped newline)

		// An escaped delimiter is a literal character: it neither closes the
		// list nor opens a nested level.
		{`x = %w[a\]b]`, []string{"a]b"}}, // ["a]b"]
		{`x = %w[a\[b]`, []string{"a[b"}}, // ["a[b"]
		{`x = %w(a\)b)`, []string{"a)b"}}, // ["a)b"]
		{`x = %w(a\(b)`, []string{"a(b"}}, // ["a(b"]
		{`x = %w{a\}b}`, []string{"a}b"}}, // ["a}b"]
		{`x = %w{a\{b}`, []string{"a{b"}}, // ["a{b"]
		{`x = %w<a\>b>`, []string{"a>b"}}, // ["a>b"]
		{`x = %w!a\!b!`, []string{"a!b"}}, // ["a!b"]

		// An escaped backslash is ONE literal backslash.
		{`x = %w[a\\b]`, []string{`a\b`}},     // ["a\\b"]
		{`x = %w[\\]`, []string{`\`}},         // ["\\"]
		{`x = %w[a\b\\c]`, []string{`a\b\c`}}, // ["a\\b\\c"]

		// Every other backslash is kept verbatim: %w has no escape set, so
		// `\t` is the two characters `\` and `t`, NOT a tab.
		{`x = %w[a\tb]`, []string{`a\tb`}},     // ["a\\tb"]
		{`x = %w[a\nb]`, []string{`a\nb`}},     // ["a\\nb"]
		{`x = %w[a\ub]`, []string{`a\ub`}},     // ["a\\ub"]
		{`x = %w[a\#b]`, []string{`a\#b`}},     // ["a\\#b"]
		{`x = %w[a\x41b]`, []string{`a\x41b`}}, // ["a\\x41b"]

		// Unescaped whitespace still separates, and brackets still nest.
		{`x = %w[a b c]`, []string{"a", "b", "c"}},  // ["a", "b", "c"]
		{"x = %w[a\tb]", []string{"a", "b"}},        // ["a", "b"]
		{"x = %w[a\nb]", []string{"a", "b"}},        // ["a", "b"]
		{`x = %w[a[b]c]`, []string{"a[b]c"}},        // ["a[b]c"]
		{`x = %w[a[b]c d]`, []string{"a[b]c", "d"}}, // ["a[b]c", "d"]
		{`x = %w(a(b)c)`, []string{"a(b)c"}},        // ["a(b)c"]
		{`x = %w[]`, nil},                           // []
		{`x = %w[ ]`, nil},                          // []
	} {
		wantStrings(t, tc.src, parseRHS(t, tc.src), tc.want)
	}
}

// TestPercentInterpWordListEscapes covers %W (interpolating word list), where
// the full double-quote escape set applies.
func TestPercentInterpWordListEscapes(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want []string
	}{
		{`x = %W[a\ b]`, []string{"a b"}},    // ["a b"]
		{`x = %W[\ ]`, []string{" "}},        // [" "]
		{`x = %W[a\]b]`, []string{"a]b"}},    // ["a]b"]
		{`x = %W{a\}b}`, []string{"a}b"}},    // ["a}b"]
		{`x = %W[a\\b]`, []string{`a\b`}},    // ["a\\b"]
		{`x = %W[a\tb]`, []string{"a\tb"}},   // ["a\tb"]
		{`x = %W[a\nb]`, []string{"a\nb"}},   // ["a\nb"]
		{`x = %W[a\sb]`, []string{"a b"}},    // ["a b"]
		{`x = %W[a\eb]`, []string{"a\x1bb"}}, // ["a\eb"]
		{`x = %W[a\#b]`, []string{"a#b"}},    // ["a#b"]
		{`x = %W[a\x41b]`, []string{"aAb"}},  // ["aAb"]
		{`x = %W[a\101b]`, []string{"aAb"}},  // ["aAb"]
		// An escaped `#{` is literal text, not an interpolation.
		{`x = %W[a\#{1}b]`, []string{"a#{1}b"}}, // ["a\#{1}b"]
		// A backslash before a newline is a literal newline here, NOT the line
		// continuation it would be inside a double-quoted string: MRI tests
		// STR_FUNC_QWORDS before STR_FUNC_EXPAND.
		{"x = %W[a\\\nb]", []string{"a\nb"}}, // ["a\nb"]
		{"x = %W[a\\\tb]", []string{"a\tb"}}, // ["a\tb"]
		{`x = %W[a b c]`, []string{"a", "b", "c"}},
	} {
		wantStrings(t, tc.src, parseRHS(t, tc.src), tc.want)
	}
}

// TestPercentSymbolListEscapes covers %i and %I, which split exactly as %w and
// %W do and differ only in producing symbols.
func TestPercentSymbolListEscapes(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want []string
	}{
		// %i: non-interpolating, so the backslash survives except before
		// whitespace, a delimiter, or another backslash.
		{`x = %i[a\ b]`, []string{"a b"}},        // [:"a b"]
		{`x = %i[\ ]`, []string{" "}},            // [:" "]
		{`x = %i[a\]b c]`, []string{"a]b", "c"}}, // [:"a]b", :c]
		{`x = %i[a\\b]`, []string{`a\b`}},        // [:"a\\b"]
		{`x = %i[a\tb]`, []string{`a\tb`}},       // [:"a\\tb"]
		{`x = %i[a b c]`, []string{"a", "b", "c"}},

		// %I: interpolating, so the double-quote escape set applies.
		{`x = %I[a\ b]`, []string{"a b"}},      // [:"a b"]
		{`x = %I[x\ y\ z]`, []string{"x y z"}}, // [:"x y z"]
		{`x = %I[a\]b]`, []string{"a]b"}},      // [:"a]b"]
		{`x = %I[a\tb]`, []string{"a\tb"}},     // [:"a\tb"]
		{`x = %I[a b c]`, []string{"a", "b", "c"}},
	} {
		wantSymbols(t, tc.src, parseRHS(t, tc.src), tc.want)
	}
}

// TestPercentWordListUnterminated guards the scanner's end-of-input handling
// now that a backslash consumes the character after it: a list that ends on a
// trailing backslash is unterminated, not silently accepted.
func TestPercentWordListUnterminated(t *testing.T) {
	for _, src := range []string{
		`x = %w[a\`,
		`x = %W[a\`,
		`x = %i[a\`,
		`x = %I[a\`,
		`x = %w[a\]`, // the `\]` is escaped, so the list never closes
	} {
		if _, err := parser.Parse(src); err == nil {
			t.Errorf("expected a parse error for %q, got none", src)
		}
	}
}
