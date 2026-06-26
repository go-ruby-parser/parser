package parser

import (
	"testing"

	"github.com/go-ruby-parser/parser/ast"
)

// A single assignment target with multiple comma-separated right-hand values is
// an implicit array assignment: `x = 1, 2, 3` ≡ `x = [1, 2, 3]` (MRI). The RHS
// may also carry a `*splat`.
func TestRhsArrayAssign(t *testing.T) {
	cases := []struct {
		src   string
		elems int
	}{
		{`args = "-t", "--server", master`, 3},
		{`x = 1, 2, 3`, 3},
		{`a = *x, y`, 2},
		{`a = 1, 2,`, 2}, // trailing comma
	}
	for _, c := range cases {
		prog, err := Parse(c.src)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", c.src, err)
		}
		asn, ok := prog.Body[0].(*ast.Assign)
		if !ok {
			t.Fatalf("%q: expected *ast.Assign, got %T", c.src, prog.Body[0])
		}
		arr, ok := asn.Value.(*ast.ArrayLit)
		if !ok {
			t.Fatalf("%q: expected RHS *ast.ArrayLit, got %T", c.src, asn.Value)
		}
		if len(arr.Elems) != c.elems {
			t.Fatalf("%q: got %d elems, want %d", c.src, len(arr.Elems), c.elems)
		}
	}
}

// A bare leading `*splat` with no comma still yields a one-element array
// (`a = *list` ≡ `a = [*list]`).
func TestRhsArrayAssignBareSplat(t *testing.T) {
	prog, err := Parse(`a = *list`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	asn := prog.Body[0].(*ast.Assign)
	arr, ok := asn.Value.(*ast.ArrayLit)
	if !ok || len(arr.Elems) != 1 {
		t.Fatalf("expected one-element array, got %#v", asn.Value)
	}
	if _, ok := arr.Elems[0].(*ast.SplatArg); !ok {
		t.Fatalf("expected SplatArg element, got %T", arr.Elems[0])
	}
}

// RHS array assignment applies to constant/ivar/cvar/gvar targets too.
func TestRhsArrayAssignOtherTargets(t *testing.T) {
	for _, src := range []string{
		`FOO = 1, 2`,
		`@x = 1, 2`,
		`@@x = 1, 2`,
		`$x = 1, 2`,
	} {
		if _, err := Parse(src); err != nil {
			t.Fatalf("Parse(%q) error: %v", src, err)
		}
	}
}

// Ordinary chained and single-value assignments are unaffected.
func TestRhsArrayAssignNoRegression(t *testing.T) {
	for _, src := range []string{
		`a = b = 1`,
		`x = foo(1, 2)`,
		`h = {a: 1}`,
	} {
		prog, err := Parse(src)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", src, err)
		}
		asn := prog.Body[0].(*ast.Assign)
		if _, isArr := asn.Value.(*ast.ArrayLit); isArr {
			t.Fatalf("%q: RHS should not be an array", src)
		}
	}
}
