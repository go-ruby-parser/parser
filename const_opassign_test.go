package parser

import (
	"testing"

	"github.com/go-ruby-parser/parser/ast"
)

// TestConstOpAssign covers compound assignment to a constant (`C += 1`,
// `C ||= x`) and to a scope-resolved constant (`A::B += 1`, `A::B ||= x`), which
// the parser previously rejected. Arithmetic and `&&=` desugar to `C = C OP x`;
// `||=` is guarded behind `defined?` so it defines an undefined constant instead
// of raising, matching MRI 4.0.6.
func TestConstOpAssign(t *testing.T) {
	for _, src := range []string{
		`C += 1`, `C -= 1`, `C *= 2`, `C **= 2`, `C &&= 1`, `C ||= 1`,
		`A::B += 1`, `A::B ||= 1`, `Foo::Bar::BAZ *= 3`,
		`(x; ConstantSpecs)::OpAssign += 2`,
		`defined?(A += 1)`,
	} {
		if _, err := Parse(src); err != nil {
			t.Errorf("Parse(%q) error: %v", src, err)
		}
	}

	// `C += 1` desugars to `C = C + 1` (a ConstAssign whose value adds a read).
	prog, err := Parse(`C += 1`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	ca, ok := prog.Body[0].(*ast.ConstAssign)
	if !ok {
		t.Fatalf("C += 1: expected *ast.ConstAssign, got %T", prog.Body[0])
	}
	bin, ok := ca.Value.(*ast.BinaryExpr)
	if !ok || bin.Op != "+" {
		t.Fatalf("C += 1: expected value `C + 1`, got %#v", ca.Value)
	}
	if _, ok := bin.Left.(*ast.ConstRef); !ok {
		t.Fatalf("C += 1: expected a ConstRef read on the left, got %T", bin.Left)
	}

	// `C ||= 1` desugars to `(defined?(C) && C) || (C = 1)` so an undefined
	// constant is defined rather than read.
	prog, err = Parse(`C ||= 1`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	or, ok := prog.Body[0].(*ast.BinaryExpr)
	if !ok || or.Op != "||" {
		t.Fatalf("C ||= 1: expected a top-level `||`, got %#v", prog.Body[0])
	}
	guard, ok := or.Left.(*ast.BinaryExpr)
	if !ok || guard.Op != "&&" {
		t.Fatalf("C ||= 1: expected a `defined?(C) && C` guard, got %#v", or.Left)
	}
	call, ok := guard.Left.(*ast.Call)
	if !ok || call.Name != "defined?" {
		t.Fatalf("C ||= 1: expected `defined?(C)` in the guard, got %#v", guard.Left)
	}
	if _, ok := or.Right.(*ast.ConstAssign); !ok {
		t.Fatalf("C ||= 1: expected a ConstAssign on the right, got %T", or.Right)
	}

	// The scoped `||=` form is guarded the same way and targets a ScopedConst.
	prog, err = Parse(`A::B ||= 1`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	or = prog.Body[0].(*ast.BinaryExpr)
	if _, ok := or.Right.(*ast.ScopedConstAssign); !ok {
		t.Fatalf("A::B ||= 1: expected a ScopedConstAssign on the right, got %T", or.Right)
	}
}

// TestPowerAssign covers the `**=` power-assignment operator, which the lexer
// previously split into `**` and `=` (a parse error), for local, instance, and
// constant targets and right-associatively chained.
func TestPowerAssign(t *testing.T) {
	for _, src := range []string{`x **= 2`, `@i **= 2`, `C **= 2`, `a **= b **= 2`} {
		if _, err := Parse(src); err != nil {
			t.Errorf("Parse(%q) error: %v", src, err)
		}
	}
	// `**` and `**=` remain distinct: `x ** 2` is the power operator.
	prog, err := Parse(`x ** 2`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if bin, ok := prog.Body[0].(*ast.BinaryExpr); !ok || bin.Op != "**" {
		t.Fatalf("x ** 2: expected a `**` BinaryExpr, got %#v", prog.Body[0])
	}
}
