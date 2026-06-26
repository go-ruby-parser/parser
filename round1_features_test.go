package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// --- Feature #2: rescue/ensure without an explicit begin on do…end blocks and
// class/module/singleton-class bodies (def bodies already supported). ---

// beginOf returns the *ast.Begin that a begin-less rescue/ensure body lowers to,
// found in the single-element body of the given node.
func bodyHasBegin(body []ast.Node) (*ast.Begin, bool) {
	if len(body) == 1 {
		b, ok := body[0].(*ast.Begin)
		return b, ok
	}
	return nil, false
}

func TestRescueOnDoBlock(t *testing.T) {
	blk := blockOf(t, "[1].each do |x|; foo; rescue => e; bar; end")
	b, ok := bodyHasBegin(blk.Body)
	if !ok {
		t.Fatalf("do-block body did not lower to a Begin: %#v", blk.Body)
	}
	if len(b.Rescues) != 1 {
		t.Fatalf("want 1 rescue clause, got %d", len(b.Rescues))
	}
	if b.Rescues[0].Var != "e" {
		t.Errorf("rescue var=%q, want e", b.Rescues[0].Var)
	}
}

func TestRescueElseEnsureOnDoBlock(t *testing.T) {
	blk := blockOf(t, "[1].each do |x|; foo; rescue E => e; r; else; el; ensure; en; end")
	b, ok := bodyHasBegin(blk.Body)
	if !ok {
		t.Fatalf("do-block body did not lower to a Begin: %#v", blk.Body)
	}
	if len(b.Rescues) != 1 || b.ElseBody == nil || b.EnsureBody == nil {
		t.Errorf("want rescue+else+ensure, got rescues=%d else=%v ensure=%v",
			len(b.Rescues), b.ElseBody != nil, b.EnsureBody != nil)
	}
}

func TestRescueOnClassBody(t *testing.T) {
	cd := mustParseSingle(t, "class C; foo; rescue => e; bar; end").(*ast.ClassDef)
	if _, ok := bodyHasBegin(cd.Body); !ok {
		t.Errorf("class body did not lower to a Begin: %#v", cd.Body)
	}
}

func TestEnsureOnModuleBody(t *testing.T) {
	md := mustParseSingle(t, "module M; foo; ensure; cleanup; end").(*ast.ModuleDef)
	b, ok := bodyHasBegin(md.Body)
	if !ok || b.EnsureBody == nil {
		t.Errorf("module body did not lower to a Begin with ensure: %#v", md.Body)
	}
}

func TestRescueOnSingletonClassBody(t *testing.T) {
	sc := mustParseSingle(t, "class << self; foo; rescue => e; bar; end").(*ast.SingletonClassDef)
	if _, ok := bodyHasBegin(sc.Body); !ok {
		t.Errorf("singleton-class body did not lower to a Begin: %#v", sc.Body)
	}
}

// A class/module body with no rescue/ensure must stay a plain body (not wrapped).
func TestClassBodyWithoutRescueUnchanged(t *testing.T) {
	cd := mustParseSingle(t, "class C; def m; end; end").(*ast.ClassDef)
	if _, ok := bodyHasBegin(cd.Body); ok {
		t.Errorf("class body with no rescue should not be wrapped in a Begin")
	}
	// A brace block must NOT accept a begin-less rescue (MRI rejects it).
	if _, err := parser.Parse("[1].each { |x| foo; rescue => e; bar }"); err == nil {
		t.Errorf("brace block with begin-less rescue should fail to parse")
	}
}

// --- Feature #3: anonymous + ordered params. ---

func defOf(t *testing.T, src string) *ast.MethodDef {
	t.Helper()
	n := mustParseSingle(t, src)
	d, ok := n.(*ast.MethodDef)
	if !ok {
		t.Fatalf("Parse(%q): top node = %T, want *ast.MethodDef", src, n)
	}
	return d
}

func TestAnonymousDefParams(t *testing.T) {
	// Anonymous splat: sentinel param name "*".
	d := defOf(t, "def f(*); end")
	if d.SplatIndex != 0 || len(d.Params) != 1 || d.Params[0] != "*" {
		t.Errorf("def f(*): SplatIndex=%d Params=%v", d.SplatIndex, d.Params)
	}
	// Anonymous double-splat: sentinel kwRest "**".
	d = defOf(t, "def g(**); end")
	if d.KwRest != "**" {
		t.Errorf("def g(**): KwRest=%q, want **", d.KwRest)
	}
	// Anonymous block: sentinel blockParam "&".
	d = defOf(t, "def h(&); end")
	if d.BlockParam != "&" {
		t.Errorf("def h(&): BlockParam=%q, want &", d.BlockParam)
	}
	// `def f(a, *)` — leading positional + anonymous splat.
	d = defOf(t, "def f(a, *); end")
	if d.SplatIndex != 1 || d.Params[1] != "*" {
		t.Errorf("def f(a, *): SplatIndex=%d Params=%v", d.SplatIndex, d.Params)
	}
	// `def f(**nil)` — explicitly no keyword args.
	d = defOf(t, "def f(**nil); end")
	if d.KwRest != "nil" {
		t.Errorf("def f(**nil): KwRest=%q, want nil", d.KwRest)
	}
}

func TestKwargsThenBlockParam(t *testing.T) {
	// **double-splat followed by &block — the ordering gap.
	d := defOf(t, "def f(**options, &block); end")
	if d.KwRest != "options" || d.BlockParam != "block" {
		t.Errorf("def f(**options, &block): KwRest=%q BlockParam=%q", d.KwRest, d.BlockParam)
	}
	d = defOf(t, "def f(*args, **opts, &block); end")
	if d.SplatIndex != 0 || d.KwRest != "opts" || d.BlockParam != "block" {
		t.Errorf("def f(*args, **opts, &block): splat=%d KwRest=%q BlockParam=%q",
			d.SplatIndex, d.KwRest, d.BlockParam)
	}
	d = defOf(t, "def f(status: :processing, **mail_options, &block); end")
	if len(d.KwParams) != 1 || d.KwRest != "mail_options" || d.BlockParam != "block" {
		t.Errorf("def f(status:, **mail_options, &block): kwparams=%d KwRest=%q BlockParam=%q",
			len(d.KwParams), d.KwRest, d.BlockParam)
	}
}

func TestAnonymousLambdaParams(t *testing.T) {
	// ->(*) {} ->(**) {} ->(&) {}
	if b := blockOf(t, "->(*) { }"); b.SplatIndex != 0 || b.Params[0] != "*" {
		t.Errorf("->(*): SplatIndex=%d Params=%v", b.SplatIndex, b.Params)
	}
	if b := blockOf(t, "->(&) { }"); b.BlockParam != "&" {
		t.Errorf("->(&): BlockParam=%q, want &", b.BlockParam)
	}
	// **rest in a lambda is recorded as a "**"-prefixed param name.
	b := blockOf(t, "->(**) { }")
	if len(b.Params) != 1 || b.Params[0] != "**" {
		t.Errorf("->(**): Params=%v, want [**]", b.Params)
	}
	b = blockOf(t, "->(**kw) { }")
	if len(b.Params) != 1 || b.Params[0] != "**kw" {
		t.Errorf("->(**kw): Params=%v, want [**kw]", b.Params)
	}
	// A lambda double-splat may be followed by a &block param.
	b = blockOf(t, "->(**kw, &b) { }")
	if len(b.Params) != 1 || b.Params[0] != "**kw" || b.BlockParam != "b" {
		t.Errorf("->(**kw, &b): Params=%v BlockParam=%q", b.Params, b.BlockParam)
	}
}

func TestNamedParamsStillDeclareLocals(t *testing.T) {
	// A named *splat / **rest / &block still declares its local in the body.
	d := defOf(t, "def f(*a, **b, &c); a; b; c; end")
	if d.Params[0] != "a" || d.KwRest != "b" || d.BlockParam != "c" {
		t.Errorf("named params: Params=%v KwRest=%q BlockParam=%q", d.Params, d.KwRest, d.BlockParam)
	}
}

// --- Feature #5: value-omitted shorthand kwargs in calls/super/array. ---

func TestKwShorthandInCall(t *testing.T) {
	// foo(format:, name:) == foo(format: format, name: name)
	call := mustParseSingle(t, "foo(format:, name:)").(*ast.Call)
	h, ok := lastHash(call.Args)
	if !ok || len(h.Keys) != 2 {
		t.Fatalf("foo(format:, name:): want trailing hash with 2 keys, got %#v", call.Args)
	}
	wantKeys := []string{"format", "name"}
	for i, k := range h.Keys {
		sym, ok := k.(*ast.SymbolLit)
		if !ok || sym.Name != wantKeys[i] {
			t.Errorf("key[%d]=%#v, want symbol %q", i, k, wantKeys[i])
		}
		// Value is the same-named bareword (a self method call here, no local).
		c, ok := h.Values[i].(*ast.Call)
		if !ok || c.Name != wantKeys[i] {
			t.Errorf("value[%d]=%#v, want bareword Call %q", i, h.Values[i], wantKeys[i])
		}
	}
}

func TestKwShorthandReferencesLocal(t *testing.T) {
	// When the name is a visible local, the shorthand value is a VarRef.
	prog, err := parser.Parse("format = 1\nfoo(format:)")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	call := prog.Body[1].(*ast.Call)
	h, _ := lastHash(call.Args)
	if _, ok := h.Values[0].(*ast.VarRef); !ok {
		t.Errorf("shorthand value=%T, want *ast.VarRef (format is a local)", h.Values[0])
	}
}

func TestKwShorthandMixedWithSplat(t *testing.T) {
	// super(columns, name:, unique:, **options)
	s := mustParseSingle(t, "super(columns, name:, unique:, **options)").(*ast.Super)
	h, ok := lastHash(s.Args)
	if !ok {
		t.Fatalf("super(...): no trailing hash: %#v", s.Args)
	}
	if len(h.Keys) != 3 { // name:, unique:, and the **options merge (nil key)
		t.Errorf("super hash keys=%d, want 3", len(h.Keys))
	}
	if h.Keys[2] != nil {
		t.Errorf("super: last key should be nil (**options merge), got %#v", h.Keys[2])
	}
}

func TestKwShorthandAndPairsInArray(t *testing.T) {
	// [callback, every: every] == [callback, {every: every}]
	arr := mustParseSingle(t, "[callback, every: every]").(*ast.ArrayLit)
	if len(arr.Elems) != 2 {
		t.Fatalf("array elems=%d, want 2 (positional + trailing hash)", len(arr.Elems))
	}
	if _, ok := arr.Elems[1].(*ast.HashLit); !ok {
		t.Errorf("array elem[1]=%T, want *ast.HashLit", arr.Elems[1])
	}
	// Nested form [[callback, every: every]].
	outer := mustParseSingle(t, "[[callback, every: every]]").(*ast.ArrayLit)
	inner, ok := outer.Elems[0].(*ast.ArrayLit)
	if !ok || len(inner.Elems) != 2 {
		t.Errorf("nested array: %#v", outer.Elems[0])
	}

	// Explicit-value pairs collapse too: [a, k: 1, k2: 2].
	arr = mustParseSingle(t, "[a, key: 1, k2: 2]").(*ast.ArrayLit)
	h := arr.Elems[1].(*ast.HashLit)
	if len(h.Keys) != 2 {
		t.Errorf("[a, key:1, k2:2]: trailing hash keys=%d, want 2", len(h.Keys))
	}
}

func TestArrayTrailingCommaAndEmpty(t *testing.T) {
	if a := mustParseSingle(t, "[]").(*ast.ArrayLit); len(a.Elems) != 0 {
		t.Errorf("[]: elems=%d, want 0", len(a.Elems))
	}
	if a := mustParseSingle(t, "[1, 2,]").(*ast.ArrayLit); len(a.Elems) != 2 {
		t.Errorf("[1, 2,]: elems=%d, want 2", len(a.Elems))
	}
}

func TestCallTrailingComma(t *testing.T) {
	c := mustParseSingle(t, "foo(1, 2,)").(*ast.Call)
	if len(c.Args) != 2 {
		t.Errorf("foo(1, 2,): args=%d, want 2", len(c.Args))
	}
}

// lastHash returns the trailing implicit-hash argument, if any.
func lastHash(args []ast.Node) (*ast.HashLit, bool) {
	if len(args) == 0 {
		return nil, false
	}
	h, ok := args[len(args)-1].(*ast.HashLit)
	return h, ok
}
