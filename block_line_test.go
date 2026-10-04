package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// collectBlocks returns every Block in the tree, in source order, for the node
// shapes these cases use.
func collectBlocks(n ast.Node, out *[]*ast.Block) {
	switch v := n.(type) {
	case *ast.Program:
		for _, s := range v.Body {
			collectBlocks(s, out)
		}
	case *ast.Call:
		if v.Recv != nil {
			collectBlocks(v.Recv, out)
		}
		for _, a := range v.Args {
			collectBlocks(a, out)
		}
		if v.Block != nil {
			*out = append(*out, v.Block)
			for _, s := range v.Block.Body {
				collectBlocks(s, out)
			}
		}
	case *ast.Assign:
		collectBlocks(v.Value, out)
	case *ast.HashLit:
		for _, k := range v.Keys {
			collectBlocks(k, out)
		}
		for _, val := range v.Values {
			collectBlocks(val, out)
		}
	case *ast.ArrayLit:
		for _, e := range v.Elems {
			collectBlocks(e, out)
		}
	}
}

// TestBlockLineIsItsOpener: a Block carries the line its `{`, `do` or `->` sits
// on, which is MRI's location.first_lineno for the block.
//
// A block is not a statement, so Program.Lines never held this, and a consumer
// had to take the enclosing statement's line -- the same answer only when the
// block opens on that statement's first line. Every want below is the line
// ruby 4.0.5 reports through Proc#source_location for the corresponding block.
func TestBlockLineIsItsOpener(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want []int
	}{
		{
			// The discriminating case: the body's first line would be 3, 6 and 7,
			// and ruby answers 2, 5 and 6 -- the openers. The third block is empty,
			// so it has no body line at all.
			name: "multi-line hash",
			src: "h = {\n" + //            1
				"  :a => proc {\n" + //    2
				"    1\n" + //             3
				"  },\n" + //              4
				"  :b => proc { 2 },\n" + // 5
				"  :c => proc {\n" + //    6
				"  },\n" + //              7
				"}\n",
			want: []int{2, 5, 6},
		},
		{
			name: "multi-line array with do...end",
			src: "x = [\n" + //      1
				"  proc do\n" + //   2
				"    3\n" + //       3
				"  end,\n" + //      4
				"]\n",
			want: []int{2},
		},
		{
			// The arrow's own line, not its `{`.
			name: "arrow lambda",
			src: "h = {\n" + //       1
				"  :a =>\n" + //      2
				"    -> {\n" + //     3
				"      1\n" + //      4
				"    },\n" + //       5
				"  :b => ->() { 2 },\n" + // 6
				"}\n",
			want: []int{3, 6},
		},
		{
			name: "single line, unchanged",
			src:  "h = { :a => proc { 1 } }\n",
			want: []int{1},
		},
		{
			name: "method call with a trailing block",
			src: "foo(1,\n" + //   1
				"    2) do\n" + // 2
				"  3\n" + //       3
				"end\n",
			want: []int{2},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prog, err := parser.Parse(tc.src)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			var blocks []*ast.Block
			collectBlocks(prog, &blocks)
			if len(blocks) != len(tc.want) {
				t.Fatalf("found %d blocks, want %d", len(blocks), len(tc.want))
			}
			for i, b := range blocks {
				if b.Line != tc.want[i] {
					t.Errorf("block %d: Line = %d, want %d (ruby 4.0.5)", i, b.Line, tc.want[i])
				}
			}
		})
	}
}
