package parser

import (
	"testing"

	"github.com/go-ruby-parser/parser/ast"
)

// A `def` may name an explicit receiver that is an instance/class/global
// variable, not just self/local/constant.
func TestDefVariableReceiver(t *testing.T) {
	cases := []struct {
		src  string
		kind string
	}{
		{"def @controller.foo(&b)\nend", "ivar"},
		{"def @@reg.foo\nend", "cvar"},
		{"def $g.foo\nend", "gvar"},
	}
	for _, c := range cases {
		prog, err := Parse(c.src)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", c.src, err)
		}
		md := prog.Body[0].(*ast.MethodDef)
		switch c.kind {
		case "ivar":
			if _, ok := md.Recv.(*ast.IvarRef); !ok {
				t.Fatalf("%q: recv=%T, want *ast.IvarRef", c.src, md.Recv)
			}
		case "cvar":
			if _, ok := md.Recv.(*ast.CVarRef); !ok {
				t.Fatalf("%q: recv=%T, want *ast.CVarRef", c.src, md.Recv)
			}
		case "gvar":
			if _, ok := md.Recv.(*ast.GVarRef); !ok {
				t.Fatalf("%q: recv=%T, want *ast.GVarRef", c.src, md.Recv)
			}
		}
	}
}

// A `def` is a value-producing expression, so it can be a command argument:
// `module_function def server; end`, `private def bar; end`.
func TestDefAsCommandArgument(t *testing.T) {
	for _, src := range []string{
		"module_function def server\n  1\nend",
		"private def bar; end",
	} {
		prog, err := Parse(src)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", src, err)
		}
		call, ok := prog.Body[0].(*ast.Call)
		if !ok {
			t.Fatalf("%q: expected *ast.Call, got %T", src, prog.Body[0])
		}
		if _, ok := call.Args[0].(*ast.MethodDef); !ok {
			t.Fatalf("%q: arg0=%T, want *ast.MethodDef", src, call.Args[0])
		}
	}
}

// A string interpolation body is a full statement, so it admits a trailing
// modifier and several statements.
func TestInterpolationBodyStatements(t *testing.T) {
	for _, src := range []string{
		`"x#{'s' if n > 1}y"`,
		`"v#{a; b}"`,
		`"#{}"`, // empty interpolation
	} {
		if _, err := Parse(src); err != nil {
			t.Fatalf("Parse(%q) error: %v", src, err)
		}
	}
}

// A bare incomplete `def` is still a clean parse error (not an internal panic).
func TestBareDefError(t *testing.T) {
	if _, err := Parse("def"); err == nil {
		t.Fatalf("expected error for bare def")
	}
}
