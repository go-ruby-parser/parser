package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// masgnOf parses src and returns its single MultiAssign node.
func masgnOf(t *testing.T, src string) *ast.MultiAssign {
	t.Helper()
	n := mustParseSingle(t, src)
	ma, ok := n.(*ast.MultiAssign)
	if !ok {
		t.Fatalf("Parse(%q): top node = %T, want *ast.MultiAssign", src, n)
	}
	return ma
}

// TestMasgnNonLocalTargets parses multiple-assignment whose left-hand targets are
// instance/class/global variables, constants, attributes, and index sets — the
// biggest real-world front-end gap. Each form is MRI-valid (`ruby -c`).
func TestMasgnNonLocalTargets(t *testing.T) {
	cases := []struct {
		src       string
		wantNames []string // "" for a non-local target
		wantSplat int
		check     []func(ast.Node) bool // per-target predicate on Targets[i]
	}{
		{
			src:       "@a, @b, @c = x, y, z",
			wantNames: []string{"", "", ""},
			wantSplat: -1,
			check: []func(ast.Node) bool{
				isIvar("@a"), isIvar("@b"), isIvar("@c"),
			},
		},
		{
			src:       "@@a, @@b = 1, 2",
			wantNames: []string{"", ""},
			wantSplat: -1,
			check:     []func(ast.Node) bool{isCvar("@@a"), isCvar("@@b")},
		},
		{
			src:       "$a, $b = 1, 2",
			wantNames: []string{"", ""},
			wantSplat: -1,
			check:     []func(ast.Node) bool{isGvar("$a"), isGvar("$b")},
		},
		{
			src:       "x, @y, $z = 1, 2, 3",
			wantNames: []string{"x", "", ""},
			wantSplat: -1,
			check:     []func(ast.Node) bool{isVar("x"), isIvar("@y"), isGvar("$z")},
		},
		{
			src:       "A, B = 1, 2",
			wantNames: []string{"", ""},
			wantSplat: -1,
			check:     []func(ast.Node) bool{isConst("A"), isConst("B")},
		},
		{
			src:       "@a, *@b = list",
			wantNames: []string{"", ""},
			wantSplat: 1,
			check:     []func(ast.Node) bool{isIvar("@a"), isIvar("@b")},
		},
		{
			src:       "*@a, @b = list",
			wantNames: []string{"", ""},
			wantSplat: 0,
			check:     []func(ast.Node) bool{isIvar("@a"), isIvar("@b")},
		},
		{
			src:       "a, *@b, c = list",
			wantNames: []string{"a", "", "c"},
			wantSplat: 1,
			check:     []func(ast.Node) bool{isVar("a"), isIvar("@b"), isVar("c")},
		},
	}
	for _, c := range cases {
		ma := masgnOf(t, c.src)
		if ma.SplatIndex != c.wantSplat {
			t.Errorf("%q: SplatIndex=%d, want %d", c.src, ma.SplatIndex, c.wantSplat)
		}
		if len(ma.Names) != len(c.wantNames) {
			t.Fatalf("%q: %d names, want %d", c.src, len(ma.Names), len(c.wantNames))
		}
		for i, want := range c.wantNames {
			if ma.Names[i] != want {
				t.Errorf("%q: Names[%d]=%q, want %q", c.src, i, ma.Names[i], want)
			}
		}
		if ma.Targets == nil {
			t.Fatalf("%q: Targets is nil (non-local masgn must populate Targets)", c.src)
		}
		for i, pred := range c.check {
			if !pred(ma.Targets[i]) {
				t.Errorf("%q: Targets[%d]=%#v failed its predicate", c.src, i, ma.Targets[i])
			}
		}
	}
}

// TestMasgnAttrTargets covers attribute and index masgn targets, which lower to
// the existing setter-call shapes (`x=` / `[]=`) so a consumer reuses its
// single-assignment store logic.
func TestMasgnAttrTargets(t *testing.T) {
	ma := masgnOf(t, "obj.x, obj.y = 1, 2")
	for i, name := range []string{"x=", "y="} {
		call, ok := ma.Targets[i].(*ast.Call)
		if !ok || call.Name != name || call.Recv == nil {
			t.Errorf("obj.x masgn: Targets[%d]=%#v, want Call name %q with recv", i, ma.Targets[i], name)
		}
	}

	ma = masgnOf(t, "arr[i], h[k] = 1, 2")
	for i := range []int{0, 1} {
		call, ok := ma.Targets[i].(*ast.Call)
		if !ok || call.Name != "[]=" || call.Recv == nil {
			t.Errorf("arr[i] masgn: Targets[%d]=%#v, want Call name []= with recv", i, ma.Targets[i])
		}
		// The index argument is retained; the value is appended by the store.
		if len(call.Args) != 1 {
			t.Errorf("arr[i] masgn: Targets[%d] has %d args, want 1 (index only)", i, len(call.Args))
		}
	}
}

// TestMasgnChainedTargets covers deeper attribute/index chains as targets.
func TestMasgnChainedTargets(t *testing.T) {
	ma := masgnOf(t, "a.b.c, d = 1, 2")
	call, ok := ma.Targets[0].(*ast.Call)
	if !ok || call.Name != "c=" {
		t.Errorf("a.b.c masgn: Targets[0]=%#v, want Call c=", ma.Targets[0])
	}

	ma = masgnOf(t, "h[k1][k2], x = 1, 2")
	call, ok = ma.Targets[0].(*ast.Call)
	if !ok || call.Name != "[]=" {
		t.Errorf("h[k1][k2] masgn: Targets[0]=%#v, want Call []=", ma.Targets[0])
	}
}

// TestMasgnNamelessSplatAndTrailingComma covers `a, * = list`, `* = list`, and
// the trailing-comma form `a, = x` — all MRI-valid.
func TestMasgnNamelessSplatAndTrailingComma(t *testing.T) {
	ma := masgnOf(t, "a, * = list")
	if ma.SplatIndex != 1 {
		t.Errorf("a, * = list: SplatIndex=%d, want 1", ma.SplatIndex)
	}
	if ma.Names[1] != "" {
		t.Errorf("a, * = list: Names[1]=%q, want empty (nameless splat)", ma.Names[1])
	}

	ma = masgnOf(t, "* = list")
	if ma.SplatIndex != 0 || len(ma.Names) != 1 || ma.Names[0] != "" {
		t.Errorf("* = list: got SplatIndex=%d Names=%v", ma.SplatIndex, ma.Names)
	}

	// `*, a = x` — a leading nameless splat followed by another target.
	ma = masgnOf(t, "*, a = x")
	if ma.SplatIndex != 0 || len(ma.Names) != 2 || ma.Names[0] != "" || ma.Names[1] != "a" {
		t.Errorf("*, a = x: SplatIndex=%d Names=%v", ma.SplatIndex, ma.Names)
	}

	ma = masgnOf(t, "a, = x")
	if len(ma.Names) != 1 || ma.Names[0] != "a" {
		t.Errorf("a, = x: Names=%v, want [a]", ma.Names)
	}
}

// TestMasgnAllLocalsFastPath: an all-local masgn keeps Targets nil so consumers
// that only understand local targets still work (unchanged behavior).
func TestMasgnAllLocalsFastPath(t *testing.T) {
	ma := masgnOf(t, "a, b = 1, 2")
	if ma.Targets != nil {
		t.Errorf("a, b = 1, 2: Targets should stay nil for the all-locals case, got %#v", ma.Targets)
	}
	if len(ma.Names) != 2 || ma.Names[0] != "a" || ma.Names[1] != "b" {
		t.Errorf("a, b = 1, 2: Names=%v", ma.Names)
	}
}

// TestSingleAssignUnaffected: single (non-comma) assignments to the same target
// kinds must NOT be parsed as masgn.
func TestSingleAssignUnaffected(t *testing.T) {
	cases := []struct {
		src  string
		want any
	}{
		{"@a = 1", &ast.IvarAssign{}},
		{"@@a = 1", &ast.CVarAssign{}},
		{"$a = 1", &ast.GVarAssign{}},
		{"A = 1", &ast.ConstAssign{}},
		{"a = 1", &ast.Assign{}},
	}
	for _, c := range cases {
		n := mustParseSingle(t, c.src)
		switch c.want.(type) {
		case *ast.IvarAssign:
			if _, ok := n.(*ast.IvarAssign); !ok {
				t.Errorf("%q: got %T, want *ast.IvarAssign", c.src, n)
			}
		case *ast.CVarAssign:
			if _, ok := n.(*ast.CVarAssign); !ok {
				t.Errorf("%q: got %T, want *ast.CVarAssign", c.src, n)
			}
		case *ast.GVarAssign:
			if _, ok := n.(*ast.GVarAssign); !ok {
				t.Errorf("%q: got %T, want *ast.GVarAssign", c.src, n)
			}
		case *ast.ConstAssign:
			if _, ok := n.(*ast.ConstAssign); !ok {
				t.Errorf("%q: got %T, want *ast.ConstAssign", c.src, n)
			}
		case *ast.Assign:
			if _, ok := n.(*ast.Assign); !ok {
				t.Errorf("%q: got %T, want *ast.Assign", c.src, n)
			}
		}
	}

	// `obj.x = 1` and `arr[i] = 1` stay single setter calls, not masgn.
	if _, ok := mustParseSingle(t, "obj.x = 1").(*ast.Call); !ok {
		t.Errorf("obj.x = 1 should parse as a setter Call, not masgn")
	}
	if _, ok := mustParseSingle(t, "arr[i] = 1").(*ast.Call); !ok {
		t.Errorf("arr[i] = 1 should parse as a []= Call, not masgn")
	}
}

func isVar(name string) func(ast.Node) bool {
	return func(n ast.Node) bool { v, ok := n.(*ast.VarRef); return ok && v.Name == name }
}
func isIvar(name string) func(ast.Node) bool {
	return func(n ast.Node) bool { v, ok := n.(*ast.IvarRef); return ok && v.Name == name }
}
func isCvar(name string) func(ast.Node) bool {
	return func(n ast.Node) bool { v, ok := n.(*ast.CVarRef); return ok && v.Name == name }
}
func isGvar(name string) func(ast.Node) bool {
	return func(n ast.Node) bool { v, ok := n.(*ast.GVarRef); return ok && v.Name == name }
}
func isConst(name string) func(ast.Node) bool {
	return func(n ast.Node) bool { v, ok := n.(*ast.ConstRef); return ok && v.Name == name }
}

// TestMasgnScopedConstTarget covers a `::`-scoped constant target.
func TestMasgnScopedConstTarget(t *testing.T) {
	ma := masgnOf(t, "A::B, c = 1, 2")
	if _, ok := ma.Targets[0].(*ast.ScopedConst); !ok {
		t.Errorf("A::B masgn: Targets[0]=%#v, want *ast.ScopedConst", ma.Targets[0])
	}
}

// TestMasgnParseErrors exercises the error branches of parseMlhsTarget: a
// method-call value with no receiver (`foo()`) and a bare `self`, neither of
// which is an assignable target (both rejected by MRI too).
func TestMasgnParseErrors(t *testing.T) {
	if _, err := parser.Parse("Foo(), a = 1, 2"); err == nil {
		t.Errorf("`Foo(), a = 1, 2` should fail: a receiver-less call is not assignable")
	}
	if _, err := parser.Parse("self, a = 1, 2"); err == nil {
		t.Errorf("`self, a = 1, 2` should fail: self is not an assignable target")
	}
}

// TestMasgnSelfAttrTarget: `self.x` is a valid attribute target.
func TestMasgnSelfAttrTarget(t *testing.T) {
	ma := masgnOf(t, "self.x, a = 1, 2")
	call, ok := ma.Targets[0].(*ast.Call)
	if !ok || call.Name != "x=" {
		t.Errorf("self.x masgn: Targets[0]=%#v, want Call x=", ma.Targets[0])
	}
	if _, ok := call.Recv.(*ast.SelfLit); !ok {
		t.Errorf("self.x masgn: recv=%T, want *ast.SelfLit", call.Recv)
	}
}
