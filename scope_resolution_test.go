package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// scopedConst asserts n is a *ast.ScopedConst with the given trailing name and
// Global flag, returning it for further receiver inspection.
func scopedConst(t *testing.T, n ast.Node, name string, global bool) *ast.ScopedConst {
	t.Helper()
	sc, ok := n.(*ast.ScopedConst)
	if !ok {
		t.Fatalf("node = %T, want *ast.ScopedConst", n)
	}
	if sc.Name != name {
		t.Errorf("ScopedConst.Name = %q, want %q", sc.Name, name)
	}
	if sc.Global != global {
		t.Errorf("ScopedConst.Global = %v, want %v", sc.Global, global)
	}
	return sc
}

func TestScopeResolutionClassName(t *testing.T) {
	n := parseOne(t, "class Foo::Bar; end")
	cd, ok := n.(*ast.ClassDef)
	if !ok {
		t.Fatalf("node = %T, want *ast.ClassDef", n)
	}
	if cd.Name != "Bar" {
		t.Errorf("Name = %q, want Bar", cd.Name)
	}
	sc := scopedConst(t, cd.NamePath, "Bar", false)
	if cr, ok := sc.Recv.(*ast.ConstRef); !ok || cr.Name != "Foo" {
		t.Errorf("NamePath.Recv = %#v, want ConstRef Foo", sc.Recv)
	}
}

func TestScopeResolutionClassDeepName(t *testing.T) {
	n := parseOne(t, "class A::B::C; end")
	cd := n.(*ast.ClassDef)
	if cd.Name != "C" {
		t.Errorf("Name = %q, want C", cd.Name)
	}
	outer := scopedConst(t, cd.NamePath, "C", false)
	inner := scopedConst(t, outer.Recv, "B", false)
	if cr, ok := inner.Recv.(*ast.ConstRef); !ok || cr.Name != "A" {
		t.Errorf("innermost recv = %#v, want ConstRef A", inner.Recv)
	}
}

func TestScopeResolutionLeadingClassName(t *testing.T) {
	n := parseOne(t, "class ::Top; end")
	cd := n.(*ast.ClassDef)
	if cd.Name != "Top" {
		t.Errorf("Name = %q, want Top", cd.Name)
	}
	sc := scopedConst(t, cd.NamePath, "Top", true)
	if sc.Recv != nil {
		t.Errorf("leading-:: NamePath.Recv = %#v, want nil", sc.Recv)
	}
}

func TestScopeResolutionBareClassNameNoPath(t *testing.T) {
	n := parseOne(t, "class Foo; end")
	cd := n.(*ast.ClassDef)
	if cd.Name != "Foo" {
		t.Errorf("Name = %q, want Foo", cd.Name)
	}
	if cd.NamePath != nil {
		t.Errorf("bare class NamePath = %#v, want nil", cd.NamePath)
	}
}

func TestScopeResolutionSuperclassPath(t *testing.T) {
	n := parseOne(t, "class E < Foo::Bar; end")
	cd := n.(*ast.ClassDef)
	if cd.Name != "E" || cd.NamePath != nil {
		t.Errorf("Name/NamePath = %q/%#v, want E/nil", cd.Name, cd.NamePath)
	}
	if cd.Super != "" {
		t.Errorf("Super = %q, want empty (path goes to SuperExpr)", cd.Super)
	}
	scopedConst(t, cd.SuperExpr, "Bar", false)
}

func TestScopeResolutionSuperclassLeading(t *testing.T) {
	n := parseOne(t, "class G < ::StandardError; end")
	cd := n.(*ast.ClassDef)
	sc := scopedConst(t, cd.SuperExpr, "StandardError", true)
	if sc.Recv != nil {
		t.Errorf("SuperExpr.Recv = %#v, want nil", sc.Recv)
	}
}

func TestScopeResolutionSuperclassBare(t *testing.T) {
	n := parseOne(t, "class E < Base; end")
	cd := n.(*ast.ClassDef)
	if cd.Super != "Base" {
		t.Errorf("Super = %q, want Base", cd.Super)
	}
	if cd.SuperExpr != nil {
		t.Errorf("bare superclass SuperExpr = %#v, want nil", cd.SuperExpr)
	}
}

func TestScopeResolutionModuleName(t *testing.T) {
	n := parseOne(t, "module A::B; end")
	md, ok := n.(*ast.ModuleDef)
	if !ok {
		t.Fatalf("node = %T, want *ast.ModuleDef", n)
	}
	if md.Name != "B" {
		t.Errorf("Name = %q, want B", md.Name)
	}
	scopedConst(t, md.NamePath, "B", false)
}

func TestScopeResolutionModuleBareNoPath(t *testing.T) {
	md := parseOne(t, "module M; end").(*ast.ModuleDef)
	if md.Name != "M" || md.NamePath != nil {
		t.Errorf("Name/NamePath = %q/%#v, want M/nil", md.Name, md.NamePath)
	}
}

func TestScopeResolutionLeadingValue(t *testing.T) {
	sc := scopedConst(t, parseOne(t, "::TopLevel"), "TopLevel", true)
	if sc.Recv != nil {
		t.Errorf("Recv = %#v, want nil", sc.Recv)
	}
}

func TestScopeResolutionLeadingChainedValue(t *testing.T) {
	outer := scopedConst(t, parseOne(t, "::Foo::Bar"), "Bar", false)
	inner := scopedConst(t, outer.Recv, "Foo", true)
	if inner.Recv != nil {
		t.Errorf("inner Recv = %#v, want nil (leading ::)", inner.Recv)
	}
}

func TestScopeResolutionDefinedLeading(t *testing.T) {
	n := parseOne(t, "defined?(::Foo)")
	call, ok := n.(*ast.Call)
	if !ok || call.Name != "defined?" {
		t.Fatalf("node = %#v, want defined? call", n)
	}
	scopedConst(t, call.Args[0], "Foo", true)
}

func TestScopeResolutionCommandArg(t *testing.T) {
	// `puts ::Foo` is `puts(::Foo)`, not `(puts)::Foo`.
	call := parseOne(t, "puts ::Foo").(*ast.Call)
	if call.Recv != nil || call.Name != "puts" || len(call.Args) != 1 {
		t.Fatalf("node = %#v, want puts(::Foo)", call)
	}
	scopedConst(t, call.Args[0], "Foo", true)
}

func TestScopeResolutionPostfixStillWorks(t *testing.T) {
	// A space-less `Math::PI` remains a postfix scoped const.
	sc := scopedConst(t, parseOne(t, "Math::PI"), "PI", false)
	if cr, ok := sc.Recv.(*ast.ConstRef); !ok || cr.Name != "Math" {
		t.Errorf("Recv = %#v, want ConstRef Math", sc.Recv)
	}
}

// TestScopeResolutionSingletonClassParsed confirms the singleton-class form
// `class << self` is now parsed (it was previously rejected). Full coverage of
// the node lives in singleton_class_test.go.
func TestScopeResolutionSingletonClassParsed(t *testing.T) {
	prog, err := parser.Parse("class << self; end")
	if err != nil {
		t.Fatalf("class << self: %v", err)
	}
	if _, ok := prog.Body[0].(*ast.SingletonClassDef); !ok {
		t.Fatalf("class << self: top is %T, want *ast.SingletonClassDef", prog.Body[0])
	}
}
