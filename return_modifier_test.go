package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// TestReturnModifier covers a value-less return/break/next followed by a trailing
// modifier (`return if c`, `return unless c`, `return while c`, …). MRI parses
// `return if c` as a bare return wrapped by the modifier, and rejects
// `return if c then … end` outright — so the if/unless after a bare return is
// always the MODIFIER, never the return value. (Regression: parseReturn used to
// treat `unless`/`if` as the start of the return value and swallow the matching
// `end`, which broke ordinary `return unless x` guard clauses.)
func TestReturnModifier(t *testing.T) {
	isReturn := func(n ast.Node) bool { r, ok := n.(*ast.Return); return ok && r.Value == nil }
	isBreak := func(n ast.Node) bool { b, ok := n.(*ast.Break); return ok && b.Value == nil }
	isNext := func(n ast.Node) bool { x, ok := n.(*ast.Next); return ok && x.Value == nil }

	cases := []struct {
		name    string
		src     string
		wantNeg bool // condition negated (unless / until)
		isLoop  bool // while/until => *ast.While, else => *ast.If
		inner   func(ast.Node) bool
	}{
		{"return if", "return if c", false, false, isReturn},
		{"return unless", "return unless c", true, false, isReturn},
		{"return while", "return while c", false, true, isReturn},
		{"return until", "return until c", true, true, isReturn},
		{"break if", "break if c", false, false, isBreak},
		{"next unless", "next unless c", true, false, isNext},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prog, err := parser.Parse(tc.src)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tc.src, err)
			}
			var body []ast.Node
			var cond ast.Node
			switch n := prog.Body[0].(type) {
			case *ast.If:
				if tc.isLoop {
					t.Fatalf("got *ast.If, want *ast.While")
				}
				body, cond = n.Then, n.Cond
			case *ast.While:
				if !tc.isLoop {
					t.Fatalf("got *ast.While, want *ast.If")
				}
				body, cond = n.Body, n.Cond
			default:
				t.Fatalf("Body[0] = %T, want If/While", n)
			}
			if len(body) != 1 || !tc.inner(body[0]) {
				t.Fatalf("modifier body = %v, want the bare value-less keyword", body)
			}
			if _, neg := cond.(*ast.UnaryExpr); neg != tc.wantNeg {
				t.Fatalf("condition negated=%v, want %v", neg, tc.wantNeg)
			}
		})
	}

	// A bare return on its own line still parses to a value-less Return.
	prog, err := parser.Parse("return")
	if err != nil {
		t.Fatalf("Parse(return): %v", err)
	}
	if r, ok := prog.Body[0].(*ast.Return); !ok || r.Value != nil {
		t.Fatalf("Body[0] = %v, want a value-less Return", prog.Body[0])
	}
}
