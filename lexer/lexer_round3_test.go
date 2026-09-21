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
// percent-literal (an alphanumeric, whitespace, or a multi-byte UTF-8 lead)
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
		"%",   // end of input: no delimiter byte at all
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

// TestPercentEqualsIsOpAssignAfterAValue pins the state-dependent half of
// percentDelimOK: after a value MRI's parse_percent reaches its `'='` test
// before the quotation branch, so `%=` is the modulo compound assignment.
func TestPercentEqualsIsOpAssignAfterAValue(t *testing.T) {
	toks := New("a %= 2").Tokenize()
	if toks[1].Type != token.OPASSIGN || toks[1].Lit != "%" {
		t.Errorf("a %%= 2: second token = %s %q, want OPASSIGN %q", toks[1].Type, toks[1].Lit, "%")
	}
}

// TestPercentEqualsDelimitsAtExpressionBegin is the other half: where a value is
// expected, `=` delimits the literal.
func TestPercentEqualsDelimitsAtExpressionBegin(t *testing.T) {
	for _, src := range []string{"%=hey=", "%q=hey=", "%w=a b="} {
		toks := New(src).Tokenize()
		if toks[0].Type == token.OPASSIGN || toks[0].Type == token.ILLEGAL {
			t.Errorf("%q: first token = %s, want a literal", src, toks[0].Type)
		}
	}
}

// TestIgnoreCaseGlobal covers `$=`, the obsolete ignore-case flag. MRI's
// parse_gvar (parse.y v3_4_0) lists `=` among the single-character globals that
// lex as tGVAR, alongside `_ ~ * $ ? ! @ / \ ; , . : < > "`.
func TestIgnoreCaseGlobal(t *testing.T) {
	toks := New("p $=").Tokenize()
	if toks[1].Type != token.GVAR || toks[1].Lit != "$=" {
		t.Errorf("p $=: second token = %s %q, want GVAR %q", toks[1].Type, toks[1].Lit, "$=")
	}
	// An assignment to it still sees the following `=` as the operator.
	toks = New("$= = false").Tokenize()
	if toks[0].Type != token.GVAR || toks[0].Lit != "$=" || toks[1].Type != token.ASSIGN {
		t.Errorf("$= = false: got %s %q then %s, want GVAR %q then ASSIGN",
			toks[0].Type, toks[0].Lit, toks[1].Type, "$=")
	}
}

// TestSetterSymbolBeforeRocket covers MRI's parse_ident rule for a trailing `=`
// in a symbol name (parse.y v3_4_0):
//
//	c == '=' && IS_lex_state(EXPR_FNAME) &&
//	 (!peek(p,'~') && !peek(p,'>') && (!peek(p,'=') || peek_n(p,'>',1)))
//
// so `{:a==>1}` is `{:a= => 1}` — the `=` joins the name precisely because the
// `=` after it is followed by `>`.
func TestSetterSymbolBeforeRocket(t *testing.T) {
	for _, src := range []string{`{:a==>1}`, `{:a= =>1}`, `{:a= => 1}`} {
		toks := New(src).Tokenize()
		if toks[1].Type != token.SYMBOL || toks[1].Lit != "a=" {
			t.Errorf("%s: second token = %s %q, want SYMBOL %q", src, toks[1].Type, toks[1].Lit, "a=")
		}
		if toks[2].Type != token.HASHROCKET {
			t.Errorf("%s: third token = %s, want ROCKET", src, toks[2].Type)
		}
	}
	// The `=` must still not be taken when it opens `==`, `=~` or `=>`.
	for _, tc := range []struct{ src, want string }{
		{`:a==`, "a"}, // end of input right after the `==`
		{`:a == 1`, "a"},
		{`:a =~ /x/`, "a"},
		{`:a => 1`, "a"},
		{`:a = 1`, "a"},
		{`:a= 1`, "a="},
	} {
		toks := New(tc.src).Tokenize()
		if toks[0].Type != token.SYMBOL || toks[0].Lit != tc.want {
			t.Errorf("%s: first token = %s %q, want SYMBOL %q", tc.src, toks[0].Type, toks[0].Lit, tc.want)
		}
	}
}
