package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// mustParse parses src, failing the test on error.
func mustParse(t *testing.T, src string) *ast.Program {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	return prog
}

// --- Feature 6: character literals ?x ---

// charLitValue digs the single-character StringLit out of `p ?x` (a Call whose
// sole argument is the literal).
func charLitArg(t *testing.T, src string) string {
	t.Helper()
	call, ok := mustParseSingle(t, src).(*ast.Call)
	if !ok {
		t.Fatalf("Parse(%q): top node is not a Call", src)
	}
	if len(call.Args) != 1 {
		t.Fatalf("Parse(%q): want 1 arg, got %d", src, len(call.Args))
	}
	s, ok := call.Args[0].(*ast.StringLit)
	if !ok {
		t.Fatalf("Parse(%q): arg is %T, want *ast.StringLit", src, call.Args[0])
	}
	return s.Value
}

func TestCharLiteralBasic(t *testing.T) {
	cases := map[string]string{
		`p ?a`:    "a",
		`p ?|`:    "|",
		`p ?/`:    "/",
		`p ?Z`:    "Z",
		`p ?1`:    "1",
		`p ?.`:    ".",
		"p ?\\n":  "\n",
		"p ?\\t":  "\t",
		"p ?\\s":  " ",
		"p ?\\\\": "\\",
		"p ?\\e":  "\x1b",
		"p ?\\0":  "\x00",
		`p ?é`:    "é",
	}
	for src, want := range cases {
		if got := charLitArg(t, src); got != want {
			t.Errorf("%q: char value = %q, want %q", src, got, want)
		}
	}
}

func TestCharLiteralAtExprBegin(t *testing.T) {
	asn := mustParseSingle(t, `x = ?a`).(*ast.Assign)
	s, ok := asn.Value.(*ast.StringLit)
	if !ok || s.Value != "a" {
		t.Fatalf("x = ?a: RHS = %#v, want StringLit \"a\"", asn.Value)
	}
}

func TestCharLiteralWithPostfix(t *testing.T) {
	// `p ?a.upcase` is `p((?a).upcase)`, not a ternary.
	call := mustParseSingle(t, `p ?a.upcase`).(*ast.Call)
	inner, ok := call.Args[0].(*ast.Call)
	if !ok || inner.Name != "upcase" {
		t.Fatalf("p ?a.upcase: arg = %#v, want .upcase call", call.Args[0])
	}
	if s, ok := inner.Recv.(*ast.StringLit); !ok || s.Value != "a" {
		t.Fatalf("p ?a.upcase: receiver = %#v, want StringLit \"a\"", inner.Recv)
	}
}

func TestQuestionStaysTernaryAfterValue(t *testing.T) {
	// After a finished value the `?` is the ternary operator, never a char
	// literal, even when a digit hugs it: `p 1 ?2 :3` == `p(1 ? 2 : 3)`.
	call := mustParseSingle(t, `p 1 ?2 :3`).(*ast.Call)
	if _, ok := call.Args[0].(*ast.If); !ok {
		t.Fatalf("p 1 ?2 :3: arg = %T, want a ternary (*ast.If)", call.Args[0])
	}
}

func TestTernaryWithSpacedQuestion(t *testing.T) {
	// `x ? y : z` over locals stays a ternary.
	prog := mustParse(t, "x=1\ny=2\np x ? y : 0")
	call := prog.Body[2].(*ast.Call)
	if _, ok := call.Args[0].(*ast.If); !ok {
		t.Fatalf("x ? y : 0: arg = %T, want a ternary (*ast.If)", call.Args[0])
	}
}

func TestQuestionNotCharLiteralForWord(t *testing.T) {
	// `?ab` is not a char literal (two chars) — MRI rejects it; here `?` after a
	// value position with a word also is not a single-char literal.
	if _, err := parser.Parse(`p ?ab`); err == nil {
		t.Fatalf("p ?ab: expected a parse error (not a char literal)")
	}
}

// --- Feature 7: generalized percent-literal delimiters ---

func TestPercentDelimiterVariants(t *testing.T) {
	// Any non-alphanumeric delimiter opens the literal; the result node is the
	// same as the bracket-delimited form.
	for _, src := range []string{
		`%r"abc"`, `%r|ab|`, `%r#ab#`,
		`%Q'hi'`, `%Q@hi@`, `%Q!hi!`,
		`%q[x]`, `%q@x@`, `%q'x'`,
		`%w'a b'`, `%w*a b*`, `%w|a b|`,
		`%i<x y>`, `%i!x y!`,
		`%W{a b}`, `%I<a b>`,
		`%(hi)`, `%!hi!`, `%@hi@`,
		`%s"sym"`, `%s|sym|`,
		`%x"echo hi"`, `%x|ls|`,
	} {
		if _, err := parser.Parse(src); err != nil {
			t.Errorf("Parse(%q): %v", src, err)
		}
	}
}

func TestPercentArrayQuoteDelim(t *testing.T) {
	arr := mustParseSingle(t, `%w'apple pear'`).(*ast.ArrayLit)
	if len(arr.Elems) != 2 {
		t.Fatalf(`%%w'apple pear': got %d elems, want 2`, len(arr.Elems))
	}
	if s, ok := arr.Elems[0].(*ast.StringLit); !ok || s.Value != "apple" {
		t.Fatalf("first elem = %#v, want StringLit \"apple\"", arr.Elems[0])
	}
}

func TestPercentRegexpQuoteDelim(t *testing.T) {
	re := mustParseSingle(t, `%r"a.c"i`).(*ast.RegexpLit)
	if re.Source != "a.c" || re.Flags != "i" {
		t.Fatalf(`%%r"a.c"i: source=%q flags=%q, want "a.c"/"i"`, re.Source, re.Flags)
	}
}

func TestPercentDoesNotBreakModuloOrOpAssign(t *testing.T) {
	// `%=` stays a compound assignment; `a % 3` stays modulo.
	prog := mustParse(t, "a = 10\na %= 3")
	if _, ok := prog.Body[1].(*ast.OpAssign); !ok {
		t.Fatalf("a %%= 3: node = %T, want *ast.OpAssign", prog.Body[1])
	}
	prog = mustParse(t, "a = 10\np a % 3")
	call := prog.Body[1].(*ast.Call)
	if _, ok := call.Args[0].(*ast.BinaryExpr); !ok {
		t.Fatalf("a %% 3: arg = %T, want *ast.BinaryExpr (modulo)", call.Args[0])
	}
}
