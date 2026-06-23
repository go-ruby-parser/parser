package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// topCall parses src and returns the single top-level Call node.
func topCall(t *testing.T, src string) *ast.Call {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	if len(prog.Body) != 1 {
		t.Fatalf("Parse(%q): want 1 top-level node, got %d", src, len(prog.Body))
	}
	call, ok := prog.Body[0].(*ast.Call)
	if !ok {
		t.Fatalf("Parse(%q): top node is %T, want *ast.Call", src, prog.Body[0])
	}
	return call
}

// TestDotCallShorthand covers the `.()` sugar: `recv.(args)` desugars to a call
// to method `call` on the receiver — the same AST a normal `recv.call(...)`
// produces (Name=="call", Recv set, Safe carried).
func TestDotCallShorthand(t *testing.T) {
	cases := []struct {
		src      string
		wantArgs int
	}{
		{"f.()", 0},
		{"f.(5)", 1},
		{"obj.(1, 2)", 2},
		{"f.(1, 2, 3)", 3},
		{"@blk.(x)", 1},
		{"(a + b).(1)", 1},
		{"f.(*args)", 1},
	}
	for _, c := range cases {
		call := topCall(t, c.src)
		if call.Name != "call" {
			t.Errorf("Parse(%q): Name=%q, want \"call\"", c.src, call.Name)
		}
		if call.Recv == nil {
			t.Errorf("Parse(%q): Recv is nil, want receiver", c.src)
		}
		if len(call.Args) != c.wantArgs {
			t.Errorf("Parse(%q): %d args, want %d", c.src, len(call.Args), c.wantArgs)
		}
	}
}

// TestDotCallSameAsExplicitCall asserts `f.(5)` parses identically to the
// explicit `f.call(5)` — i.e. the shorthand is pure desugaring, no new AST.
func TestDotCallSameAsExplicitCall(t *testing.T) {
	a := topCall(t, "f.(5)")
	b := topCall(t, "f.call(5)")
	if a.Name != b.Name || len(a.Args) != len(b.Args) {
		t.Fatalf("f.(5) and f.call(5) differ: %+v vs %+v", a, b)
	}
	// `f` is not a known local, so the receiver is a zero-arg call to `f`
	// (matching `f.call(5)`), not a variable read.
	ra, rb := a.Recv.(*ast.Call), b.Recv.(*ast.Call)
	if ra == nil || rb == nil || ra.Name != "f" || rb.Name != "f" {
		t.Errorf("f.(5)/f.call(5): receivers = %T/%T, want call to f", a.Recv, b.Recv)
	}
}

// TestDotCallChains covers `g.(1).(2)` — the shorthand nests as a postfix chain.
func TestDotCallChains(t *testing.T) {
	outer := topCall(t, "g.(1).(2)")
	if outer.Name != "call" || len(outer.Args) != 1 {
		t.Fatalf("g.(1).(2): outer = %+v", outer)
	}
	inner, ok := outer.Recv.(*ast.Call)
	if !ok {
		t.Fatalf("g.(1).(2): outer.Recv is %T, want *ast.Call", outer.Recv)
	}
	if inner.Name != "call" || len(inner.Args) != 1 {
		t.Fatalf("g.(1).(2): inner = %+v", inner)
	}
	// `g` resolves to a zero-arg call (not a known local).
	g, ok := inner.Recv.(*ast.Call)
	if !ok || g.Name != "g" {
		t.Errorf("g.(1).(2): inner.Recv is %T, want call to g", inner.Recv)
	}
	// Mixed chain: a normal call then a shorthand call.
	mixed := topCall(t, "g.foo.(2)")
	if mixed.Name != "call" {
		t.Errorf("g.foo.(2): Name=%q, want \"call\"", mixed.Name)
	}
}

// TestDotCallSafe covers `f&.()` — the safe-navigation flag must carry through.
func TestDotCallSafe(t *testing.T) {
	call := topCall(t, "f&.(1)")
	if call.Name != "call" || !call.Safe || len(call.Args) != 1 {
		t.Fatalf("f&.(1): %+v (want call/safe/1arg)", call)
	}
}

// TestCommandCallOnMethodResult covers paren-less command arguments on a
// receiver method call: `<primary>.<ident> <arg>` parses <arg> as the argument.
func TestCommandCallOnMethodResult(t *testing.T) {
	cases := []struct {
		src      string
		name     string
		wantArgs int
	}{
		{"Fiber.yield 1", "yield", 1},
		{"obj.foo bar", "foo", 1},
		{"y.yield 10", "yield", 1},
		{"obj.foo 1, 2, 3", "foo", 3},
		{"obj.foo \"a\"", "foo", 1},
		{"obj.foo :sym", "foo", 1},
		{"obj.foo -1", "foo", 1}, // unary-style arg hugs the sign
		{"obj.foo [1, 2]", "foo", 1},
		{"a.b.c d", "c", 1}, // command arg on the tail of a method chain
	}
	for _, c := range cases {
		call := topCall(t, c.src)
		if call.Name != c.name {
			t.Errorf("Parse(%q): Name=%q, want %q", c.src, call.Name, c.name)
		}
		if call.Recv == nil {
			t.Errorf("Parse(%q): Recv is nil, want receiver", c.src)
		}
		if len(call.Args) != c.wantArgs {
			t.Errorf("Parse(%q): %d args, want %d", c.src, len(call.Args), c.wantArgs)
		}
	}
}

// TestCommandCallSafe covers a command argument through safe navigation.
func TestCommandCallSafe(t *testing.T) {
	call := topCall(t, "obj&.foo bar")
	if call.Name != "foo" || !call.Safe || len(call.Args) != 1 {
		t.Fatalf("obj&.foo bar: %+v (want foo/safe/1arg)", call)
	}
}

// TestCommandCallRegressions guards the disambiguations the command-arg path
// must NOT break: operator chains, method chains, newline-terminated calls,
// paren'd calls, and bare attribute reads.
func TestCommandCallRegressions(t *testing.T) {
	// `a.b + c` is an operator expression on the read, not a command call.
	prog, err := parser.Parse("a.b + c")
	if err != nil {
		t.Fatalf("Parse(a.b + c): %v", err)
	}
	if _, ok := prog.Body[0].(*ast.BinaryExpr); !ok {
		t.Errorf("a.b + c: top is %T, want *ast.BinaryExpr", prog.Body[0])
	}

	// `a.b.c` is a method chain with no args on the tail.
	tail := topCall(t, "a.b.c")
	if tail.Name != "c" || len(tail.Args) != 0 {
		t.Errorf("a.b.c: tail = %+v (want c/0args)", tail)
	}
	if _, ok := tail.Recv.(*ast.Call); !ok {
		t.Errorf("a.b.c: tail.Recv is %T, want *ast.Call", tail.Recv)
	}

	// `a.b(c)` stays a paren call (one arg, no command parsing).
	paren := topCall(t, "a.b(c)")
	if paren.Name != "b" || len(paren.Args) != 1 {
		t.Errorf("a.b(c): %+v (want b/1arg)", paren)
	}

	// `a.b` alone is a zero-arg attribute read.
	read := topCall(t, "a.b")
	if read.Name != "b" || len(read.Args) != 0 {
		t.Errorf("a.b: %+v (want b/0args)", read)
	}

	// Newline terminates the call; the next line is a separate statement.
	multi, err := parser.Parse("a.b\nc")
	if err != nil {
		t.Fatalf("Parse(a.b\\nc): %v", err)
	}
	if len(multi.Body) != 2 {
		t.Errorf("a.b\\nc: %d statements, want 2", len(multi.Body))
	}

	// Attribute assignment is unaffected (`=` is not a command-arg starter).
	asn := topCall(t, "a.b = 1")
	if asn.Name != "b=" || len(asn.Args) != 1 {
		t.Errorf("a.b = 1: %+v (want b=/1arg)", asn)
	}
}

// TestCallSyntaxErrors covers the error branches: a `.` followed by neither a
// method name nor `(` still fails as before.
func TestCallSyntaxErrors(t *testing.T) {
	bad := []string{
		"f.",    // nothing after the dot
		"f.123", // a numeric literal is not a method name
	}
	for _, src := range bad {
		if _, err := parser.Parse(src); err == nil {
			t.Errorf("Parse(%q): expected error, got nil", src)
		}
	}
}
