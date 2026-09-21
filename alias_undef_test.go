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

// TestDynamicSymbolFitem covers a dynamic symbol as an alias/undef name. MRI's
// `fitem: fname | symbol` and `symbol: ssym | dsym` (parse.y v3_4_0) admit
// `dsym: tSYMBEG string_contents tSTRING_END`, so `alias :"#{x}" y` names the
// new method by an expression evaluated at run time. The lexer desugars an
// interpolated symbol to `"…".to_sym`, which is the node that lands in
// Alias.NewNameExpr / Undef.Exprs; the matching string is left empty.
func TestDynamicSymbolFitem(t *testing.T) {
	t.Parallel()

	t.Run("alias new name", func(t *testing.T) {
		t.Parallel()
		a := parseAlias(t, `alias :"#{'a'}" value`)
		if a.NewName != "" || a.NewNameExpr == nil {
			t.Errorf("NewName = %q, NewNameExpr = %v; want \"\" and a node", a.NewName, a.NewNameExpr)
		}
		if a.OldName != "value" || a.OldNameExpr != nil {
			t.Errorf("OldName = %q, OldNameExpr = %v; want \"value\" and nil", a.OldName, a.OldNameExpr)
		}
		wantToSym(t, a.NewNameExpr)
	})

	t.Run("alias old name", func(t *testing.T) {
		t.Parallel()
		a := parseAlias(t, `alias a :"#{'value'}"`)
		if a.NewName != "a" || a.NewNameExpr != nil {
			t.Errorf("NewName = %q, NewNameExpr = %v; want \"a\" and nil", a.NewName, a.NewNameExpr)
		}
		if a.OldName != "" || a.OldNameExpr == nil {
			t.Errorf("OldName = %q, OldNameExpr = %v; want \"\" and a node", a.OldName, a.OldNameExpr)
		}
	})

	t.Run("alias both names", func(t *testing.T) {
		t.Parallel()
		a := parseAlias(t, `alias :"#{x}" :"#{y}"`)
		if a.NewNameExpr == nil || a.OldNameExpr == nil {
			t.Errorf("want both names dynamic, got %v / %v", a.NewNameExpr, a.OldNameExpr)
		}
	})

	t.Run("static alias keeps Expr nil", func(t *testing.T) {
		t.Parallel()
		a := parseAlias(t, `alias foo bar`)
		if a.NewNameExpr != nil || a.OldNameExpr != nil {
			t.Errorf("static alias carries expressions: %v / %v", a.NewNameExpr, a.OldNameExpr)
		}
	})

	t.Run("undef mixed list", func(t *testing.T) {
		t.Parallel()
		u := parseUndef(t, `undef :"#{a}", :b`)
		if got := []string{"", "b"}; len(u.Names) != 2 || u.Names[0] != got[0] || u.Names[1] != got[1] {
			t.Fatalf("Names = %q, want %q", u.Names, got)
		}
		if len(u.Exprs) != 2 || u.Exprs[0] == nil || u.Exprs[1] != nil {
			t.Fatalf("Exprs = %v, want [node nil]", u.Exprs)
		}
		wantToSym(t, u.Exprs[0])
	})

	t.Run("static undef leaves Exprs nil", func(t *testing.T) {
		t.Parallel()
		u := parseUndef(t, `undef a, b`)
		if u.Exprs != nil {
			t.Errorf("Exprs = %v, want nil", u.Exprs)
		}
	})

	t.Run("a plain interpolated string is not a name", func(t *testing.T) {
		t.Parallel()
		if _, err := Parse(`alias "#{a}" b`); err == nil {
			t.Error(`alias "#{a}" b: want a parse error, got none`)
		}
	})
}

func parseAlias(t *testing.T, src string) *ast.Alias {
	t.Helper()
	prog, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q) error: %v", src, err)
	}
	a, ok := prog.Body[0].(*ast.Alias)
	if !ok {
		t.Fatalf("Parse(%q): node = %T, want *ast.Alias", src, prog.Body[0])
	}
	return a
}

func parseUndef(t *testing.T, src string) *ast.Undef {
	t.Helper()
	prog, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q) error: %v", src, err)
	}
	u, ok := prog.Body[0].(*ast.Undef)
	if !ok {
		t.Fatalf("Parse(%q): node = %T, want *ast.Undef", src, prog.Body[0])
	}
	return u
}

// wantToSym asserts n is the `"…".to_sym` call the lexer desugars an
// interpolated symbol into.
func wantToSym(t *testing.T, n ast.Node) {
	t.Helper()
	call, ok := n.(*ast.Call)
	if !ok {
		t.Fatalf("dynamic name = %T, want *ast.Call", n)
	}
	if call.Name != "to_sym" {
		t.Errorf("dynamic name calls %q, want %q", call.Name, "to_sym")
	}
	if call.Recv == nil {
		t.Error("dynamic name has no receiver")
	}
	if len(call.Args) != 0 {
		t.Errorf("dynamic name got %d arguments, want 0", len(call.Args))
	}
}
