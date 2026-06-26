package parser

import (
	"testing"

	"github.com/go-ruby-parser/parser/ast"
)

// `alias` accepts symbol, operator-symbol, bare-name, and global-variable items.
func TestAlias(t *testing.T) {
	cases := []struct {
		src           string
		newN, oldName string
	}{
		{`alias :== :eql?`, "==", "eql?"},
		{`alias foo bar`, "foo", "bar"},
		{`alias == eql?`, "==", "eql?"},
		{`alias new_name old_name`, "new_name", "old_name"},
		{`alias $x $y`, "$x", "$y"},
	}
	for _, c := range cases {
		prog, err := Parse(c.src)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", c.src, err)
		}
		a, ok := prog.Body[0].(*ast.Alias)
		if !ok {
			t.Fatalf("%q: expected *ast.Alias, got %T", c.src, prog.Body[0])
		}
		if a.NewName != c.newN || a.OldName != c.oldName {
			t.Fatalf("%q: got (%q, %q), want (%q, %q)", c.src, a.NewName, a.OldName, c.newN, c.oldName)
		}
	}
}

// `undef` accepts one or more comma-separated method names.
func TestUndef(t *testing.T) {
	prog, err := Parse(`undef :foo, :bar, baz`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	u, ok := prog.Body[0].(*ast.Undef)
	if !ok {
		t.Fatalf("expected *ast.Undef, got %T", prog.Body[0])
	}
	want := []string{"foo", "bar", "baz"}
	if len(u.Names) != len(want) {
		t.Fatalf("names=%v, want %v", u.Names, want)
	}
	for i, w := range want {
		if u.Names[i] != w {
			t.Fatalf("name %d=%q, want %q", i, u.Names[i], w)
		}
	}
}

// `alias_method` (a plain identifier, not the keyword) is unaffected.
func TestAliasMethodIdentifierUnaffected(t *testing.T) {
	if _, err := Parse(`alias_method :a, :b`); err != nil {
		t.Fatalf("Parse error: %v", err)
	}
}

// A chained multiple-assignment right-hand side (`a, b = c = expr`) is a further
// assignment whose value is then destructured.
func TestChainedMasgnRhs(t *testing.T) {
	for _, src := range []string{
		`a, b = c = [1, 2]`,
		`_, headers, _ = response = call(x)`,
	} {
		prog, err := Parse(src)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", src, err)
		}
		ma, ok := prog.Body[0].(*ast.MultiAssign)
		if !ok {
			t.Fatalf("%q: expected *ast.MultiAssign, got %T", src, prog.Body[0])
		}
		if _, ok := ma.Values[0].(*ast.Assign); !ok {
			t.Fatalf("%q: expected value0 *ast.Assign, got %T", src, ma.Values[0])
		}
	}
}
