package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// TestMultilineTernary covers a ternary `cond ? then : else` whose arms and
// separators are split across physical lines. MRI (`ruby` 4.0.x) allows newlines
// after `?`, on either side of `:`, and before the else arm, so every source
// below is ONE statement desugaring to a single *ast.If. The lexer already joins
// a line ending in `?` or `:` (line-continuation); the parser additionally skips
// newlines at each ternary seam so the newline *before* the `:` — the one the
// lexer cannot know to swallow — is tolerated too. Each case was cross-checked
// against MRI 4.0.5.
func TestMultilineTernary(t *testing.T) {
	tests := []struct {
		name string
		src  string
		then string // expected StringLit value of the then arm
		els  string // expected StringLit value of the else arm
	}{
		{
			// Newline after `?` and after `:` (both lexer-joined): the
			// pre-existing supported shape, kept as a regression guard.
			name: "after-question-and-colon",
			src:  "x = c ?\n  \"a\" :\n  \"b\"\n",
			then: "a", els: "b",
		},
		{
			// Newline *before* the `:` — the shape found while authoring the
			// Ruby wasmdesk client; this is what previously failed to parse.
			name: "before-colon",
			src:  "y = flag ?\n  \"yes\"\n: \"no\"\n",
			then: "yes", els: "no",
		},
		{
			// Newlines on both sides of `:` at once.
			name: "around-colon",
			src:  "z = c ?\n  \"t\"\n:\n  \"e\"\n",
			then: "t", els: "e",
		},
		{
			// Blank lines everywhere inside the ternary (MRI tolerates them).
			name: "blank-lines",
			src:  "w = c ?\n\n  \"p\"\n\n:\n\n  \"q\"\n",
			then: "p", els: "q",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog, err := parser.Parse(tt.src)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.src, err)
			}
			if len(prog.Body) != 1 {
				t.Fatalf("Parse(%q): %d statements, want 1", tt.src, len(prog.Body))
			}
			n := ifNode(t, prog.Body[0])
			if got := strOf(t, n.Then); got != tt.then {
				t.Errorf("then arm = %q, want %q", got, tt.then)
			}
			if got := strOf(t, n.Else); got != tt.els {
				t.Errorf("else arm = %q, want %q", got, tt.els)
			}
			if len(n.Elsifs) != 0 {
				t.Errorf("ternary produced %d elsifs, want 0", len(n.Elsifs))
			}
		})
	}
}

// strOf returns the value of a single StringLit body arm.
func strOf(t *testing.T, body []ast.Node) string {
	t.Helper()
	if len(body) != 1 {
		t.Fatalf("arm has %d nodes, want 1", len(body))
	}
	s, ok := body[0].(*ast.StringLit)
	if !ok {
		t.Fatalf("arm node %T, want *ast.StringLit", body[0])
	}
	return s.Value
}

// TestMultilineTernaryNegatives pins the boundaries the fix must NOT cross.
func TestMultilineTernaryNegatives(t *testing.T) {
	// A newline BEFORE the `?` terminates the condition first, so `?` opening
	// the next line is a syntax error — exactly as MRI 4.0.5 rejects it. The
	// newline-skipping only happens once a `?` has already been consumed.
	for _, src := range []string{
		"x = (1 > 0)\n? 1 : 2\n",
		"a = 1\n? 2 : 3\n",
	} {
		if _, err := parser.Parse(src); err == nil {
			t.Errorf("Parse(%q): want error (newline before ? is not a ternary)", src)
		}
	}

	// Non-ternary uses of `?` and `?`-suffixed names keep parsing unchanged.
	for _, src := range []string{
		"w = ?a\n",    // ?c character literal
		"foo?\n",      // predicate method call
		"c ? a : b\n", // single-line ternary
		"x.empty?\n",  // predicate method on a receiver
	} {
		if _, err := parser.Parse(src); err != nil {
			t.Errorf("Parse(%q): unexpected error %v", src, err)
		}
	}
}
