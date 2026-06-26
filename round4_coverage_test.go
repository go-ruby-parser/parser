package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// Round-4 coverage fillers: exercise the remaining branches of the new code
// (paren-group masgn comma path, scoped masgn target rejection, nested OP= on a
// known local, scope-resolution command call with a do-block, and the lexer's
// unknown-character path).

func TestParenGroupThenComma(t *testing.T) {
	// `(a, b), c = …` — a leading paren group followed by another top-level target,
	// hitting the group-then-COMMA branch of looksLikeMlhs and parseMlhs.
	parsesOK(t,
		"(a, b), c = [1, 2], 3\n",
		"(a, b), (c, d) = x, y\n",
	)
}

func TestScopeNotConstNotMasgn(t *testing.T) {
	// A `::` target whose next token is not a CONST is not a masgn LHS, so the
	// scan falls through and the line parses as an ordinary expression.
	parsesOK(t,
		"a::b\n",        // method call via ::, not a target
		"x = a::b, c\n", // RHS array, not an mlhs
	)
	// A `::` at a (later) target-start position followed by a non-constant makes
	// looksLikeMlhs reject the run as a masgn LHS (`a, ::b` — `::b` is not a valid
	// scoped target), so the line is parsed by the ordinary expression path.
	parseErrs(t, "a, ::b = 1, 2\n")
}

func TestNestedOpAssignKnownLocal(t *testing.T) {
	// `count` is a declared local before the nested `count += 1`, so the operand
	// resolves to a VarRef and the VarRef branch of inlineOpAssign runs.
	prog := mustParse(t, "count = 0\nok && count += 1\n")
	if len(prog.Body) != 2 {
		t.Fatalf("want 2 statements, got %d", len(prog.Body))
	}
	bin, ok := prog.Body[1].(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("second statement is %T, want *ast.BinaryExpr", prog.Body[1])
	}
	if _, ok := bin.Right.(*ast.OpAssign); !ok {
		t.Fatalf("RHS of && is %T, want *ast.OpAssign", bin.Right)
	}
}

func TestScopeCommandWithDoBlock(t *testing.T) {
	parsesOK(t,
		"Mod::run x do\n y\nend\n",  // lowercase scope-resolution command + do
		"obj::Down x do\n y\nend\n", // capitalized scope-resolution command + do
		"obj::Down x, y\n",          // capitalized scope-resolution command, no block
	)
}

func TestInlineOpAssignNonAssignable(t *testing.T) {
	// A compound-assignment operator following a non-assignable operand is left
	// unconsumed by inlineOpAssign (it returns nil); the parser then reports the
	// stray operator rather than crashing — exercising the nil-return path.
	if _, err := parser.Parse("a && 1 += 1\n"); err == nil {
		t.Fatalf("expected an error for `1 += 1`")
	}
}

func TestLexerUnknownChar(t *testing.T) {
	// A stray control / unmapped byte yields a clean parse error, not a panic,
	// covering lexToken's final ILLEGAL fallthrough.
	if _, err := parser.Parse("a \x01 b\n"); err == nil {
		t.Fatalf("expected an error for an unknown character")
	}
}
