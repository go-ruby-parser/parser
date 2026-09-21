package parser_test

import (
	"fmt"
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// TestDefinitionIsAPrimary pins MRI's grammar: `k_class cpath superclass
// bodystmt k_end`, `k_class tLSHFT expr_value term bodystmt k_end`,
// `k_module cpath bodystmt k_end` and `defn_head f_arglist bodystmt k_end` are
// all alternatives of `primary` (parse.y v3_4_0), so a definition composes with
// everything a primary composes with — not only a trailing `.method` and a
// statement modifier, but binary operators too. `class C; end.should == nil` is
// the shape ruby/spec uses to pin a class body's value.
func TestDefinitionIsAPrimary(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		src  string
	}{
		{"class compared", `class C; end.should == nil`},
		{"class with a body compared", `class C; 20; end.value == 20`},
		{"singleton class compared", `class << self; :s; end.value == :s`},
		{"module compared", `module M; end.to_s == "M"`},
		{"def compared", `def f; end.to_s == "f"`},
		{"class in a boolean chain", `class C; end.nil? && true`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			n := parseOne(t, tc.src)
			if _, ok := n.(*ast.BinaryExpr); !ok {
				t.Fatalf("Parse(%q): node = %T, want *ast.BinaryExpr", tc.src, n)
			}
		})
	}
}

// TestDefinitionKeepsStatementForms checks the plain statement shapes still
// parse the same way once definitions go through the expression path.
func TestDefinitionKeepsStatementForms(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		src  string
		want any
	}{
		{`class C; end`, (*ast.ClassDef)(nil)},
		{`module M; end`, (*ast.ModuleDef)(nil)},
		{`def f; end`, (*ast.MethodDef)(nil)},
		{`class C; end if true`, (*ast.If)(nil)},
		{`def f; end if true`, (*ast.If)(nil)},
		{`def f; end.tap { |x| x }`, (*ast.Call)(nil)},
	} {
		n := parseOne(t, tc.src)
		if got, want := fmt.Sprintf("%T", n), fmt.Sprintf("%T", tc.want); got != want {
			t.Errorf("Parse(%q): node = %s, want %s", tc.src, got, want)
		}
	}
	if _, err := parser.Parse("class C; def m; end; end"); err != nil {
		t.Errorf("nested definition: %v", err)
	}
}
