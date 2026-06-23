package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// blockOf returns the literal block of the top-level call in src (a brace/do
// block, or a stabby lambda lowered to a `lambda` call carrying the Block).
func blockOf(t *testing.T, src string) *ast.Block {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	blk := prog.Body[0].(*ast.Call).Block
	if blk == nil {
		t.Fatalf("Parse(%q): no block on call", src)
	}
	return blk
}

// TestBlockOptionalParams covers optional `name = default` parameters in brace,
// do, and stabby-lambda parameter lists. Defaults parallels Params: nil for a
// required or *splat param, non-nil for an optional one — mirroring MethodDef.
func TestBlockOptionalParams(t *testing.T) {
	cases := []struct {
		src        string
		wantParams int
		wantSplat  int
		// optAt indexes Params whose Defaults entry must be non-nil.
		optAt []int
	}{
		{"proc { |a, b = 5| a }", 2, -1, []int{1}},
		{"lambda { |a, b = 5| a }", 2, -1, []int{1}},
		{"[1].each do |a, b = 5| a end", 2, -1, []int{1}},
		{"->(a, b = 5) { a }", 2, -1, []int{1}},
		{"->(a, b = 5) do a end", 2, -1, []int{1}},
		{"proc { |a = 1, b = 2| a }", 2, -1, []int{0, 1}},
		{"->(*a) { a }", 1, 0, nil},
		{"->(a, *b) { a }", 2, 1, nil},
		{"->(a, *b, c) { a }", 3, 1, nil},
		{"->(a, b = 5, *c) { a }", 3, 2, []int{1}},
		{"->(a) { a }", 1, -1, nil}, // plain required only
		{"proc { |a| a }", 1, -1, nil},
	}
	for _, c := range cases {
		blk := blockOf(t, c.src)
		if len(blk.Params) != c.wantParams {
			t.Errorf("%q: params=%d, want %d", c.src, len(blk.Params), c.wantParams)
			continue
		}
		if blk.SplatIndex != c.wantSplat {
			t.Errorf("%q: splat=%d, want %d", c.src, blk.SplatIndex, c.wantSplat)
		}
		if len(blk.Defaults) != c.wantParams {
			t.Errorf("%q: len(Defaults)=%d, want %d", c.src, len(blk.Defaults), c.wantParams)
			continue
		}
		opt := map[int]bool{}
		for _, i := range c.optAt {
			opt[i] = true
		}
		for i := range blk.Params {
			if opt[i] && blk.Defaults[i] == nil {
				t.Errorf("%q: Defaults[%d] is nil, want a default expr", c.src, i)
			}
			if !opt[i] && blk.Defaults[i] != nil {
				t.Errorf("%q: Defaults[%d] is non-nil, want nil", c.src, i)
			}
		}
	}
}

// TestBlockOptionalDefaultValue verifies the parsed default expression is the
// one written (e.g. `b = 5` yields the IntLit 5).
func TestBlockOptionalDefaultValue(t *testing.T) {
	for _, src := range []string{"proc { |a, b = 5| a }", "->(a, b = 5) { a }"} {
		blk := blockOf(t, src)
		lit, ok := blk.Defaults[1].(*ast.IntLit)
		if !ok || lit.Value != 5 {
			t.Errorf("%q: Defaults[1]=%#v, want IntLit 5", src, blk.Defaults[1])
		}
	}
}

// TestStabbyLambdaSplatErrors: two rest parameters in a stabby-lambda list are
// rejected, like the brace-block and method-param paths.
func TestStabbyLambdaSplatErrors(t *testing.T) {
	for _, src := range []string{
		"->(*a, *b) { a }",
		"->(a, *b, *c) { a }",
	} {
		if _, err := parser.Parse(src); err == nil {
			t.Errorf("expected a parse error for %q, got none", src)
		}
	}
}

// TestStabbyLambdaGroupParam: destructuring group params still work in the
// stabby-lambda `(...)` list now that it shares the block-param grammar.
func TestStabbyLambdaGroupParam(t *testing.T) {
	blk := blockOf(t, "->((a, b), c) { a }")
	if len(blk.Params) != 2 {
		t.Fatalf("params=%d, want 2", len(blk.Params))
	}
	if _, ok := blk.Body[0].(*ast.MultiAssign); !ok {
		t.Errorf("expected a leading MultiAssign prepend, got %T", blk.Body[0])
	}
}
