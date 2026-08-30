package parser

import (
	"testing"

	"github.com/go-ruby-parser/parser/ast"
)

// TestParenCallArgAssign covers assignment expressions used as parenthesised
// call arguments (and array elements): the comma separates arguments, so an
// assignment's right-hand side stops at it rather than gathering an implicit
// array. `f(a = 1, b = 2, 3)` is three arguments, not `f(a = [1, b = [2, 3]])`.
// This mirrors the paren-less command-argument and def parameter-default rules,
// which already suppress multiple-assignment detection. Regression for the
// mspec idiom `ec.primitive_convert(src = +"...", dst = +"", nil, 10)`.
func TestParenCallArgAssign(t *testing.T) {
	cases := []struct {
		src   string
		nargs int
	}{
		{`g(s = 1, d = 2, 3)`, 3},
		{`g(a = 1, b = 2)`, 2},
		{`f(x = 1)`, 1},
		{`ec.primitive_convert(src = +"a", dst = +"", nil, 10)`, 4},
		{`f(obj.x = 1, 2)`, 2},
		{`f(a = 1, 2, 3)`, 3},
	}
	for _, c := range cases {
		prog, err := Parse(c.src)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", c.src, err)
		}
		var call *ast.Call
		switch n := prog.Body[0].(type) {
		case *ast.Call:
			call = n
		case *ast.Assign:
			// A leading `x = f(...)` wraps the call as the assignment value.
			call, _ = n.Value.(*ast.Call)
		}
		if call == nil {
			t.Fatalf("%q: expected a call, got %T", c.src, prog.Body[0])
		}
		if len(call.Args) != c.nargs {
			t.Errorf("%q: got %d args, want %d", c.src, len(call.Args), c.nargs)
		}
		// No argument may itself be an implicit-array RHS (the pre-fix defect
		// nested the trailing arguments into the first assignment's value).
		for i, a := range call.Args {
			if asn, ok := a.(*ast.Assign); ok {
				if _, isArr := asn.Value.(*ast.ArrayLit); isArr {
					t.Errorf("%q: arg %d assignment gathered an array RHS", c.src, i)
				}
			}
		}
	}
}

// TestParenArrayElemAssign covers assignment expressions as array-literal
// elements: `[x = 1, 2]` is a two-element array, not a one-element array holding
// `x = [1, 2]`.
func TestParenArrayElemAssign(t *testing.T) {
	prog, err := Parse(`[x = 1, 2, 3]`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	arr, ok := prog.Body[0].(*ast.ArrayLit)
	if !ok {
		t.Fatalf("expected *ast.ArrayLit, got %T", prog.Body[0])
	}
	if len(arr.Elems) != 3 {
		t.Fatalf("got %d elements, want 3", len(arr.Elems))
	}
	asn, ok := arr.Elems[0].(*ast.Assign)
	if !ok {
		t.Fatalf("expected first element *ast.Assign, got %T", arr.Elems[0])
	}
	if _, isArr := asn.Value.(*ast.ArrayLit); isArr {
		t.Fatalf("first element's assignment gathered an array RHS")
	}
}
