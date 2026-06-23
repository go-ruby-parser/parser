package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// TestIfExpression covers `if`/`unless` used in expression position (RHS of an
// assignment, a call argument, a return value). The produced node is the same
// *ast.If used in statement position — `unless` still desugars to an `if` with a
// negated condition.
func TestIfExpression(t *testing.T) {
	cases := []struct {
		name string
		src  string
		// check inspects the *ast.If reached from the program body.
		check func(t *testing.T, n *ast.If)
	}{
		{
			name: "assign rhs if",
			src:  "x = if c then 1 else 2 end",
			check: func(t *testing.T, n *ast.If) {
				if len(n.Then) != 1 || len(n.Else) != 1 {
					t.Errorf("then=%d else=%d, want 1/1", len(n.Then), len(n.Else))
				}
			},
		},
		{
			name: "call arg if",
			src:  "foo(if c then 1 else 2 end)",
			check: func(t *testing.T, n *ast.If) {
				if n.Else == nil {
					t.Errorf("expected an else branch")
				}
			},
		},
		{
			name: "return if",
			src:  "return if c then 1 else 2 end",
			check: func(t *testing.T, n *ast.If) {
				if len(n.Then) != 1 {
					t.Errorf("then=%d, want 1", len(n.Then))
				}
			},
		},
		{
			name: "assign rhs unless",
			src:  "y = unless c then 1 else 2 end",
			check: func(t *testing.T, n *ast.If) {
				// unless desugars to `if !c`.
				if _, ok := n.Cond.(*ast.UnaryExpr); !ok {
					t.Errorf("Cond=%T, want a negated UnaryExpr", n.Cond)
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prog, err := parser.Parse(c.src)
			if err != nil {
				t.Fatalf("Parse(%q): %v", c.src, err)
			}
			c.check(t, ifNode(t, prog.Body[0]))
		})
	}
}

// ifNode extracts the *ast.If from an assignment value, a single call argument,
// a return value, or the node itself.
func ifNode(t *testing.T, n ast.Node) *ast.If {
	t.Helper()
	switch v := n.(type) {
	case *ast.If:
		return v
	case *ast.Assign:
		return ifNode(t, v.Value)
	case *ast.Return:
		return ifNode(t, v.Value)
	case *ast.Call:
		if len(v.Args) != 1 {
			t.Fatalf("call has %d args, want 1", len(v.Args))
		}
		return ifNode(t, v.Args[0])
	}
	t.Fatalf("no *ast.If reachable from %T", n)
	return nil
}

// TestIfStatementUnchanged: statement-position if/unless still parse and still
// yield the same *ast.If node.
func TestIfStatementUnchanged(t *testing.T) {
	for _, src := range []string{
		"if a\n  b\nelsif c\n  d\nelse\n  e\nend",
		"unless a\n  b\nelse\n  c\nend",
		"if a then b else c end",
	} {
		prog, err := parser.Parse(src)
		if err != nil {
			t.Fatalf("Parse(%q): %v", src, err)
		}
		if _, ok := prog.Body[0].(*ast.If); !ok {
			t.Errorf("%q: top node %T, want *ast.If", src, prog.Body[0])
		}
	}
}
