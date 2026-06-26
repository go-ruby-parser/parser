package parser

import (
	"testing"

	"github.com/go-ruby-parser/parser/ast"
)

// Assignment to a scope-resolved constant (`A::B = v`, `::Top::X = v`).
func TestScopedConstAssign(t *testing.T) {
	for _, src := range []string{
		`A::B = 1`,
		`Foo::BAR = baz`,
		`::Rack::Cache::MetaStore::RAILS = self`,
	} {
		prog, err := Parse(src)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", src, err)
		}
		sca, ok := prog.Body[0].(*ast.ScopedConstAssign)
		if !ok {
			t.Fatalf("%q: expected *ast.ScopedConstAssign, got %T", src, prog.Body[0])
		}
		if _, ok := sca.Target.(*ast.ScopedConst); !ok {
			t.Fatalf("%q: target=%T, want *ast.ScopedConst", src, sca.Target)
		}
	}
}

// A class superclass may be any expression: a bare constant keeps the plain
// Super name; self / scoped / call superclasses go into SuperExpr.
func TestClassSuperclassExpression(t *testing.T) {
	bare, err := Parse("class A < B\nend")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	cd := bare.Body[0].(*ast.ClassDef)
	if cd.Super != "B" || cd.SuperExpr != nil {
		t.Fatalf("bare super: got Super=%q SuperExpr=%v, want B/nil", cd.Super, cd.SuperExpr)
	}

	for _, c := range []struct {
		src  string
		kind string
	}{
		{"class A < self\nend", "self"},
		{"class A < Struct.new(:x)\nend", "call"},
		{"class A < ns::Base\nend", "scoped"},
	} {
		prog, err := Parse(c.src)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", c.src, err)
		}
		cd := prog.Body[0].(*ast.ClassDef)
		if cd.SuperExpr == nil || cd.Super != "" {
			t.Fatalf("%q: expected SuperExpr set and Super empty, got Super=%q SuperExpr=%T", c.src, cd.Super, cd.SuperExpr)
		}
	}
}
