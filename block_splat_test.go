package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// TestBlockSplatParams covers top-level rest parameters in block signatures
// (|*rest|, |a, *rest|, |*rest, b|) — the SplatIndex the parser records on the
// Block, which the compiler lowers like a method's *splat.
func TestBlockSplatParams(t *testing.T) {
	cases := []struct {
		src        string
		wantParams int
		wantSplat  int
	}{
		{"[1].each { |*a| a }", 1, 0},
		{"[1].each { |a, *b| a }", 2, 1},
		{"[1].each { |*a, b| b }", 2, 0},
		{"[1].each { |a, *b, c| b }", 3, 1},
		{"[1].each { |a| a }", 1, -1}, // no splat
	}
	for _, c := range cases {
		prog, err := parser.Parse(c.src)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.src, err)
		}
		blk := prog.Body[0].(*ast.Call).Block
		if len(blk.Params) != c.wantParams || blk.SplatIndex != c.wantSplat {
			t.Errorf("%q: params=%d splat=%d, want params=%d splat=%d",
				c.src, len(blk.Params), blk.SplatIndex, c.wantParams, c.wantSplat)
		}
	}
}

// TestBlockSplatErrors: two rest parameters in one block signature is rejected.
func TestBlockSplatErrors(t *testing.T) {
	for _, src := range []string{
		"[1].each { |*a, *b| a }",
		"[1].each { |a, *b, *c| a }",
	} {
		if _, err := parser.Parse(src); err == nil {
			t.Errorf("expected a parse error for %q, got none", src)
		}
	}
}
