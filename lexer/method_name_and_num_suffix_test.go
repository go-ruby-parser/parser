package lexer

import (
	"testing"

	"github.com/go-ruby-parser/parser/token"
)

// types returns the token types of src, without the trailing EOF.
func types(src string) []token.Type {
	toks := New(src).Tokenize()
	out := make([]token.Type, 0, len(toks))
	for _, t := range toks {
		if t.Type == token.EOF {
			break
		}
		out = append(out, t.Type)
	}
	return out
}

// TestOperatorAfterDotIsAMethodName covers atMethodName: in a method-NAME
// position — MRI's EXPR_FNAME after `def`, EXPR_DOT after `.`, `&.` or `::` —
// a `%` or `/` spells the method's name, so neither may open a literal. Neither
// state belongs to EXPR_BEG_ANY, the set parser_yylex tests before it calls
// parse_percent or opens a regexp.
//
// Each case is a three-token prefix: the receiver/keyword, the separator, and the
// operator that must arrive as an OPERATOR token rather than a STRING or REGEXP.
func TestOperatorAfterDotIsAMethodName(t *testing.T) {
	cases := []struct {
		src  string
		at   int // index of the token that must be the operator, not a literal
		want token.Type
	}{
		{"x.%(1)", 2, token.PERCENT},
		{"x&.%(1)", 2, token.PERCENT},
		{"X::%(1)", 2, token.PERCENT},
		{"def %(o)", 1, token.PERCENT},
		{"x./(1)", 2, token.SLASH},
		{"x&./(1)", 2, token.SLASH},
		{"X::/(1)", 2, token.SLASH},
		{"def /(o)", 1, token.SLASH},
	}
	for _, c := range cases {
		got := types(c.src)
		if len(got) <= c.at {
			t.Errorf("%q: %d tokens, want more than %d", c.src, len(got), c.at)
			continue
		}
		if got[c.at] != c.want {
			t.Errorf("%q: token %d = %s, want %s (a literal opened instead of a method name)",
				c.src, c.at, got[c.at], c.want)
		}
	}
}

// TestPercentAndSlashStillOpenLiteralsElsewhere is the other side of that
// predicate: outside a method-name position the same characters must still open
// their literals, so the fix above cannot be satisfied by never opening them.
func TestPercentAndSlashStillOpenLiteralsElsewhere(t *testing.T) {
	if got := types("%(hi)"); len(got) != 1 || got[0] != token.STRING {
		t.Errorf("%%(hi): tokens = %v, want one STRING", got)
	}
	if got := types("/re/"); len(got) != 1 || got[0] != token.REGEXP {
		t.Errorf("/re/: tokens = %v, want one REGEXP", got)
	}
	if got := types("a % b"); len(got) != 3 || got[1] != token.PERCENT {
		t.Errorf("a %% b: tokens = %v, want the modulo operator in the middle", got)
	}
}

// TestNumberLiteralSuffix covers numberLiteralSuffix, which mirrors MRI's
// number_literal_suffix (parse.y v3_4_0 9044-9071). Three behaviours are pinned,
// and the last two are the ones a simpler "consume r then consume i" reading gets
// wrong:
//
//   - `r`, `i` and `ri` are the accepted suffixes, on decimal AND radix literals;
//   - an `r` after an `i` is masked off, so `2ir` is not a suffix pair;
//   - a letter, `_` or non-ASCII byte ending the scan rejects the WHOLE suffix and
//     rewinds, which is why `2rescue nil` is the integer 2 with a modifier rescue.
func TestNumberLiteralSuffix(t *testing.T) {
	cases := []struct {
		src        string
		wantLit    string
		wantFlags  string
		wantNext   token.Type // the token after the number (EOF-trimmed away if absent)
		wantNextIs bool
	}{
		{src: "2r", wantLit: "2", wantFlags: "r"},
		{src: "2i", wantLit: "2", wantFlags: "i"},
		{src: "2ri", wantLit: "2", wantFlags: "ri"},
		{src: "0x10r", wantLit: "0x10", wantFlags: "r"},
		{src: "0b10i", wantLit: "0b10", wantFlags: "i"},
		{src: "0o7r", wantLit: "0o7", wantFlags: "r"},
		{src: "0d9i", wantLit: "9", wantFlags: "i"},
		{src: "0x10ri", wantLit: "0x10", wantFlags: "ri"},
		// `i` first: the `r` bit is cleared, so the trailing `r` is a letter that
		// rejects the whole suffix and rewinds — `2ir` is `2` then the name `ir`.
		{src: "2ir", wantLit: "2", wantFlags: "", wantNext: token.IDENT, wantNextIs: true},
		// The rewind that makes `2rescue nil` legal.
		{src: "2rescue", wantLit: "2", wantFlags: "", wantNext: token.RESCUE, wantNextIs: true},
		// A letter with no suffix character at all takes the same path.
		{src: "2x", wantLit: "2", wantFlags: "", wantNext: token.IDENT, wantNextIs: true},
		// A `_` is in MRI's reject set too.
		{src: "2r_x", wantLit: "2", wantFlags: "", wantNext: token.IDENT, wantNextIs: true},
		// A non-letter after the suffix simply ends the scan.
		{src: "2r.to_s", wantLit: "2", wantFlags: "r", wantNext: token.DOT, wantNextIs: true},
		{src: "2 + 1", wantLit: "2", wantFlags: "", wantNext: token.PLUS, wantNextIs: true},
	}
	for _, c := range cases {
		toks := New(c.src).Tokenize()
		if len(toks) == 0 {
			t.Errorf("%q: no tokens", c.src)
			continue
		}
		if toks[0].Type != token.INT {
			t.Errorf("%q: first token = %s, want INT", c.src, toks[0].Type)
			continue
		}
		if toks[0].Lit != c.wantLit {
			t.Errorf("%q: Lit = %q, want %q", c.src, toks[0].Lit, c.wantLit)
		}
		if toks[0].Flags != c.wantFlags {
			t.Errorf("%q: Flags = %q, want %q", c.src, toks[0].Flags, c.wantFlags)
		}
		if c.wantNextIs {
			if len(toks) < 2 {
				t.Errorf("%q: only %d tokens, want a following %s", c.src, len(toks), c.wantNext)
				continue
			}
			if toks[1].Type != c.wantNext {
				t.Errorf("%q: second token = %s, want %s", c.src, toks[1].Type, c.wantNext)
			}
		}
	}
}

// TestClauseKeywordInMethodNamePositionDoesNotContinueTheLine covers prevFname:
// `when`, `in` and `case` continue a line the way `if` and `while` already did,
// but a reserved word in a method-NAME position is a NAME and must not. Without
// this, `def in` followed by a body on the next line joined into one line and
// prism/node.rb stopped parsing.
func TestClauseKeywordInMethodNamePositionDoesNotContinueTheLine(t *testing.T) {
	for _, name := range []string{"in", "when", "case", "then", "if", "while", "and"} {
		got := types("def " + name + "\n1\n")
		// def, NAME, NEWLINE, 1, NEWLINE — the newline after the name must survive.
		if len(got) < 3 {
			t.Errorf("def %s: tokens = %v, want at least 3", name, got)
			continue
		}
		if got[2] != token.NEWLINE {
			t.Errorf("def %s: third token = %s, want NEWLINE (the line must not continue)", name, got[2])
		}
	}
	// In statement position the same keywords DO continue the line, so the guard is
	// not simply switching the continuation off.
	for _, src := range []string{"case x\nwhen\n1\n", "for i in\n[1]\n"} {
		got := types(src)
		for i, tt := range got {
			if tt == token.WHEN || tt == token.IN {
				if i+1 < len(got) && got[i+1] == token.NEWLINE {
					t.Errorf("%q: a NEWLINE survived directly after %s, want it joined", src, tt)
				}
			}
		}
	}
}
