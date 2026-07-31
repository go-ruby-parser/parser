package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// strLit parses `x = <src>` and returns the Value of the resulting plain
// (non-interpolated) string literal, failing if the RHS is anything else.
func strLit(t *testing.T, src string) string {
	t.Helper()
	rhs := parseRHS(t, "x = "+src)
	s, ok := rhs.(*ast.StringLit)
	if !ok {
		t.Fatalf("Parse(%q): RHS = %T, want *ast.StringLit", src, rhs)
	}
	return s.Value
}

// TestStringLineContinuation pins MRI's behaviour for a backslash immediately
// before a newline inside a double-quoted / interpolating string literal: both
// the backslash and the newline are removed (line continuation). Every expected
// value below was checked against MRI `ruby` 4.0.x.
func TestStringLineContinuation(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		// Double-quoted, LF continuation: `"a\<LF>b"` => "ab".
		{"dq_lf", "\"a\\\nb\"", "ab"},
		// Double-quoted, CRLF continuation: `"a\<CR><LF>b"` => "ab".
		{"dq_crlf", "\"a\\\r\nb\"", "ab"},
		// A lone `\<CR>` (no LF) is NOT a continuation: MRI drops the backslash
		// and keeps the carriage return => "a\rb".
		{"dq_lone_cr", "\"a\\\rb\"", "a\rb"},
		// Continuation immediately followed by another escape: `"a\<LF>\tb"`
		// strips the continuation, then decodes `\t` => "a\tb".
		{"dq_then_escape", "\"a\\\n\\tb\"", "a\tb"},
		// Continuation at end of the literal: `"a\<LF>"` => "a".
		{"dq_at_end", "\"a\\\n\"", "a"},
		// A backslash-newline that is itself the whole body: `"\<LF>"` => "".
		{"dq_only", "\"\\\n\"", ""},
		// %Q keeps double-quote semantics, so it continues too.
		{"pctQ_lf", "%Q(a\\\nb)", "ab"},
		// %() is the interpolating percent form as well.
		{"pct_lf", "%(a\\\nb)", "ab"},
		// %Q CRLF continuation.
		{"pctQ_crlf", "%Q(a\\\r\nb)", "ab"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := strLit(t, tc.src); got != tc.want {
				t.Errorf("%q => %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// TestHeredocLineContinuation covers the interpolating heredoc flavours, where a
// trailing backslash also continues the line (MRI). Values checked against MRI.
func TestHeredocLineContinuation(t *testing.T) {
	// <<~END (squiggly), <<-END (indented), and plain <<END all interpolate and
	// therefore honour the `\<LF>` continuation. The trailing body newline stays.
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"squiggly", "<<~END\na\\\nb\nEND", "ab\n"},
		{"indented", "<<-END\na\\\nb\n  END", "ab\n"},
		{"plain", "<<END\na\\\nb\nEND", "ab\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := strLit(t, tc.src); got != tc.want {
				t.Errorf("%q => %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// TestStringLineContinuationInterpAdjacent checks a continuation that sits right
// before an interpolation `#{…}`: the string part preceding the interpolation
// has both backslash and newline removed. `"a\<LF>#{1}b"` => "a" then 1 then "b".
func TestStringLineContinuationInterpAdjacent(t *testing.T) {
	check := func(t *testing.T, src, wantLead, wantTail string) {
		t.Helper()
		rhs := parseRHS(t, "x = "+src)
		si, ok := rhs.(*ast.StrInterp)
		if !ok {
			t.Fatalf("Parse(%q): RHS = %T, want *ast.StrInterp", src, rhs)
		}
		if len(si.Parts) != 3 {
			t.Fatalf("Parse(%q): %d parts, want 3", src, len(si.Parts))
		}
		lead, ok := si.Parts[0].(*ast.StringLit)
		if !ok {
			t.Fatalf("Parse(%q): part 0 = %T, want *ast.StringLit", src, si.Parts[0])
		}
		if lead.Value != wantLead {
			t.Errorf("%q: leading part = %q, want %q", src, lead.Value, wantLead)
		}
		tail, ok := si.Parts[2].(*ast.StringLit)
		if !ok {
			t.Fatalf("Parse(%q): part 2 = %T, want *ast.StringLit", src, si.Parts[2])
		}
		if tail.Value != wantTail {
			t.Errorf("%q: trailing part = %q, want %q", src, tail.Value, wantTail)
		}
	}
	// Double-quoted with a continuation just before the interpolation.
	check(t, "\"a\\\n#{1}b\"", "a", "b")
	// Interpolating heredoc with a continuation just before the interpolation.
	check(t, "<<~END\na\\\n#{1}b\nEND", "a", "b\n")
}

// TestStringLineContinuationNegative pins the flavours that must NOT treat a
// backslash-newline as a continuation: single-quoted strings, %q, and the
// non-interpolating heredoc all keep the backslash and the newline verbatim,
// exactly as MRI does.
func TestStringLineContinuationNegative(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		// Single-quoted: only \\ and \' escape; `\<LF>` stays literal.
		{"single_quote", "'a\\\nb'", "a\\\nb"},
		// %q: single-quote semantics.
		{"pctq", "%q(a\\\nb)", "a\\\nb"},
		// Non-interpolating heredoc keeps the backslash+newline (plus the body
		// newline).
		{"heredoc_squote", "<<'END'\na\\\nb\nEND", "a\\\nb\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := strLit(t, tc.src); got != tc.want {
				t.Errorf("%q => %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// TestStringLineContinuationParses is a light end-to-end guard that a multi-line
// continuation inside a real assignment parses to a single statement.
func TestStringLineContinuationParses(t *testing.T) {
	prog, err := parser.Parse("s = \"a\\\nb\"\np s")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(prog.Body) != 2 {
		t.Fatalf("body = %d statements, want 2", len(prog.Body))
	}
}
