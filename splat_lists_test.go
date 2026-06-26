package parser

import (
	"testing"

	"github.com/go-ruby-parser/parser/ast"
)

// A `*splat` is accepted in a rescue class list, a when candidate list, and a
// multiple-assignment right-hand side.
func TestSplatInRescueWhenMasgn(t *testing.T) {
	for _, src := range []string{
		"begin\nrescue *EXCEPTIONS => e\nend",
		"begin\nrescue *FOO, Bar => e\nend",
		"case x\nwhen *LIST\n  1\nend",
		"case x\nwhen 1, *rest\n  1\nend",
		"path, headers = *args",
		"a, b = 1, *rest",
	} {
		if _, err := Parse(src); err != nil {
			t.Fatalf("Parse(%q) error: %v", src, err)
		}
	}
}

// The rescue/when/masgn splats are represented as SplatArg nodes.
func TestSplatNodeShapes(t *testing.T) {
	// rescue
	prog, err := Parse("begin\nrescue *EX => e\nend")
	if err != nil {
		t.Fatalf("rescue parse: %v", err)
	}
	beg := prog.Body[0].(*ast.Begin)
	if _, ok := beg.Rescues[0].Classes[0].(*ast.SplatArg); !ok {
		t.Fatalf("rescue class0 = %T, want *ast.SplatArg", beg.Rescues[0].Classes[0])
	}

	// when
	prog, err = Parse("case x\nwhen *L\n 1\nend")
	if err != nil {
		t.Fatalf("when parse: %v", err)
	}
	cs := prog.Body[0].(*ast.Case)
	if _, ok := cs.Whens[0].Conds[0].(*ast.SplatArg); !ok {
		t.Fatalf("when cond0 = %T, want *ast.SplatArg", cs.Whens[0].Conds[0])
	}

	// masgn RHS
	prog, err = Parse("a, b = *args")
	if err != nil {
		t.Fatalf("masgn parse: %v", err)
	}
	ma := prog.Body[0].(*ast.MultiAssign)
	if _, ok := ma.Values[0].(*ast.SplatArg); !ok {
		t.Fatalf("masgn value0 = %T, want *ast.SplatArg", ma.Values[0])
	}
}
