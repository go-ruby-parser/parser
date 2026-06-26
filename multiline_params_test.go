package parser

import (
	"testing"

	"github.com/go-ruby-parser/parser/ast"
)

// A parenthesised method parameter list may span several lines, with newlines
// after the open paren, around the commas, and before the close paren — and may
// carry scoped-constant / call defaults.
func TestMultilineDefParams(t *testing.T) {
	for _, src := range []string{
		"def h(\n  a,\n  b\n)\nend",
		"def f(a,\n  b = Encoding::UTF_8,\n  c = false)\nend",
		"def f(a,\n  b = 1,\n)\nend", // trailing comma + newline
		"def g(\n)\nend",             // empty multiline list
		"def k(x = Foo.bar, y = baz())\nend",
		"def i(a = CONST::SUB)\nend",
	} {
		if _, err := Parse(src); err != nil {
			t.Fatalf("Parse(%q) error: %v", src, err)
		}
	}
}

// A paren-less parameter list still terminates at the newline (no spill into the
// body).
func TestParenlessDefParamsUnaffected(t *testing.T) {
	prog, err := Parse("def m a, b\n  a + b\nend")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	md, ok := prog.Body[0].(*ast.MethodDef)
	if !ok {
		t.Fatalf("expected *ast.MethodDef, got %T", prog.Body[0])
	}
	if len(md.Params) != 2 || len(md.Body) != 1 {
		t.Fatalf("params=%v body=%d, want 2 params and 1 body stmt", md.Params, len(md.Body))
	}
}
