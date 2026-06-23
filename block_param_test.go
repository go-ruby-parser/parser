package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// TestBlockBlockParam covers the `&block` parameter in brace, do, and stabby
// lambda parameter lists. It is recorded on ast.Block.BlockParam, parallel to
// MethodDef.BlockParam — and is always the last parameter.
func TestBlockBlockParam(t *testing.T) {
	cases := []struct {
		src        string
		wantParams int
		wantSplat  int
		wantBlock  string
	}{
		{"foo { |*a, &b| b }", 1, 0, "b"},
		{"foo do |x, &b| b end", 1, -1, "b"},
		{"->(x, &b){ b }", 1, -1, "b"},
		{"foo { |&b| b }", 0, -1, "b"},
		{"foo { |x, y, &blk| blk }", 2, -1, "blk"},
		{"foo { |x| x }", 1, -1, ""}, // no &block: BlockParam empty
		{"->(x){ x }", 1, -1, ""},
	}
	for _, c := range cases {
		blk := blockOf(t, c.src)
		if len(blk.Params) != c.wantParams {
			t.Errorf("%q: params=%d, want %d", c.src, len(blk.Params), c.wantParams)
		}
		if blk.SplatIndex != c.wantSplat {
			t.Errorf("%q: splat=%d, want %d", c.src, blk.SplatIndex, c.wantSplat)
		}
		if blk.BlockParam != c.wantBlock {
			t.Errorf("%q: BlockParam=%q, want %q", c.src, blk.BlockParam, c.wantBlock)
		}
	}
}

// TestBlockBlockParamDeclaredLocal: the &block name is in scope inside the body
// (it resolves as a local, not a bare method call).
func TestBlockBlockParamDeclaredLocal(t *testing.T) {
	blk := blockOf(t, "foo { |&b| b }")
	if _, ok := blk.Body[0].(*ast.VarRef); !ok {
		t.Errorf("body[0]=%T, want *ast.VarRef (b is a declared local)", blk.Body[0])
	}
}

// TestDefBlockParamUnchanged: `def f(x, &b)` still records the method block param
// on MethodDef, untouched by the block-literal change.
func TestDefBlockParamUnchanged(t *testing.T) {
	prog, err := parser.Parse("def f(x, &b)\n  b\nend")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	def := prog.Body[0].(*ast.MethodDef)
	if def.BlockParam != "b" {
		t.Errorf("MethodDef.BlockParam=%q, want %q", def.BlockParam, "b")
	}
}
