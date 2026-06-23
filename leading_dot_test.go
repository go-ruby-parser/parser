package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// TestLeadingDotChain: a newline whose next significant token is `.` (or `&.`)
// does not terminate the statement — the dot line chains onto the previous
// expression, as MRI joins such lines in the lexer.
func TestLeadingDotChain(t *testing.T) {
	src := "r = \"abc\"\n  .upcase\n  .reverse"
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	if len(prog.Body) != 1 {
		t.Fatalf("body has %d statements, want 1 (chain joined)", len(prog.Body))
	}
	asn, ok := prog.Body[0].(*ast.Assign)
	if !ok {
		t.Fatalf("top node %T, want *ast.Assign", prog.Body[0])
	}
	outer, ok := asn.Value.(*ast.Call)
	if !ok || outer.Name != "reverse" {
		t.Fatalf("outer call = %#v, want .reverse", asn.Value)
	}
	inner, ok := outer.Recv.(*ast.Call)
	if !ok || inner.Name != "upcase" {
		t.Fatalf("inner call = %#v, want .upcase", outer.Recv)
	}
	if _, ok := inner.Recv.(*ast.StringLit); !ok {
		t.Fatalf("chain base = %T, want *ast.StringLit", inner.Recv)
	}
}

// TestLeadingSafeDotChain: `&.` lines chain just like `.` lines, and the call is
// marked safe.
func TestLeadingSafeDotChain(t *testing.T) {
	src := "r = x\n  &.foo\n  &.bar"
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	if len(prog.Body) != 1 {
		t.Fatalf("body has %d statements, want 1", len(prog.Body))
	}
	outer := prog.Body[0].(*ast.Assign).Value.(*ast.Call)
	if outer.Name != "bar" || !outer.Safe {
		t.Errorf("outer = %#v, want safe .bar", outer)
	}
	if inner := outer.Recv.(*ast.Call); inner.Name != "foo" || !inner.Safe {
		t.Errorf("inner = %#v, want safe .foo", inner)
	}
}

// TestLeadingDotAcrossBlankLinesAndComments: blank lines and comments between
// the expression and the dot line are tolerated.
func TestLeadingDotAcrossBlankLinesAndComments(t *testing.T) {
	for _, src := range []string{
		"r = foo\n\n  .bar",
		"r = foo # trailing\n  .bar",
		"r = foo\n  # standalone\n  .bar",
	} {
		prog, err := parser.Parse(src)
		if err != nil {
			t.Fatalf("Parse(%q): %v", src, err)
		}
		if len(prog.Body) != 1 {
			t.Errorf("%q: body has %d statements, want 1", src, len(prog.Body))
		}
	}
}

// TestNewlineStillTerminates: a normal newline still ends the statement when the
// next line does NOT begin with a leading dot — including a `.b` call followed by
// an ordinary next line, and `..`/`...` ranges (which must not be mistaken for a
// chain dot).
func TestNewlineStillTerminates(t *testing.T) {
	cases := []struct {
		src  string
		want int
	}{
		{"a = 1\nb = 2", 2},
		{"x = a.b\ny = c.d", 2},
		{"r = (1..5)\nputs r", 2},
		{"r = (1...5)\nputs r", 2},
	}
	for _, c := range cases {
		prog, err := parser.Parse(c.src)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.src, err)
		}
		if len(prog.Body) != c.want {
			t.Errorf("%q: %d statements, want %d", c.src, len(prog.Body), c.want)
		}
	}
}
