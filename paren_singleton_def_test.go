package parser

import (
	"testing"

	"github.com/go-ruby-parser/parser/ast"
)

// A singleton method definition may name its receiver as a parenthesised
// expression: `def (expr).name … end`. MRI evaluates the group to an arbitrary
// object and defines a singleton method on it, so the receiver can be a local,
// a constant, a method call, `self`, a ternary, and so on. The production must
// emit the identical MethodDef shape as the bare `def obj.name` form — a
// non-singleton def carrying the parsed receiver in Recv — so the compiler
// lowers it with no special case.

// The parenthesised receiver parses to the same Recv node type as the
// equivalent bare form, for every receiver kind the bare form supports.
func TestParenSingletonDefMatchesBareForm(t *testing.T) {
	cases := []struct {
		name string
		bare string // def obj.foo …
		pipe string // def (obj).foo …
		recv string // expected Recv concrete type
	}{
		{
			name: "local receiver",
			bare: "obj = Object.new\ndef obj.foo; 1; end",
			pipe: "obj = Object.new\ndef (obj).foo; 1; end",
			recv: "*ast.VarRef",
		},
		{
			name: "const receiver",
			bare: "def String.foo; 1; end",
			pipe: "def (String).foo; 1; end",
			recv: "*ast.ConstRef",
		},
		{
			name: "self receiver",
			bare: "def self.foo; 1; end",
			pipe: "def (self).foo; 1; end",
			// Bare `def self.foo` is Singleton=true/Recv=nil; the parenthesised
			// `(self)` is a normal self expression, so it is Recv=*ast.SelfLit with
			// Singleton=false. Both lower to a singleton method on self, but the node
			// shapes differ, so this case is asserted separately below.
			recv: "*ast.SelfLit",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pipeProg, err := Parse(c.pipe)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", c.pipe, err)
			}
			pmd, ok := pipeProg.Body[len(pipeProg.Body)-1].(*ast.MethodDef)
			if !ok {
				t.Fatalf("%q: last stmt = %T, want *ast.MethodDef", c.pipe, pipeProg.Body[len(pipeProg.Body)-1])
			}
			if pmd.Singleton {
				t.Fatalf("%q: Singleton=true, want false (receiver lives in Recv)", c.pipe)
			}
			if pmd.Recv == nil {
				t.Fatalf("%q: Recv=nil, want a parsed receiver", c.pipe)
			}
			if got := typeName(pmd.Recv); got != c.recv {
				t.Fatalf("%q: Recv=%s, want %s", c.pipe, got, c.recv)
			}
			if pmd.Name != "foo" {
				t.Fatalf("%q: Name=%q, want foo", c.pipe, pmd.Name)
			}

			// For receiver kinds the bare form also supports (local, const), the
			// parenthesised node must be byte-identical to the bare form's.
			if c.name == "self receiver" {
				return
			}
			bareProg, err := Parse(c.bare)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", c.bare, err)
			}
			bmd := bareProg.Body[len(bareProg.Body)-1].(*ast.MethodDef)
			if typeName(bmd.Recv) != typeName(pmd.Recv) {
				t.Fatalf("%q vs %q: Recv %s != %s", c.bare, c.pipe, typeName(bmd.Recv), typeName(pmd.Recv))
			}
			if !recvEqual(bmd.Recv, pmd.Recv) {
				t.Fatalf("%q vs %q: receivers differ: %#v != %#v", c.bare, c.pipe, bmd.Recv, pmd.Recv)
			}
			if bmd.Singleton != pmd.Singleton {
				t.Fatalf("%q vs %q: Singleton %v != %v", c.bare, c.pipe, bmd.Singleton, pmd.Singleton)
			}
		})
	}
}

// The receiver may be an arbitrary primary expression the bare form cannot
// express — a method-call chain — which is exactly why MRI requires the parens.
func TestParenSingletonDefMethodCallReceiver(t *testing.T) {
	prog, err := Parse("def (foo.bar).baz; 1; end")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	md := prog.Body[0].(*ast.MethodDef)
	if md.Singleton {
		t.Fatalf("Singleton=true, want false")
	}
	call, ok := md.Recv.(*ast.Call)
	if !ok {
		t.Fatalf("Recv=%T, want *ast.Call", md.Recv)
	}
	if call.Name != "bar" {
		t.Fatalf("Recv call Name=%q, want bar", call.Name)
	}
	if _, ok := call.Recv.(*ast.Call); !ok {
		t.Fatalf("Recv.Recv=%T, want *ast.Call (foo)", call.Recv)
	}
	if md.Name != "baz" {
		t.Fatalf("Name=%q, want baz", md.Name)
	}
}

// The parenthesised receiver form carries positional params, keyword params,
// paren-less params, and the endless `= expr` body — the whole `def` grammar
// applies unchanged after the receiver.
func TestParenSingletonDefParamForms(t *testing.T) {
	cases := []struct {
		src        string
		name       string
		params     []string
		kw         []string
		bodyLen    int
	}{
		{src: "def (obj).m(a, b); a; end", name: "m", params: []string{"a", "b"}, bodyLen: 1},
		{src: "def (obj).m(a:); a; end", name: "m", kw: []string{"a"}, bodyLen: 1},
		{src: "def (obj).m(a:, b: 2); a; end", name: "m", kw: []string{"a", "b"}, bodyLen: 1},
		{src: "def (obj).m x, y\n  x\nend", name: "m", params: []string{"x", "y"}, bodyLen: 1},
		{src: "def (String).m = 7", name: "m", bodyLen: 1},
		{src: "def (obj).m; end", name: "m", bodyLen: 0},
	}
	for _, c := range cases {
		t.Run(c.src, func(t *testing.T) {
			prog, err := Parse(c.src)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", c.src, err)
			}
			md := prog.Body[0].(*ast.MethodDef)
			if md.Recv == nil {
				t.Fatalf("%q: Recv=nil, want a parsed receiver", c.src)
			}
			if md.Name != c.name {
				t.Fatalf("%q: Name=%q, want %q", c.src, md.Name, c.name)
			}
			if len(md.Params) != len(c.params) {
				t.Fatalf("%q: Params=%v, want %v", c.src, md.Params, c.params)
			}
			for i, p := range c.params {
				if md.Params[i] != p {
					t.Fatalf("%q: Params[%d]=%q, want %q", c.src, i, md.Params[i], p)
				}
			}
			if len(md.KwParams) != len(c.kw) {
				t.Fatalf("%q: KwParams=%v, want %v", c.src, md.KwParams, c.kw)
			}
			for i, k := range c.kw {
				if md.KwParams[i].Name != k {
					t.Fatalf("%q: KwParams[%d]=%q, want %q", c.src, i, md.KwParams[i].Name, k)
				}
			}
			if len(md.Body) != c.bodyLen {
				t.Fatalf("%q: len(Body)=%d, want %d", c.src, len(md.Body), c.bodyLen)
			}
		})
	}
}

// MRI restricts the group to a single expression: a compound `def (a; b).m`,
// a multi-line group, and an empty group are all syntax errors, so the parser
// rejects them too rather than inventing a Begin-wrapped receiver.
func TestParenSingletonDefRejectsCompoundReceiver(t *testing.T) {
	for _, src := range []string{
		"def (a; b).m; end",
		"def (a\nb).m; end",
		"def ().m; end",
	} {
		if _, err := Parse(src); err == nil {
			t.Fatalf("Parse(%q): expected error, got none", src)
		}
	}
}

// typeName returns the concrete type name of an AST node ("*ast.VarRef", …).
func typeName(n ast.Node) string {
	switch n.(type) {
	case *ast.VarRef:
		return "*ast.VarRef"
	case *ast.ConstRef:
		return "*ast.ConstRef"
	case *ast.SelfLit:
		return "*ast.SelfLit"
	case *ast.Call:
		return "*ast.Call"
	case *ast.IvarRef:
		return "*ast.IvarRef"
	default:
		return "?"
	}
}

// recvEqual compares two receiver nodes structurally for the kinds the bare and
// parenthesised forms share (name-bearing references).
func recvEqual(a, b ast.Node) bool {
	switch av := a.(type) {
	case *ast.VarRef:
		bv, ok := b.(*ast.VarRef)
		return ok && av.Name == bv.Name
	case *ast.ConstRef:
		bv, ok := b.(*ast.ConstRef)
		return ok && av.Name == bv.Name
	}
	return false
}
