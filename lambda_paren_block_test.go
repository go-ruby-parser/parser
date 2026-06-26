package parser

import (
	"testing"

	"github.com/go-ruby-parser/parser/ast"
)

// A stabby lambda accepts unparenthesized parameters: `->x { }`, `-> ctx { }`,
// `->a, b { }`, `-> message do … end`, `->x, y=1 { }`.
func TestStabbyLambdaUnparenthesizedParams(t *testing.T) {
	cases := []struct {
		src    string
		params []string
	}{
		{`->x { x }`, []string{"x"}},
		{`-> ctx { ctx }`, []string{"ctx"}},
		{`->a, b { a + b }`, []string{"a", "b"}},
		{`-> message do message end`, []string{"message"}},
		{`->x, y=1 { x }`, []string{"x", "y"}},
		{`-> data { queue << data }`, []string{"data"}},
	}
	for _, c := range cases {
		prog, err := Parse(c.src)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", c.src, err)
		}
		call := prog.Body[0].(*ast.Call)
		if call.Name != "lambda" || call.Block == nil {
			t.Fatalf("%q: expected lambda call with block, got %#v", c.src, call)
		}
		if len(call.Block.Params) != len(c.params) {
			t.Fatalf("%q: params=%v, want %v", c.src, call.Block.Params, c.params)
		}
		for i, p := range c.params {
			if call.Block.Params[i] != p {
				t.Fatalf("%q: param %d=%q, want %q", c.src, i, call.Block.Params[i], p)
			}
		}
	}
}

// Keyword parameters are accepted in block/lambda parameter lists.
func TestBlockKeywordParams(t *testing.T) {
	for _, src := range []string{
		`[1].each { |a, b:| a }`,
		`foo do |channel, count: nil, timeout: 5| 1 end`,
		`m { |k:| k }`,
		`->a, k: 1 { a }`,
	} {
		if _, err := Parse(src); err != nil {
			t.Fatalf("Parse(%q) error: %v", src, err)
		}
	}
}

// A keyword block param is recorded with a trailing-colon sentinel name.
func TestBlockKeywordParamShape(t *testing.T) {
	prog, err := Parse(`[1].each { |a, b:, c: 5| a }`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	call := prog.Body[0].(*ast.Call)
	want := []string{"a", "b:", "c:"}
	if len(call.Block.Params) != len(want) {
		t.Fatalf("params=%v, want %v", call.Block.Params, want)
	}
	for i, w := range want {
		if call.Block.Params[i] != w {
			t.Fatalf("param %d=%q, want %q", i, call.Block.Params[i], w)
		}
	}
}

// A parenthesised group accepts trailing modifiers and statement sequences,
// evaluating to its last expression.
func TestModifierAndSequenceInParens(t *testing.T) {
	for _, src := range []string{
		`(expr if cond)`,
		`(x if y) || z`,
		`(a; b)`,
		`x = (a = 1; a + 1)`,
		`(do_it unless skip)`,
	} {
		if _, err := Parse(src); err != nil {
			t.Fatalf("Parse(%q) error: %v", src, err)
		}
	}
}

// A multi-statement parenthesised group becomes a Begin whose value is its last
// statement; a single-statement group returns that statement directly.
func TestParenGroupShape(t *testing.T) {
	prog, err := Parse(`(a; b)`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if _, ok := prog.Body[0].(*ast.Begin); !ok {
		t.Fatalf("expected *ast.Begin for (a; b), got %T", prog.Body[0])
	}
	prog, err = Parse(`(1)`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if _, ok := prog.Body[0].(*ast.IntLit); !ok {
		t.Fatalf("expected *ast.IntLit for (1), got %T", prog.Body[0])
	}
}
