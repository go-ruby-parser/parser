package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

func parseFor(t *testing.T, src string) *ast.For {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q) returned error: %v", src, err)
	}
	f, ok := prog.Body[0].(*ast.For)
	if !ok {
		t.Fatalf("Parse(%q): node = %T, want *ast.For", src, prog.Body[0])
	}
	return f
}

// TestForVarPlainLocals keeps the simple spellings in Vars, with Target nil.
func TestForVarPlainLocals(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		src  string
		want []string
	}{
		{`for i in 1..3; end`, []string{"i"}},
		{`for a, b in pairs; end`, []string{"a", "b"}},
		{`for i in 1..3 do end`, []string{"i"}},
	} {
		f := parseFor(t, tc.src)
		if f.Target != nil {
			t.Errorf("Parse(%q): Target = %T, want nil", tc.src, f.Target)
		}
		if len(f.Vars) != len(tc.want) {
			t.Fatalf("Parse(%q): Vars = %q, want %q", tc.src, f.Vars, tc.want)
		}
		for i, w := range tc.want {
			if f.Vars[i] != w {
				t.Errorf("Parse(%q): Vars[%d] = %q, want %q", tc.src, i, f.Vars[i], w)
			}
		}
	}
}

// TestForVarIsALhsOrAnMlhs covers MRI's `for_var: lhs | mlhs` (parse.y v3_4_0).
// Every shape a multiple assignment accepts is a loop variable: a non-local, an
// attribute or index target, a `*rest`, a nested group, and the trailing comma
// of `mlhs_head: mlhs_item ','`, which destructures.
func TestForVarIsALhsOrAnMlhs(t *testing.T) {
	t.Parallel()
	t.Run("single non-local target", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			src  string
			want any
		}{
			{`for @v in m; end`, (*ast.IvarRef)(nil)},
			{`for @@v in m; end`, (*ast.CVarRef)(nil)},
			{`for $v in m; end`, (*ast.GVarRef)(nil)},
			{`for C in m; end`, (*ast.ConstRef)(nil)},
			{`for o.a in m; end`, (*ast.Call)(nil)},
			{`for o&.a in m; end`, (*ast.Call)(nil)},
			{`for a[0] in m; end`, (*ast.Call)(nil)},
		} {
			f := parseFor(t, tc.src)
			if len(f.Vars) != 0 {
				t.Errorf("Parse(%q): Vars = %q, want empty", tc.src, f.Vars)
			}
			if got, want := nodeType(f.Target), nodeType(tc.want); got != want {
				t.Errorf("Parse(%q): Target = %s, want %s", tc.src, got, want)
			}
		}
		// The attribute and index forms arrive already assignable.
		if call := parseFor(t, `for o.a in m; end`).Target.(*ast.Call); call.Name != "a=" {
			t.Errorf(`for o.a: target calls %q, want "a="`, call.Name)
		}
		if call := parseFor(t, `for a[0] in m; end`).Target.(*ast.Call); call.Name != "[]=" {
			t.Errorf(`for a[0]: target calls %q, want "[]="`, call.Name)
		}
	})

	t.Run("target lists", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			src   string
			names int
			splat int
		}{
			{`for i, in [[1,2]]; end`, 1, -1},
			{`for i, * in [[1,2]]; end`, 2, 1},
			{`for i, *j in [[1,2]]; end`, 2, 1},
			{`for i, *j, k in [[1,2,3,4]]; end`, 3, 1},
			{`for (i, j, k) in [[1,2,3]]; end`, 3, -1},
			{`for i, (j, k) in [[1,[2,3]]]; end`, 2, -1},
			{`for (i, j), k in [[[1,2],3]]; end`, 2, -1},
			{`for @a, b in m; end`, 2, -1},
		} {
			f := parseFor(t, tc.src)
			if len(f.Vars) != 0 {
				t.Errorf("Parse(%q): Vars = %q, want empty", tc.src, f.Vars)
			}
			ma, ok := f.Target.(*ast.MultiAssign)
			if !ok {
				t.Fatalf("Parse(%q): Target = %T, want *ast.MultiAssign", tc.src, f.Target)
			}
			if ma.Values != nil {
				t.Errorf("Parse(%q): Target carries Values; a for-var drives no right-hand side", tc.src)
			}
			if len(ma.Names) != tc.names {
				t.Errorf("Parse(%q): %d names %q, want %d", tc.src, len(ma.Names), ma.Names, tc.names)
			}
			if ma.SplatIndex != tc.splat {
				t.Errorf("Parse(%q): SplatIndex = %d, want %d", tc.src, ma.SplatIndex, tc.splat)
			}
		}
	})

	t.Run("a trailing comma is not the bare form", func(t *testing.T) {
		t.Parallel()
		// `for i in …` binds the whole element; `for i, in …` destructures it.
		if parseFor(t, `for i in m; end`).Target != nil {
			t.Error("for i in: Target must be nil")
		}
		if parseFor(t, `for i, in m; end`).Target == nil {
			t.Error("for i, in: Target must be set, else the destructuring is lost")
		}
	})
}
