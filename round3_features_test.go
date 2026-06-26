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

// --- Feature 2: multi-line ternary ---

func TestMultiLineTernary(t *testing.T) {
	for _, src := range []string{
		"a=1\nb=2\nc=3\nx = a ?\n  b :\n  c",
		"a=1\nb=2\nc=3\nx = a ? b :\n  c",
		"a=1\nb=2\nc=3\nx = a ?\n  b : c",
		"x=1\ny=2\nz=3\np x ?\ny :\nz",
	} {
		if _, err := parser.Parse(src); err != nil {
			t.Errorf("Parse(%q): %v", src, err)
		}
	}
}

func TestMultiLineTernaryShape(t *testing.T) {
	prog := mustParse(t, "a=1\nb=2\nc=3\nx = a ?\n  b :\n  c")
	asn := prog.Body[3].(*ast.Assign)
	if _, ok := asn.Value.(*ast.If); !ok {
		t.Fatalf("multi-line ternary RHS = %T, want *ast.If", asn.Value)
	}
}

// --- Feature 3: adjacent string-literal concatenation ---

func TestAdjacentStringConcat(t *testing.T) {
	s, ok := mustParseSingle(t, `"a" "b" "c"`).(*ast.StringLit)
	if !ok || s.Value != "abc" {
		t.Fatalf(`"a" "b" "c" = %#v, want StringLit "abc"`, mustParseSingle(t, `"a" "b" "c"`))
	}
}

func TestAdjacentStringConcatSingleQuote(t *testing.T) {
	s := mustParseSingle(t, `'x' 'y'`).(*ast.StringLit)
	if s.Value != "xy" {
		t.Fatalf(`'x' 'y' = %q, want "xy"`, s.Value)
	}
}

func TestAdjacentStringConcatBackslashContinued(t *testing.T) {
	s := mustParseSingle(t, "\"a\" \\\n  \"b\"").(*ast.StringLit)
	if s.Value != "ab" {
		t.Fatalf(`backslash-continued concat = %q, want "ab"`, s.Value)
	}
}

func TestAdjacentStringConcatWithInterp(t *testing.T) {
	// A plain piece adjacent to an interpolated one folds into one StrInterp.
	for _, src := range []string{
		`"a" "b#{1}"`,
		`"a#{1}" "b"`,
		`"a#{1}" "b#{2}"`,
	} {
		n := mustParseSingle(t, src)
		if _, ok := n.(*ast.StrInterp); !ok {
			t.Errorf("Parse(%q): node = %T, want *ast.StrInterp", src, n)
		}
	}
}

// --- Feature 1: string-adjacent command argument (no space) ---

func TestHuggingStringCommandArg(t *testing.T) {
	for _, src := range []string{
		`foo"bar"`,
		`p"hello"`,
		`assert"x", y`,
		`foo"a""b"`,
		`foo"a#{1}"`,
	} {
		call, ok := mustParseSingle(t, src).(*ast.Call)
		if !ok {
			t.Errorf("Parse(%q): node = %T, want *ast.Call", src, mustParseSingle(t, src))
			continue
		}
		if len(call.Args) == 0 {
			t.Errorf("Parse(%q): call has no args", src)
		}
	}
}

func TestHuggingStringCommandArgWithBlock(t *testing.T) {
	call := mustParseSingle(t, `step"Ensure something" do; end`).(*ast.Call)
	if call.Name != "step" || call.Block == nil {
		t.Fatalf("step\"...\" do…end: name=%q block=%v, want step + a block", call.Name, call.Block)
	}
	if s, ok := call.Args[0].(*ast.StringLit); !ok || s.Value != "Ensure something" {
		t.Fatalf("step arg = %#v, want StringLit", call.Args[0])
	}
}

func TestHuggingStringOnLocalIsCall(t *testing.T) {
	// `x"y"` is a command call even though x is a local (MRI: x("y")).
	prog := mustParse(t, "x = 1\nx\"y\"")
	call, ok := prog.Body[1].(*ast.Call)
	if !ok || call.Name != "x" {
		t.Fatalf("x\"y\" = %#v, want a Call named x", prog.Body[1])
	}
}

func TestHuggingStringOnReceiverAndConst(t *testing.T) {
	for _, src := range []string{`obj.foo"bar"`, `Foo"bar"`} {
		if _, ok := mustParseSingle(t, src).(*ast.Call); !ok {
			t.Errorf("Parse(%q): node = %T, want *ast.Call", src, mustParseSingle(t, src))
		}
	}
}

// --- Feature 9: block params on a continued line ---

func TestBlockParamsOnContinuedLine(t *testing.T) {
	for _, src := range []string{
		"foo {\n|x| x }",
		"foo do\n|a, b|\na + b\nend",
		"[1].each {\n  |x|\n  p x\n}",
		"foo do\n\n|compare, target, success|\nnil\nend",
	} {
		if _, err := parser.Parse(src); err != nil {
			t.Errorf("Parse(%q): %v", src, err)
		}
	}
}

func TestBlockParamsContinuedShape(t *testing.T) {
	call := mustParseSingle(t, "foo do\n|a, b|\na\nend").(*ast.Call)
	if call.Block == nil {
		t.Fatal("foo do\\n|a,b| …: no block")
	}
	if len(call.Block.Params) != 2 || call.Block.Params[0] != "a" || call.Block.Params[1] != "b" {
		t.Fatalf("block params = %v, want [a b]", call.Block.Params)
	}
}

func TestBlockWithoutParamsStillWorks(t *testing.T) {
	// A body that simply starts on the next line must not be read as params.
	for _, src := range []string{"foo {\nx }", "foo {\n}", "foo do\nx\nend"} {
		if _, err := parser.Parse(src); err != nil {
			t.Errorf("Parse(%q): %v", src, err)
		}
	}
}

// --- Feature 5: control-flow + modifier inside parens ---

func TestControlFlowInsideParens(t *testing.T) {
	for _, src := range []string{
		`(set_param(p) and return) unless cur`,
		`[1].each { ( x or next )[1] }`,
		`(expr if cond)`,
		`(x if y) || z`,
		`[1].each { (m.match(s) or next)[1] }`,
	} {
		if _, err := parser.Parse(src); err != nil {
			t.Errorf("Parse(%q): %v", src, err)
		}
	}
}

// --- Feature 8: control-flow keyword as expression operand ---

func TestControlFlowKeywordAsOperand(t *testing.T) {
	for _, src := range []string{
		`def f; x || return; end`,
		`[1].each { |x| x || next }`,
		`[1].each { |x| x || break }`,
		`while true; x || break; end`,
		`a = b || return`,
		`def f; x or return; end`,
		`def f; x && return; end`,
	} {
		if _, err := parser.Parse(src); err != nil {
			t.Errorf("Parse(%q): %v", src, err)
		}
	}
}

func TestControlFlowOperandRejectsValue(t *testing.T) {
	// MRI: a value after the jump in operand position is a syntax error.
	if _, err := parser.Parse(`def f; x || return 5; end`); err == nil {
		t.Fatalf("x || return 5: expected a parse error")
	}
}

func TestControlFlowOperandShape(t *testing.T) {
	ifn := mustParseSingle(t, "def f; x || return; end").(*ast.MethodDef)
	be := ifn.Body[0].(*ast.BinaryExpr)
	if _, ok := be.Right.(*ast.Return); !ok {
		t.Fatalf("RHS of || = %T, want *ast.Return", be.Right)
	}
}

// --- Feature 4: assignment inside a condition / expression ---

func TestInlineAssignmentInCondition(t *testing.T) {
	for _, src := range []string{
		`if a && b = c; end`,
		`if ct && m = RE.match(s); end`,
		`if a || b = c; end`,
		`while x && line = gets; end`,
		`r = a && @b = 1`,
		`if a && @@c = 2; end`,
		`if a && $g = 3; end`,
		`if a && K = 4; end`,
		`if a && h[k] = 5; end`,
		`if a && o.attr = 6; end`,
	} {
		if _, err := parser.Parse(src); err != nil {
			t.Errorf("Parse(%q): %v", src, err)
		}
	}
}

func TestInlineAssignmentShape(t *testing.T) {
	// `if a && b = c` is `if a && (b = c)`.
	ifn := mustParseSingle(t, "if a && b = c; end").(*ast.If)
	be := ifn.Cond.(*ast.BinaryExpr)
	if be.Op != "&&" {
		t.Fatalf("cond op = %q, want &&", be.Op)
	}
	if _, ok := be.Right.(*ast.Assign); !ok {
		t.Fatalf("RHS of && = %T, want *ast.Assign", be.Right)
	}
}

func TestInlineAssignmentDoesNotEatComparisons(t *testing.T) {
	// `==`, `=>`, `=~` must not be swallowed as assignments.
	for _, src := range []string{`p(a == b)`, `p(a =~ b)`, `x == y`} {
		if _, err := parser.Parse(src); err != nil {
			t.Errorf("Parse(%q): %v", src, err)
		}
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
