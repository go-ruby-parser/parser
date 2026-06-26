package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

func methodDef(t *testing.T, src string) *ast.MethodDef {
	t.Helper()
	m, ok := parseOne(t, src).(*ast.MethodDef)
	if !ok {
		t.Fatalf("Parse(%q): node = %T, want *ast.MethodDef", src, parseOne(t, src))
	}
	return m
}

// firstCall returns the first *ast.Call found in a method body.
func firstCall(t *testing.T, m *ast.MethodDef) *ast.Call {
	t.Helper()
	for _, n := range m.Body {
		if c, ok := n.(*ast.Call); ok {
			return c
		}
	}
	t.Fatalf("no call in method body")
	return nil
}

func TestForwardDefAndCall(t *testing.T) {
	m := methodDef(t, "def f(...); g(...); end")
	if !m.Forward {
		t.Error("MethodDef.Forward = false, want true")
	}
	if len(m.Params) != 0 {
		t.Errorf("Params = %v, want empty", m.Params)
	}
	call := firstCall(t, m)
	if len(call.Args) != 1 {
		t.Fatalf("call args = %d, want 1", len(call.Args))
	}
	if _, ok := call.Args[0].(*ast.ForwardArgs); !ok {
		t.Errorf("call arg = %T, want *ast.ForwardArgs", call.Args[0])
	}
}

func TestForwardWithLeadingParam(t *testing.T) {
	m := methodDef(t, "def f(a, ...); g(a, ...); end")
	if !m.Forward {
		t.Error("Forward = false, want true")
	}
	if len(m.Params) != 1 || m.Params[0] != "a" {
		t.Errorf("Params = %v, want [a]", m.Params)
	}
	call := firstCall(t, m)
	if len(call.Args) != 2 {
		t.Fatalf("call args = %d, want 2", len(call.Args))
	}
	if _, ok := call.Args[1].(*ast.ForwardArgs); !ok {
		t.Errorf("last arg = %T, want *ast.ForwardArgs", call.Args[1])
	}
}

func TestForwardWithPositionalThenForward(t *testing.T) {
	m := methodDef(t, "def f(...); g(1, ...); end")
	call := firstCall(t, m)
	if len(call.Args) != 2 {
		t.Fatalf("call args = %d, want 2", len(call.Args))
	}
	if _, ok := call.Args[0].(*ast.IntLit); !ok {
		t.Errorf("arg0 = %T, want *ast.IntLit", call.Args[0])
	}
	if _, ok := call.Args[1].(*ast.ForwardArgs); !ok {
		t.Errorf("arg1 = %T, want *ast.ForwardArgs", call.Args[1])
	}
}

func TestForwardDefOnly(t *testing.T) {
	m := methodDef(t, "def f(...); end")
	if !m.Forward {
		t.Error("Forward = false, want true")
	}
}

func TestForwardParenlessDef(t *testing.T) {
	m := methodDef(t, "def f ...\nend")
	if !m.Forward {
		t.Error("paren-less Forward = false, want true")
	}
}

func TestForwardDoesNotBreakRange(t *testing.T) {
	for _, src := range []string{"(1...5)", "g(1...5)", "a = 1...10", "x[1...3]"} {
		if _, err := parser.Parse(src); err != nil {
			t.Errorf("Parse(%q) returned error: %v", src, err)
		}
	}
}

func TestForwardValidPrograms(t *testing.T) {
	for _, src := range []string{
		"def f(...); g(...); end",
		"def f(a, ...); g(a, ...); end",
		"def f(...); g(1, ...); end",
		"def f(...); end",
	} {
		if _, err := parser.Parse(src); err != nil {
			t.Errorf("Parse(%q) returned error: %v", src, err)
		}
	}
}
