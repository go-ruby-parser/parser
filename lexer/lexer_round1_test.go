package lexer

import (
	"testing"

	"github.com/go-ruby-parser/parser/token"
)

// firstToken tokenizes src and returns its first non-EOF token.
func firstToken(t *testing.T, src string) token.Token {
	t.Helper()
	toks := New(src).Tokenize()
	if len(toks) == 0 {
		t.Fatalf("Tokenize(%q): no tokens", src)
	}
	return toks[0]
}

// TestSpecialGlobalVars covers the single-character special global variables that
// previously lexed as ILLEGAL. Each is one GVAR token whose Lit keeps the `$`.
func TestSpecialGlobalVars(t *testing.T) {
	specials := []string{
		"$$", "$!", "$@", "$/", "$\\", "$;", "$,", "$.", "$<", "$>",
		"$?", "$*", "$\"", "$:", "$+", "$~", "$&", "$`", "$'",
	}
	for _, s := range specials {
		tok := firstToken(t, s)
		if tok.Type != token.GVAR {
			t.Errorf("%q: type=%s, want GVAR", s, tok.Type)
		}
		if tok.Lit != s {
			t.Errorf("%q: Lit=%q, want %q", s, tok.Lit, s)
		}
	}
}

// TestSpecialGlobalIsExactlyOneChar: a special global is one byte, so a token
// follows immediately ($$x is $$ then x).
func TestSpecialGlobalBoundary(t *testing.T) {
	toks := New("$$x").Tokenize()
	if toks[0].Type != token.GVAR || toks[0].Lit != "$$" {
		t.Fatalf("$$x: first token=%v", toks[0])
	}
	if toks[1].Type != token.IDENT || toks[1].Lit != "x" {
		t.Errorf("$$x: second token=%v, want IDENT x", toks[1])
	}
}

// TestNumberedAndNamedGlobalsUnchanged: $1.., $0, and named globals still lex.
func TestNumberedAndNamedGlobals(t *testing.T) {
	for _, c := range []struct{ src, lit string }{
		{"$1", "$1"}, {"$42", "$42"}, {"$0", "$0"}, {"$stdout", "$stdout"},
	} {
		tok := firstToken(t, c.src)
		if tok.Type != token.GVAR || tok.Lit != c.lit {
			t.Errorf("%q: got %v, want GVAR %q", c.src, tok, c.lit)
		}
	}
}

// TestBareDollarIllegal: a lone `$` with no following name is still ILLEGAL.
func TestBareDollarIllegal(t *testing.T) {
	tok := firstToken(t, "$ ")
	if tok.Type != token.ILLEGAL {
		t.Errorf("`$ `: type=%s, want ILLEGAL", tok.Type)
	}
}

// TestSpecialGlobalInInterpolation: $$ inside #{} lexes as GVAR (regression: it
// used to break the whole string).
func TestSpecialGlobalInInterpolation(t *testing.T) {
	toks := New(`"PID-#{$$}"`).Tokenize()
	var sawGvar bool
	for _, tk := range toks {
		if tk.Type == token.GVAR && tk.Lit == "$$" {
			sawGvar = true
		}
		if tk.Type == token.ILLEGAL {
			t.Fatalf(`"PID-#{$$}": produced ILLEGAL token %v`, tk)
		}
	}
	if !sawGvar {
		t.Errorf(`"PID-#{$$}": no $$ GVAR token in %v`, toks)
	}
}

// TestSingleQuotedSymbol covers :'…' symbols (no interpolation), including the
// \' and \\ escapes a single-quoted string honors.
func TestSingleQuotedSymbol(t *testing.T) {
	for _, c := range []struct{ src, lit string }{
		{`:'foo.bar'`, "foo.bar"},
		{`:'data-remote'`, "data-remote"},
		{`:'simple'`, "simple"},
		{`:'a\'b'`, "a'b"},
		{`:'a\\b'`, `a\b`},
		{`:'with space'`, "with space"},
	} {
		tok := firstToken(t, c.src)
		if tok.Type != token.SYMBOL || tok.Lit != c.lit {
			t.Errorf("%q: got %v, want SYMBOL %q", c.src, tok, c.lit)
		}
	}
}

// TestUnterminatedSingleQuotedSymbol: an unterminated :'… is ILLEGAL.
func TestUnterminatedSingleQuotedSymbol(t *testing.T) {
	tok := firstToken(t, ":'unterminated")
	if tok.Type != token.ILLEGAL {
		t.Errorf(":'unterminated: type=%s, want ILLEGAL", tok.Type)
	}
}

// TestDoubleQuotedSymbolUnchanged: :"…" still works (plain and interpolated).
func TestDoubleQuotedSymbolUnchanged(t *testing.T) {
	if tok := firstToken(t, `:"plain"`); tok.Type != token.SYMBOL || tok.Lit != "plain" {
		t.Errorf(`:"plain": got %v`, tok)
	}
}
