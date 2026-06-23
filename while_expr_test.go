package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// TestWhileExpression covers `while`/`until` in expression position. The value
// of such a loop is nil unless broken with a value; the parser produces the same
// *ast.While used in statement position (until still desugars to a negated
// while), so no parser-driven VM change is needed.
func TestWhileExpression(t *testing.T) {
	cases := []struct {
		name string
		src  string
		// neg is true when the produced While must have a negated condition
		// (the until desugaring).
		neg bool
	}{
		{"paren while", "x = (while i < 1; i += 1; end)", false},
		{"paren until", "x = (until i > 1; i += 1; end)", true},
		{"bare while rhs", "x = while i < 1 do i += 1 end", false},
		{"bare until rhs", "x = until i > 1 do i += 1 end", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prog, err := parser.Parse(c.src)
			if err != nil {
				t.Fatalf("Parse(%q): %v", c.src, err)
			}
			w := whileNode(t, prog.Body[0])
			_, negated := w.Cond.(*ast.UnaryExpr)
			if negated != c.neg {
				t.Errorf("%q: cond negated=%v, want %v (%#v)", c.src, negated, c.neg, w.Cond)
			}
		})
	}
}

// whileNode reaches the *ast.While from an assignment value (possibly wrapped in
// the parenthesized primary, which the parser unwraps to the inner expression).
func whileNode(t *testing.T, n ast.Node) *ast.While {
	t.Helper()
	switch v := n.(type) {
	case *ast.While:
		return v
	case *ast.Assign:
		return whileNode(t, v.Value)
	}
	t.Fatalf("no *ast.While reachable from %T", n)
	return nil
}

// TestWhileStatementUnchanged: statement-position while/until still parse to the
// same *ast.While, and the `begin ... end while a` do-while modifier is
// unaffected.
func TestWhileStatementUnchanged(t *testing.T) {
	for _, src := range []string{
		"while a\n  b\nend",
		"until a\n  b\nend",
		"begin\n  x\nend while a",
	} {
		prog, err := parser.Parse(src)
		if err != nil {
			t.Fatalf("Parse(%q): %v", src, err)
		}
		if _, ok := prog.Body[0].(*ast.While); !ok {
			t.Errorf("%q: top node %T, want *ast.While", src, prog.Body[0])
		}
	}
}
