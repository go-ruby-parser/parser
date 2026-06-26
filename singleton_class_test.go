package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// topSingleton parses src and returns the single top-level SingletonClassDef.
func topSingleton(t *testing.T, src string) *ast.SingletonClassDef {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	if len(prog.Body) != 1 {
		t.Fatalf("Parse(%q): want 1 top-level node, got %d", src, len(prog.Body))
	}
	sc, ok := prog.Body[0].(*ast.SingletonClassDef)
	if !ok {
		t.Fatalf("Parse(%q): top node is %T, want *ast.SingletonClassDef", src, prog.Body[0])
	}
	return sc
}

// TestSingletonClassSelf covers `class << self` — the idiomatic class-method
// form. The target is *SelfLit and the body holds the defined methods.
func TestSingletonClassSelf(t *testing.T) {
	sc := topSingleton(t, "class << self; def x; 1; end; end")
	if _, ok := sc.Target.(*ast.SelfLit); !ok {
		t.Fatalf("Target is %T, want *ast.SelfLit", sc.Target)
	}
	if len(sc.Body) != 1 {
		t.Fatalf("Body has %d nodes, want 1", len(sc.Body))
	}
	md, ok := sc.Body[0].(*ast.MethodDef)
	if !ok || md.Name != "x" {
		t.Fatalf("Body[0] = %T (%+v), want method def x", sc.Body[0], sc.Body[0])
	}
}

// TestSingletonClassSelfNewlines covers the multi-line layout (no `;`).
func TestSingletonClassSelfNewlines(t *testing.T) {
	sc := topSingleton(t, "class << self\n  def x\n    1\n  end\nend")
	if _, ok := sc.Target.(*ast.SelfLit); !ok {
		t.Fatalf("Target is %T, want *ast.SelfLit", sc.Target)
	}
	if len(sc.Body) != 1 {
		t.Fatalf("Body has %d nodes, want 1", len(sc.Body))
	}
}

// TestSingletonClassLocalTarget covers `class << obj` where obj is a local
// variable — the target resolves to a VarRef read.
func TestSingletonClassLocalTarget(t *testing.T) {
	prog, err := parser.Parse("o = Object.new\nclass << o\n  def y; 2; end\nend")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(prog.Body) != 2 {
		t.Fatalf("want 2 top-level nodes, got %d", len(prog.Body))
	}
	sc, ok := prog.Body[1].(*ast.SingletonClassDef)
	if !ok {
		t.Fatalf("Body[1] is %T, want *ast.SingletonClassDef", prog.Body[1])
	}
	vr, ok := sc.Target.(*ast.VarRef)
	if !ok || vr.Name != "o" {
		t.Fatalf("Target = %T (%+v), want VarRef o", sc.Target, sc.Target)
	}
}

// TestSingletonClassConstTarget covers `class << SomeConst` (e.g. a module's
// metaclass) — the target is a constant call/ref expression.
func TestSingletonClassConstTarget(t *testing.T) {
	sc := topSingleton(t, "class << Foo\n  def bar; end\nend")
	if _, ok := sc.Target.(*ast.ConstRef); !ok {
		t.Fatalf("Target is %T, want *ast.ConstRef", sc.Target)
	}
}

// TestSingletonClassExprTarget covers an arbitrary expression target such as a
// method-call result: `class << obj.thing`.
func TestSingletonClassExprTarget(t *testing.T) {
	sc := topSingleton(t, "class << obj.thing\n  def z; end\nend")
	call, ok := sc.Target.(*ast.Call)
	if !ok || call.Name != "thing" || call.Recv == nil {
		t.Fatalf("Target = %T (%+v), want call to thing on a receiver", sc.Target, sc.Target)
	}
}

// TestSingletonClassEmptyBody covers a singleton class with no body statements.
func TestSingletonClassEmptyBody(t *testing.T) {
	sc := topSingleton(t, "class << self\nend")
	if len(sc.Body) != 0 {
		t.Fatalf("Body has %d nodes, want 0", len(sc.Body))
	}
}

// TestSingletonClassNested covers the common `class Foo; class << self; ...`
// nesting used to declare class methods inside a class body.
func TestSingletonClassNested(t *testing.T) {
	prog, err := parser.Parse("class Foo\n  class << self\n    def bar; end\n  end\nend")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cd, ok := prog.Body[0].(*ast.ClassDef)
	if !ok || cd.Name != "Foo" {
		t.Fatalf("top is %T, want ClassDef Foo", prog.Body[0])
	}
	if len(cd.Body) != 1 {
		t.Fatalf("class body has %d nodes, want 1", len(cd.Body))
	}
	if _, ok := cd.Body[0].(*ast.SingletonClassDef); !ok {
		t.Fatalf("class body[0] is %T, want *ast.SingletonClassDef", cd.Body[0])
	}
}
