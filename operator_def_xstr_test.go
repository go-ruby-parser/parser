package parser

import (
	"testing"

	"github.com/go-ruby-parser/parser/ast"
)

// A bare `cmd` backtick command literal produces the same XStr node `%x{}` does.
func TestBacktickXString(t *testing.T) {
	cases := []struct {
		src string
		cmd string
	}{
		{"`ls -la`", "ls -la"},
		{"`git rev-parse HEAD`", "git rev-parse HEAD"},
		{"`echo \\`hi\\``", "echo `hi`"}, // escaped backtick
	}
	for _, c := range cases {
		prog, err := Parse(c.src)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", c.src, err)
		}
		x, ok := prog.Body[0].(*ast.XStr)
		if !ok {
			t.Fatalf("%q: expected *ast.XStr, got %T", c.src, prog.Body[0])
		}
		if x.Command != c.cmd {
			t.Fatalf("%q: command=%q, want %q", c.src, x.Command, c.cmd)
		}
	}
}

// A backtick literal works in value position (assignment RHS) too.
func TestBacktickXStringInAssign(t *testing.T) {
	prog, err := Parse("x = `whoami`")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	asn := prog.Body[0].(*ast.Assign)
	if x, ok := asn.Value.(*ast.XStr); !ok || x.Command != "whoami" {
		t.Fatalf("expected RHS XStr `whoami`, got %#v", asn.Value)
	}
}

// An unterminated backtick command is a parse error.
func TestBacktickXStringUnterminated(t *testing.T) {
	if _, err := Parse("x = `oops"); err == nil {
		t.Fatalf("expected error for unterminated backtick")
	}
}

// The full set of operator method names is accepted in `def`, including the
// unary-operator forms `+@`/`-@`/`~@`/`!@` and `def /` (which must not be lexed
// as a regexp).
func TestOperatorMethodDef(t *testing.T) {
	cases := []struct {
		src  string
		name string
	}{
		{"def ===(o); end", "==="},
		{"def ==(o); end", "=="},
		{"def <=>(o); end", "<=>"},
		{"def /(o); end", "/"},
		{"def %(o); end", "%"},
		{"def *(o); end", "*"},
		{"def **(o); end", "**"},
		{"def <<(o); end", "<<"},
		{"def >>(o); end", ">>"},
		{"def &(o); end", "&"},
		{"def |(o); end", "|"},
		{"def ^(o); end", "^"},
		{"def =~(o); end", "=~"},
		{"def [](i); end", "[]"},
		{"def []=(i, v); end", "[]="},
		{"def +(o); end", "+"},
		{"def -(o); end", "-"},
		{"def +@; end", "+@"},
		{"def -@; end", "-@"},
		{"def ~; end", "~"},
		{"def ~@; end", "~@"},
		{"def !; end", "!"},
	}
	for _, c := range cases {
		prog, err := Parse(c.src)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", c.src, err)
		}
		md, ok := prog.Body[0].(*ast.MethodDef)
		if !ok {
			t.Fatalf("%q: expected *ast.MethodDef, got %T", c.src, prog.Body[0])
		}
		if md.Name != c.name {
			t.Fatalf("%q: name=%q, want %q", c.src, md.Name, c.name)
		}
	}
}

// A singleton operator method `def self.===(o)` also parses.
func TestSingletonOperatorMethodDef(t *testing.T) {
	prog, err := Parse("def self.===(o); end")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	md := prog.Body[0].(*ast.MethodDef)
	if !md.Singleton || md.Name != "===" {
		t.Fatalf("got singleton=%v name=%q, want true ===", md.Singleton, md.Name)
	}
}

// `def /` must not break ordinary regexp/division lexing elsewhere.
func TestDefSlashNoRegexpRegression(t *testing.T) {
	for _, src := range []string{
		`x = a / b`,
		`y = /regex/`,
		`foo(/re/)`,
	} {
		if _, err := Parse(src); err != nil {
			t.Fatalf("Parse(%q) error: %v", src, err)
		}
	}
}
