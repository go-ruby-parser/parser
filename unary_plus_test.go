package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// TestUnaryPlusDispatchesExceptOnANumericLiteral pins MRI's rule, which is
// narrower than either "always fold" or "always dispatch".
//
// `arg: tUPLUS arg` calls call_uni_op(p, $2, idUPlus) (parse.y-ruby_4_0:3962),
// so unary plus is a #+@ send. The one exception is LEXICAL: a plus directly
// before a numeric literal token is absorbed into the literal, exactly as the
// minus beside it is. Measured on ruby 4.0.5 with Integer#+@, Float#+@ and
// String#+@ all redefined to return a marker:
//
//	+1        -> 1            the literal, nothing dispatched
//	+1.5      -> 1.5          likewise
//	+"lit"    -> the marker
//	+n        -> the marker   a variable holding a number still dispatches
//	+(1)      -> the marker   PARENTHESISED, so no longer a bare literal token
//	+(1+1)    -> the marker
//
// The parser used to elide unary plus entirely. That was invisible until the
// frozen_string_literal pragma made `+"..."` the ordinary way to ask a frozen
// literal for a mutable copy: eliding handed back the frozen literal, so
// `buf = +"start"; buf << "-more"` raised FrozenError where ruby prints
// "start-more".
func TestUnaryPlusDispatchesExceptOnANumericLiteral(t *testing.T) {
	for _, tc := range []struct {
		src       string
		wantUnary bool // a #+@ send, rather than the bare operand
	}{
		{"+1", false},
		{"+1.5", false},
		{`+"lit"`, true},
		{"x = 1; +x", true},
		{"+(1)", true},
		{"+(1 + 1)", true},
		{"+foo", true},
		{"+@ivar", true},
		{"+[1]", true},
	} {
		t.Run(tc.src, func(t *testing.T) {
			n := lastNode(t, tc.src)
			u, isUnary := n.(*ast.UnaryExpr)
			if isUnary != tc.wantUnary {
				t.Fatalf("%s: got %T, want unary=%v", tc.src, n, tc.wantUnary)
			}
			if isUnary && u.Op != "+@" {
				t.Errorf("%s: operator is %q, want %q — it is the method name the "+
					"consumer sends, and must not collide with binary +", tc.src, u.Op, "+@")
			}
		})
	}
}

// TestUnaryPlusOnALiteralKeepsItsPostfix: the folded form is still an ordinary
// literal, so a method call or an exponent binds to it as it would without the
// sign. `+2**3` is 8 and `+1.abs` is 1 on ruby 4.0.5.
func TestUnaryPlusOnALiteralKeepsItsPostfix(t *testing.T) {
	if n := lastNode(t, "+1.abs"); n == nil {
		t.Fatal("+1.abs did not parse")
	} else if _, isUnary := n.(*ast.UnaryExpr); isUnary {
		t.Errorf("+1.abs built a unary node; the literal should have absorbed the sign")
	}
	if _, err := parser.Parse("+2**3"); err != nil {
		t.Errorf("+2**3 did not parse: %v", err)
	}
}
