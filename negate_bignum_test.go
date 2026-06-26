package parser_test

import (
	"math/big"
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// TestNegativeBignumLiteral covers a negated integer literal that overflows
// int64: the magnitude lexes as a BignumLit, so negation must produce a negated
// BignumLit rather than panicking on a bad type assertion (the cause of the
// real-world TypeAssertionError crashes on Rails sources).
func TestNegativeBignumLiteral(t *testing.T) {
	cases := []struct {
		src  string
		want string // decimal string of the expected big value
	}{
		{"-9999999999999999999999999999999", "-9999999999999999999999999999999"},
		{"-9223372036854775808", "-9223372036854775808"}, // int64 min: magnitude overflows
		{"-10000000000000000000000", "-10000000000000000000000"},
	}
	for _, c := range cases {
		prog, err := parser.Parse(c.src)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.src, err)
		}
		bn, ok := prog.Body[0].(*ast.BignumLit)
		if !ok {
			t.Fatalf("Parse(%q): top is %T, want *ast.BignumLit", c.src, prog.Body[0])
		}
		want, _ := new(big.Int).SetString(c.want, 10)
		if bn.Val.Cmp(want) != 0 {
			t.Errorf("Parse(%q): value = %s, want %s", c.src, bn.Val, want)
		}
	}
}

// TestNegativeBignumPostfixAndAssign covers the negated-bignum node flowing
// through a postfix chain and an assignment RHS (the surrounding paths the
// negate helper feeds).
func TestNegativeBignumPostfixAndAssign(t *testing.T) {
	// (-big).abs — the negated bignum carries its own postfix chain.
	call, ok := mustParseSingle(t, "(-9999999999999999999999).abs").(*ast.Call)
	if !ok || call.Name != "abs" {
		t.Fatalf("(-big).abs: top is %T, want call abs", call)
	}
	// x = -big — negated bignum as an assignment value.
	asn, ok := mustParseSingle(t, "x = -10000000000000000000000").(*ast.Assign)
	if !ok {
		t.Fatalf("x = -big: top is %T, want *ast.Assign", asn)
	}
	if _, ok := asn.Value.(*ast.BignumLit); !ok {
		t.Errorf("x = -big: RHS is %T, want *ast.BignumLit", asn.Value)
	}
}

// mustParseSingle parses src and returns its single top-level node.
func mustParseSingle(t *testing.T, src string) ast.Node {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	if len(prog.Body) != 1 {
		t.Fatalf("Parse(%q): want 1 node, got %d", src, len(prog.Body))
	}
	return prog.Body[0]
}
