package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// parseOne parses src and returns its single top-level statement, failing the
// test on any parse error or unexpected body length.
func parseOne(t *testing.T, src string) ast.Node {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q) returned error: %v", src, err)
	}
	if len(prog.Body) != 1 {
		t.Fatalf("Parse(%q): want 1 statement, got %d", src, len(prog.Body))
	}
	return prog.Body[0]
}

// commandArg returns the i-th argument of a paren-less command call to name.
func commandArg(t *testing.T, src, name string, i int) ast.Node {
	t.Helper()
	n := parseOne(t, src)
	call, ok := n.(*ast.Call)
	if !ok {
		t.Fatalf("Parse(%q): top node = %T, want *ast.Call", src, n)
	}
	if call.Name != name {
		t.Fatalf("Parse(%q): call name = %q, want %q", src, call.Name, name)
	}
	if i >= len(call.Args) {
		t.Fatalf("Parse(%q): only %d args, wanted arg %d", src, len(call.Args), i)
	}
	return call.Args[i]
}

// wantStrings asserts that n is an ArrayLit of StringLits with the given values.
func wantStrings(t *testing.T, src string, n ast.Node, want []string) {
	t.Helper()
	arr, ok := n.(*ast.ArrayLit)
	if !ok {
		t.Fatalf("%q: node = %T, want *ast.ArrayLit", src, n)
	}
	if len(arr.Elems) != len(want) {
		t.Fatalf("%q: %d elems, want %d", src, len(arr.Elems), len(want))
	}
	for i, w := range want {
		s, ok := arr.Elems[i].(*ast.StringLit)
		if !ok {
			t.Fatalf("%q: elem %d = %T, want *ast.StringLit", src, i, arr.Elems[i])
		}
		if s.Value != w {
			t.Errorf("%q: elem %d = %q, want %q", src, i, s.Value, w)
		}
	}
}

// wantSymbols asserts that n is an ArrayLit of SymbolLits with the given names.
func wantSymbols(t *testing.T, src string, n ast.Node, want []string) {
	t.Helper()
	arr, ok := n.(*ast.ArrayLit)
	if !ok {
		t.Fatalf("%q: node = %T, want *ast.ArrayLit", src, n)
	}
	if len(arr.Elems) != len(want) {
		t.Fatalf("%q: %d elems, want %d", src, len(arr.Elems), len(want))
	}
	for i, w := range want {
		s, ok := arr.Elems[i].(*ast.SymbolLit)
		if !ok {
			t.Fatalf("%q: elem %d = %T, want *ast.SymbolLit", src, i, arr.Elems[i])
		}
		if s.Name != w {
			t.Errorf("%q: elem %d name = %q, want %q", src, i, s.Name, w)
		}
	}
}

// TestPercentWordArrayCommandArg covers %w/%i in paren-less command-argument
// position: the literal must become the call's argument, not a modulo operand.
func TestPercentWordArrayCommandArg(t *testing.T) {
	wantStrings(t, "p %w[a b c]", commandArg(t, "p %w[a b c]", "p", 0), []string{"a", "b", "c"})
	wantStrings(t, "p %w(x y)", commandArg(t, "p %w(x y)", "p", 0), []string{"x", "y"})
	wantSymbols(t, "p %i[a b]", commandArg(t, "p %i[a b]", "p", 0), []string{"a", "b"})
	wantSymbols(t, "p %i{x y z}", commandArg(t, "p %i{x y z}", "p", 0), []string{"x", "y", "z"})
}

// TestPercentStringForms covers %q/%Q/%() string literals in both assignment
// (exprBegin) and command-argument (exprEnd) positions.
func TestPercentStringForms(t *testing.T) {
	// %q is single-quote semantics: no interpolation.
	if s, ok := parseRHS(t, `x = %q(a#{1}b)`).(*ast.StringLit); !ok || s.Value != "a#{1}b" {
		t.Errorf(`%%q assign: got %#v, want StringLit "a#{1}b"`, parseRHS(t, `x = %q(a#{1}b)`))
	}
	if s, ok := commandArg(t, "p %q(a#{1}b)", "p", 0).(*ast.StringLit); !ok || s.Value != "a#{1}b" {
		t.Errorf(`%%q cmd-arg: got %#v, want StringLit "a#{1}b"`, commandArg(t, "p %q(a#{1}b)", "p", 0))
	}
	// %q escapes: only \\ and the delimiters.
	if s, ok := parseRHS(t, `x = %q[a\]b]`).(*ast.StringLit); !ok || s.Value != "a]b" {
		t.Errorf(`%%q escape: got %#v, want StringLit "a]b"`, parseRHS(t, `x = %q[a\]b]`))
	}

	// %Q and bare %() are double-quote semantics: interpolating.
	for _, src := range []string{`x = %Q(a#{1}b)`, `x = %(a#{1}b)`} {
		if _, ok := parseRHS(t, src).(*ast.StrInterp); !ok {
			t.Errorf("%q: RHS = %T, want *ast.StrInterp", src, parseRHS(t, src))
		}
	}
	// A non-interpolating %Q body is a plain StringLit.
	if s, ok := parseRHS(t, `x = %Q(plain)`).(*ast.StringLit); !ok || s.Value != "plain" {
		t.Errorf(`%%Q plain: got %#v, want StringLit "plain"`, parseRHS(t, `x = %Q(plain)`))
	}
	// Command-argument position.
	if _, ok := commandArg(t, "p %Q(a#{1}b)", "p", 0).(*ast.StrInterp); !ok {
		t.Errorf("%%Q cmd-arg: got %T, want *ast.StrInterp", commandArg(t, "p %Q(a#{1}b)", "p", 0))
	}
	if _, ok := commandArg(t, "p %(a#{1}b)", "p", 0).(*ast.StrInterp); !ok {
		t.Errorf("%%() cmd-arg: got %T, want *ast.StrInterp", commandArg(t, "p %(a#{1}b)", "p", 0))
	}
}

// TestPercentInterpArrays covers %W/%I interpolating word/symbol arrays.
func TestPercentInterpArrays(t *testing.T) {
	// %W with no interpolation degrades to plain string elements.
	wantStrings(t, "x = %W[a b]", parseRHS(t, "x = %W[a b]"), []string{"a", "b"})
	// %W with interpolation: the interpolating element is a StrInterp.
	arr, ok := parseRHS(t, "x = %W[a#{1} b]").(*ast.ArrayLit)
	if !ok || len(arr.Elems) != 2 {
		t.Fatalf("%%W interp: got %#v", parseRHS(t, "x = %W[a#{1} b]"))
	}
	if _, ok := arr.Elems[0].(*ast.StrInterp); !ok {
		t.Errorf("%%W interp: elem 0 = %T, want *ast.StrInterp", arr.Elems[0])
	}
	if s, ok := arr.Elems[1].(*ast.StringLit); !ok || s.Value != "b" {
		t.Errorf("%%W interp: elem 1 = %#v, want StringLit \"b\"", arr.Elems[1])
	}

	// %I with no interpolation is a plain symbol array.
	wantSymbols(t, "x = %I[a b]", parseRHS(t, "x = %I[a b]"), []string{"a", "b"})

	// Command-argument position for %W/%I.
	if _, ok := commandArg(t, "p %W[a#{1} b]", "p", 0).(*ast.ArrayLit); !ok {
		t.Errorf("%%W cmd-arg: got %T, want *ast.ArrayLit", commandArg(t, "p %W[a#{1} b]", "p", 0))
	}
	if _, ok := commandArg(t, "p %I[a#{1}]", "p", 0).(*ast.ArrayLit); !ok {
		t.Errorf("%%I cmd-arg: got %T, want *ast.ArrayLit", commandArg(t, "p %I[a#{1}]", "p", 0))
	}
}

// TestPercentModuloStaysModulo guards the disambiguation: a '%' that is not a
// percent-literal start must keep being parsed as the modulo operator.
func TestPercentModuloStaysModulo(t *testing.T) {
	for _, src := range []string{
		"5 % 2", // operator between two operands
		"a % b", // space on both sides
		"a %b",  // space before, but %b is not a valid percent-literal
		"p % 2", // space before and after -> modulo on the result of p
	} {
		n := parseOne(t, src)
		if be, ok := unwrapBinary(n); !ok || be.Op != "%" {
			t.Errorf("%q: want a '%%' BinaryExpr, got %T", src, n)
		}
	}
	// Same, on an assignment RHS.
	if be, ok := unwrapBinary(parseRHS(t, "x = 5 % 2")); !ok || be.Op != "%" {
		t.Errorf(`"x = 5 %% 2": want a '%%' BinaryExpr RHS, got %#v`, parseRHS(t, "x = 5 % 2"))
	}
}

// TestPercentLiteralErrors covers the unterminated/edge error branches.
func TestPercentLiteralErrors(t *testing.T) {
	for _, src := range []string{
		"p %w[a b c",     // unterminated %w array in command position
		"p %q(unterm",    // unterminated %q string in command position
		"p %Q(a#{1}",     // unterminated interpolating %Q
		"p %W[a#{1}",     // unterminated interpolating %W
		"%w[a b",         // unterminated %w array
		"%q{nested {ok}", // unbalanced nested delimiters
	} {
		if _, err := parser.Parse(src); err == nil {
			t.Errorf("expected a parse error for %q, got none", src)
		}
	}
}

// parseRHS parses `x = <expr>` and returns the assignment's value node.
func parseRHS(t *testing.T, src string) ast.Node {
	t.Helper()
	n := parseOne(t, src)
	a, ok := n.(*ast.Assign)
	if !ok {
		t.Fatalf("Parse(%q): top node = %T, want *ast.Assign", src, n)
	}
	return a.Value
}

// unwrapBinary returns the BinaryExpr at the root of n (the parser may wrap a
// command call's modulo so the outermost node is the BinaryExpr itself).
func unwrapBinary(n ast.Node) (*ast.BinaryExpr, bool) {
	be, ok := n.(*ast.BinaryExpr)
	return be, ok
}
