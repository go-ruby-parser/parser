package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// binExpr asserts n is a *ast.BinaryExpr with the given operator.
func binExpr(t *testing.T, n ast.Node, op string) *ast.BinaryExpr {
	t.Helper()
	be, ok := n.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("node = %T, want *ast.BinaryExpr", n)
	}
	if be.Op != op {
		t.Fatalf("BinaryExpr.Op = %q, want %q", be.Op, op)
	}
	return be
}

// unaryExpr asserts n is a *ast.UnaryExpr with the given operator.
func unaryExpr(t *testing.T, n ast.Node, op string) *ast.UnaryExpr {
	t.Helper()
	ue, ok := n.(*ast.UnaryExpr)
	if !ok {
		t.Fatalf("node = %T, want *ast.UnaryExpr", n)
	}
	if ue.Op != op {
		t.Fatalf("UnaryExpr.Op = %q, want %q", ue.Op, op)
	}
	return ue
}

func TestKeywordAndDesugarsToAndAnd(t *testing.T) {
	// `x = true and false` is `(x = true) && false` (and binds below =).
	be := binExpr(t, parseOne(t, "x = true and false"), "&&")
	if _, ok := be.Left.(*ast.Assign); !ok {
		t.Errorf("Left = %T, want *ast.Assign (= binds tighter than and)", be.Left)
	}
}

func TestKeywordOrDesugarsToOrOr(t *testing.T) {
	binExpr(t, parseOne(t, "a or b"), "||")
}

func TestKeywordAndOrLeftAssociative(t *testing.T) {
	// `a and b or c` is `(a and b) or c`.
	top := binExpr(t, parseOne(t, "a and b or c"), "||")
	binExpr(t, top.Left, "&&")
}

func TestKeywordNotPrefixStatement(t *testing.T) {
	unaryExpr(t, parseOne(t, "not x"), "!")
}

func TestKeywordNotInAssignRHS(t *testing.T) {
	a, ok := parseOne(t, "x = not true").(*ast.Assign)
	if !ok {
		t.Fatalf("node = %T, want *ast.Assign", parseOne(t, "x = not true"))
	}
	unaryExpr(t, a.Value, "!")
}

func TestKeywordNotDoubled(t *testing.T) {
	outer := unaryExpr(t, parseOne(t, "not not x"), "!")
	unaryExpr(t, outer.Operand, "!")
}

func TestKeywordNotLowPrecedence(t *testing.T) {
	// `not a == b` is `not (a == b)`.
	ue := unaryExpr(t, parseOne(t, "not a == b"), "!")
	binExpr(t, ue.Operand, "==")
}

func TestKeywordNotInCallArg(t *testing.T) {
	call := parseOne(t, "foo(not flag)").(*ast.Call)
	unaryExpr(t, call.Args[0], "!")
}

func TestKeywordOrReturnOperand(t *testing.T) {
	// `a or return` — the jump keyword is a valid or-operand.
	be := binExpr(t, parseOne(t, "a or return"), "||")
	if _, ok := be.Right.(*ast.Return); !ok {
		t.Errorf("Right = %T, want *ast.Return", be.Right)
	}
}

func TestKeywordAndBreakOperand(t *testing.T) {
	be := binExpr(t, parseOne(t, "a and break"), "&&")
	if _, ok := be.Right.(*ast.Break); !ok {
		t.Errorf("Right = %T, want *ast.Break", be.Right)
	}
}

func TestKeywordOrNextOperand(t *testing.T) {
	be := binExpr(t, parseOne(t, "a or next"), "||")
	if _, ok := be.Right.(*ast.Next); !ok {
		t.Errorf("Right = %T, want *ast.Next", be.Right)
	}
}

func TestKeywordAndReturnWithValue(t *testing.T) {
	// `x = 1 and return v` — return takes a value as the and-operand.
	be := binExpr(t, parseOne(t, "x = 1 and return 2"), "&&")
	ret := be.Right.(*ast.Return)
	if ret.Value == nil {
		t.Errorf("return value = nil, want a value")
	}
}

func TestKeywordLogicalInCondition(t *testing.T) {
	ifn := parseOne(t, "if a and b then 1 end").(*ast.If)
	binExpr(t, ifn.Cond, "&&")
}

func TestKeywordLogicalInUnless(t *testing.T) {
	// `unless a or b` desugars cond to `!(a || b)`.
	ifn := parseOne(t, "unless a or b then 1 end").(*ast.If)
	ue := unaryExpr(t, ifn.Cond, "!")
	binExpr(t, ue.Operand, "||")
}

func TestKeywordLogicalInElsif(t *testing.T) {
	ifn := parseOne(t, "if a\n1\nelsif b and c\n2\nend").(*ast.If)
	binExpr(t, ifn.Elsifs[0].Cond, "&&")
}

func TestKeywordLogicalInWhile(t *testing.T) {
	w := parseOne(t, "while a or b; c; end").(*ast.While)
	binExpr(t, w.Cond, "||")
}

func TestKeywordLogicalInUntil(t *testing.T) {
	w := parseOne(t, "until a and b; c; end").(*ast.While)
	unaryExpr(t, w.Cond, "!") // until desugars to while !cond
}

func TestKeywordLogicalInParens(t *testing.T) {
	binExpr(t, parseOne(t, "(a and b)"), "&&")
}

func TestKeywordNotInParens(t *testing.T) {
	unaryExpr(t, parseOne(t, "(not x)"), "!")
}

func TestKeywordLogicalModifierIf(t *testing.T) {
	// `x if a and b` — the modifier condition admits and/or.
	ifn := parseOne(t, "x if a and b").(*ast.If)
	binExpr(t, ifn.Cond, "&&")
}

func TestKeywordLogicalModifierWhile(t *testing.T) {
	w := parseOne(t, "x while a or b").(*ast.While)
	binExpr(t, w.Cond, "||")
}

func TestKeywordLogicalModifierUntil(t *testing.T) {
	w := parseOne(t, "x until a and b").(*ast.While)
	unaryExpr(t, w.Cond, "!")
}

func TestKeywordLogicalValidPrograms(t *testing.T) {
	for _, src := range []string{
		"x = true and false",
		"a or return",
		"not x",
		"x = not not x",
		"a and b or c",
		"if x and y then 1 end",
		"while a or b; c; end",
	} {
		if _, err := parser.Parse(src); err != nil {
			t.Errorf("Parse(%q) returned error: %v", src, err)
		}
	}
}
