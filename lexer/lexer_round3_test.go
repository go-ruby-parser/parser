package lexer

import (
	"testing"

	"github.com/go-ruby-parser/parser/token"
)

// firstType returns the type of the first token of src.
func firstType(src string) token.Type {
	return New(src).Tokenize()[0].Type
}

// TestPercentNonDelimiters checks that a character that cannot delimit a
// percent-literal (an alphanumeric, whitespace, '=', or a multi-byte UTF-8 lead)
// leaves the '%' as the modulo/percent operator rather than opening a literal.
// This exercises every false branch of isPercentDelim.
func TestPercentNonDelimiters(t *testing.T) {
	// `%w` followed by a non-delimiter is not a word-array literal.
	for _, src := range []string{
		"%wx",  // alpha delimiter candidate
		"%w9",  // digit
		"%w ",  // space
		"%w\t", // tab
	} {
		if firstType(src) == token.WORDS {
			t.Errorf("%q: unexpectedly lexed as a %%w array", src)
		}
	}
	// A bare `%` followed by a non-delimiter is the percent operator.
	for _, src := range []string{
		"%=",  // compound-assignment, never a literal
		"%9",  // digit
		"%a",  // alpha (not q/Q/w/i/r/x/s)
		"% ",  // space
		"%é",  // multi-byte UTF-8 lead
		"%9x", // digit then alpha
	} {
		ty := firstType(src)
		if ty == token.STRING || ty == token.WORDS || ty == token.REGEXP {
			t.Errorf("%q: unexpectedly lexed as a percent literal (%s)", src, ty)
		}
	}
}

// TestPercentDelimMultibyteRejected confirms a multibyte lead after a kind letter
// is not accepted as a delimiter (isPercentDelim's >= 0x80 branch).
func TestPercentDelimMultibyteRejected(t *testing.T) {
	if firstType("%qé") == token.STRING {
		t.Errorf("%%qé: a multibyte delimiter must not open a %%q string")
	}
}

// TestPercentDelimiterAccepted exercises isPercentDelim's success path: assorted
// punctuation delimiters open their literals.
func TestPercentDelimiterAccepted(t *testing.T) {
	cases := map[string]token.Type{
		`%w|a b|`: token.WORDS,
		`%q!hi!`:  token.STRING,
		`%r#ab#`:  token.REGEXP,
		`%i@a b@`: token.SYMBOLS,
		`%(hi)`:   token.STRING,
	}
	for src, want := range cases {
		if got := firstType(src); got != want {
			t.Errorf("%q: first token = %s, want %s", src, got, want)
		}
	}
}
