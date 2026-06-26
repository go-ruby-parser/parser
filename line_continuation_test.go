package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// TestLineContinuationTrailingOperator covers MRI's implicit line continuation: a
// line ending in an infix operator (or comma, or a trailing dot) joins the next
// line, so each source below is a SINGLE statement spanning two physical lines.
func TestLineContinuationTrailingOperator(t *testing.T) {
	oneStmt := []string{
		"a ||\nb",
		"a &&\nb",
		"1 +\n2",
		"1 -\n2",
		"1 *\n2",
		"1 **\n2",
		"1 /\n2",
		"1 %\n2",
		"a ==\nb",
		"a ===\nb",
		"a =~\nb",
		"a !=\nb",
		"a <\nb",
		"a >\nb",
		"a <=\nb",
		"a >=\nb",
		"a <=>\nb",
		"x =\n1",
		"x +=\n1",
		"foo(1,\n2)",
		"h = {a =>\n1}",
		"foo.\nbar",
		"foo&.\nbar",
		"c ?\na : b",
		// Trailing low-precedence keyword operators and modifiers continue too.
		"a and\nb",
		"a or\nb",
		"foo if\nbar",
		"foo unless\nbar",
		"foo while\nbar",
		"foo until\nbar",
		"x = 1 or\nfail",
		"y =~ /re/ or\nraise",
	}
	for _, src := range oneStmt {
		t.Run(src, func(t *testing.T) {
			prog, err := parser.Parse(src)
			if err != nil {
				t.Fatalf("Parse(%q): %v", src, err)
			}
			if len(prog.Body) != 1 {
				t.Fatalf("Parse(%q): %d statements, want 1 (a continuation)", src, len(prog.Body))
			}
		})
	}
}

// TestLineContinuationBoundaries pins the cases that must NOT be glued.
func TestLineContinuationBoundaries(t *testing.T) {
	// A value-less return must not swallow the next line.
	prog, err := parser.Parse("return\n5")
	if err != nil {
		t.Fatalf("Parse(return\\n5): %v", err)
	}
	if len(prog.Body) != 2 {
		t.Fatalf("`return\\n5`: %d statements, want 2", len(prog.Body))
	}
	if _, ok := prog.Body[0].(*ast.Return); !ok {
		t.Fatalf("first statement = %T, want *ast.Return", prog.Body[0])
	}

	// A semicolon is an explicit terminator.
	if prog, err := parser.Parse("a = 1; b = 2"); err != nil || len(prog.Body) != 2 {
		t.Fatalf("`a=1; b=2`: body=%d err=%v, want 2 statements", len(prog.Body), err)
	}

	// Two independent statements separated by a newline stay separate.
	if prog, err := parser.Parse("a = 1\nb = 2"); err != nil || len(prog.Body) != 2 {
		t.Fatalf("two statements: body=%d err=%v, want 2", len(prog.Body), err)
	}

	// A `|`-delimited block-parameter list is not treated as a trailing operator.
	if _, err := parser.Parse("[1].each { |x|\n  x\n}"); err != nil {
		t.Fatalf("block params over two lines: %v", err)
	}
}
