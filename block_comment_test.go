package parser

import (
	"testing"

	"github.com/go-ruby-parser/parser/ast"
)

// `=begin` … `=end` block comments (at column 0) are skipped wholesale, with the
// surrounding code parsing normally.
func TestBlockComment(t *testing.T) {
	cases := []string{
		"=begin\nthis is a comment\nmore\n=end\nputs 1",
		"x = 1\n=begin\ncomment\n=end\ny = 2",
		"=begin doc\nfoo\n=end",
		"=begin\nunterminated comment to EOF",
	}
	for _, src := range cases {
		if _, err := Parse(src); err != nil {
			t.Fatalf("Parse(%q) error: %v", src, err)
		}
	}
}

// A `=begin` not at column 0 is not a block comment: `x = begin … end` is an
// ordinary begin expression assigned to x.
func TestBlockCommentNotAtLineStart(t *testing.T) {
	prog, err := Parse("x = begin\n  1\nend")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	asn, ok := prog.Body[0].(*ast.Assign)
	if !ok {
		t.Fatalf("expected *ast.Assign, got %T", prog.Body[0])
	}
	if _, ok := asn.Value.(*ast.Begin); !ok {
		t.Fatalf("expected RHS *ast.Begin, got %T", asn.Value)
	}
}

// An identifier that merely starts with "begin" is unaffected.
func TestBlockCommentIdentifierPrefix(t *testing.T) {
	if _, err := Parse("result = beginning"); err != nil {
		t.Fatalf("Parse error: %v", err)
	}
}

// The bitwise op-assignments |=, &=, ^=, >>= now lex (they were previously
// missing, so `mode |= x` failed). Confirm each produces an OpAssign.
func TestBitwiseOpAssign(t *testing.T) {
	for _, c := range []struct {
		src string
		op  string
	}{
		{"mode |= x", "|"},
		{"a &= b", "&"},
		{"c ^= d", "^"},
		{"x >>= 2", ">>"},
		{"y <<= 1", "<<"},
	} {
		prog, err := Parse(c.src)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", c.src, err)
		}
		oa, ok := prog.Body[0].(*ast.OpAssign)
		if !ok {
			t.Fatalf("%q: expected *ast.OpAssign, got %T", c.src, prog.Body[0])
		}
		if oa.Op != c.op {
			t.Fatalf("%q: op=%q, want %q", c.src, oa.Op, c.op)
		}
	}
}
