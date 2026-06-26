package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// mustParseR5 parses src, failing the test on a parse error. It is the shared
// success oracle for the Round-5 features, each of which is MRI-valid (verified
// with `ruby -c` against MRI 4.0.x).
func mustParseR5(t *testing.T, src string) *ast.Program {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	return prog
}

// mustFailR5 asserts that src is rejected, used for the constructs MRI itself
// rejects so the parser's matching behaviour is pinned.
func mustFailR5(t *testing.T, src string) {
	t.Helper()
	if _, err := parser.Parse(src); err == nil {
		t.Fatalf("Parse(%q): expected a parse error, got none", src)
	}
}

// TestR5ParenlessDefParams covers a method definition whose parameters have no
// surrounding parentheses, including leading *splat / **double-splat and a
// trailing &block (`def f a, b`, `def initialize *names`, `def g *a, &b`).
func TestR5ParenlessDefParams(t *testing.T) {
	for _, src := range []string{
		"def initialize *argnames\nend\n",
		"def f a, b\nend\n",
		"def g *a, &b\nend\n",
		"def h a, *b, c:\nend\n",
		"def k a: 1, b: 2\nend\n",
		"def dd **opts\nend\n",
		"def m a, b = 2, *c, d:, e: 5, **f, &g\nend\n",
	} {
		mustParseR5(t, src)
	}
}

// TestR5DestructuringDefParams covers a parenthesised destructuring positional
// parameter in a def list, including a further-nested group.
func TestR5DestructuringDefParams(t *testing.T) {
	for _, src := range []string{
		"def f((a, b), c)\n  a + b + c\nend\n",
		"def make_tmpname((prefix, suffix), n)\n  [prefix, suffix, n]\nend\n",
		"def g(a, (b, c), *d)\n  [a, b, c, d]\nend\n",
		"def h((a, (b, c)), d)\n  [a, b, c, d]\nend\n",
		"def i((a, *b), c)\n  [a, b, c]\nend\n",
		"def j((a, b), (c, d))\n  [a, b, c, d]\nend\n", // two groups, first not last
		"def k((a, *), c)\n  [a, c]\nend\n",            // anonymous splat inside a group
		"def m((a, b)) = a + b\n",                      // endless method with a destructure
	} {
		mustParseR5(t, src)
	}
	// A paren-less destructuring head is a SyntaxError in MRI too.
	mustFailR5(t, "def f (a, b), c\n  a\nend\n")
}

// TestR5DestructuringDefBody verifies a destructuring parameter expands to a
// leading multiple-assignment prepended to the method body.
func TestR5DestructuringDefBody(t *testing.T) {
	prog := mustParseR5(t, "def f((a, b), c)\n  a\nend\n")
	md := prog.Body[0].(*ast.MethodDef)
	if len(md.Body) == 0 {
		t.Fatalf("empty body")
	}
	ma, ok := md.Body[0].(*ast.MultiAssign)
	if !ok {
		t.Fatalf("body[0] = %T, want *ast.MultiAssign (the destructure unpack)", md.Body[0])
	}
	if len(ma.Names) != 2 || ma.Names[0] != "a" || ma.Names[1] != "b" {
		t.Fatalf("unpack names = %v, want [a b]", ma.Names)
	}
}

// TestR5CharLiteralEscapes covers hex / unicode / octal / control / meta escapes
// in a `?x` character literal (the simple escapes were already supported).
func TestR5CharLiteralEscapes(t *testing.T) {
	for _, src := range []string{
		"x = ?\\x00\n",
		"x = ?\\xff\n",
		"x = ?\\u00e9\n",
		"x = ?\\u{1f600}\n",
		"x = ?\\C-a\n",
		"x = ?\\M-x\n",
		"x = ?\\M-\\C-a\n",
		"x = ?\\012\n",
		"x.split(?\\x00)\n",
		"x = ?\\s\n",
		"x = ?a\n",
	} {
		mustParseR5(t, src)
	}
}

// TestR5BlockParams covers an empty `||` parameter list and block-local
// variables declared after a `;` in the parameter list.
func TestR5BlockParams(t *testing.T) {
	for _, src := range []string{
		"x { || 'yay' }\n",
		"lambda { || x }\n",
		"proc { || }\n",
		"x { |a; y, z| a }\n",
		"x { |a; y| a }\n",
		"x { |; y| y }\n",
		"x.each { |a, b; tmp| tmp }\n",
		"x do |a; t| t end\n",
	} {
		mustParseR5(t, src)
	}
}

// TestR5RegexInIndexAndInterp covers a regexp literal in index/value position and
// a nested regexp inside an interpolation inside an outer regexp.
func TestR5RegexInIndexAndInterp(t *testing.T) {
	for _, src := range []string{
		"x[/\\A\\s*/]\n",
		"obj[/re/]\n",
		"h[/re/] = v\n",
		"x[/\\A\\s*/, 1]\n",
		"x = /^#{x[/\\A\\s*/]}/\n",
		"x = /^#{ scan(/^\\s*/).min }/\n",
		"x = /^#{ scan(/^\\s*/).min_by { |l| l.size } }/\n",
		"%r\"^#{Regexp.escape 'import \"x\"'}\"\n",
	} {
		mustParseR5(t, src)
	}
}

// TestR5ControlFlowCommandArg covers a paren-less command whose argument is a
// `yield(...)` (or bare `yield`) control-flow call.
func TestR5ControlFlowCommandArg(t *testing.T) {
	for _, src := range []string{
		"raise yield('x')\n",
		"raise yield\n",
		"foo yield(1)\n",
		"puts yield\n",
		"raise super(x)\n",
	} {
		mustParseR5(t, src)
	}
}

// TestR5ItMethodCall covers RSpec's `it { … }` / `it do … end`: a bare `it`
// taking a block is a method call, not the Ruby-3.4 implicit block parameter.
func TestR5ItMethodCall(t *testing.T) {
	for _, src := range []string{
		"describe Foo do\n  it { is_expected.to be_versionable }\nend\n",
		"describe Foo do\n  it do\n    expect(1).to eq 1\n  end\nend\n",
	} {
		prog := mustParseR5(t, src)
		// The inner `it` is a Call carrying a block.
		outer := prog.Body[0].(*ast.Call).Block
		inner := outer.Body[0]
		c, ok := inner.(*ast.Call)
		if !ok || c.Name != "it" || c.Block == nil {
			t.Fatalf("inner it = %T, want a *ast.Call named it with a block", inner)
		}
	}
	// Bare `it` with no block is still the implicit parameter.
	prog := mustParseR5(t, "[1].map { it * 2 }\n")
	blk := prog.Body[0].(*ast.Call).Block
	if len(blk.Params) != 1 || blk.Params[0] != "it" {
		t.Fatalf("implicit it params = %v, want [it]", blk.Params)
	}
}

// TestR5LocalAndConstCommandArg covers a bareword that is a known local, or a
// constant, promoted back to a method call by a following command argument
// (`type = 1; type 'X'` → `type('X')`; `raise ArgumentError urn.inspect`).
func TestR5LocalAndConstCommandArg(t *testing.T) {
	for _, src := range []string{
		"type = 1\ntype 'X = Y'\n",
		"def f(type)\n  type 'X = Y'\nend\n",
		"fork = false\npid = fork do\n  x\nend\n",
		"raise ArgumentError urn.inspect unless match\n",
		"X y, z\n",
		"type = 1\ntype x do\n  y\nend\n", // local promoted to a call taking a do block
	} {
		mustParseR5(t, src)
	}
	// Operators on an in-scope local stay binary, and a plain constant stays a
	// reference — neither is mis-promoted to a call.
	prog := mustParseR5(t, "x = 5\nx - 1\n")
	if _, ok := prog.Body[1].(*ast.BinaryExpr); !ok {
		t.Fatalf("x - 1 = %T, want *ast.BinaryExpr", prog.Body[1])
	}
	prog = mustParseR5(t, "y = Foo\n")
	if _, ok := prog.Body[0].(*ast.Assign).Value.(*ast.ConstRef); !ok {
		t.Fatalf("Foo = %T, want *ast.ConstRef", prog.Body[0].(*ast.Assign).Value)
	}
}

// TestR5KeywordAndAssignArgs covers a keyword/assignment value that must not
// swallow the following argument, and command-argument multiple-assignment
// suppression (`f a(1), b = c`, `assert_equal a, tag = b`).
func TestR5KeywordAndAssignArgs(t *testing.T) {
	for _, src := range []string{
		"f a: x = 1, b: y = 2\n",
		"f(a: x = 1, b: y = 2)\n",
		"assert_equal tags(:g), tag = posts(:w).tags.first\n",
		"f a(1), b = c\n",
		"f \\\n  title: title = \"x\",\n  body: body = \"y\"\n",
	} {
		mustParseR5(t, src)
	}
	// A real statement-level masgn still gathers its RHS array.
	prog := mustParseR5(t, "a, b = 1, 2\n")
	if _, ok := prog.Body[0].(*ast.MultiAssign); !ok {
		t.Fatalf("a, b = 1, 2 = %T, want *ast.MultiAssign", prog.Body[0])
	}
	// A masgn inside a nested body (command-arg begin) is not suppressed.
	mustParseR5(t, "p begin\n  a, b = z\nend\n")
}

// TestR5LabelValueOnNextLine covers a `key:` whose value sits on the next line
// inside a parenthesised call or a hash literal, distinguished from the
// value-omitted shorthand.
func TestR5LabelValueOnNextLine(t *testing.T) {
	for _, src := range []string{
		"f(\n  k:\n    v,\n)\n",
		"{\n  k:\n    Foo::Bar.new(1),\n}\n",
		"f(a:, b:)\n",
		"f(\n  a:,\n  b:,\n)\n",
		"{ a:, b: }\n",
	} {
		mustParseR5(t, src)
	}
}

// TestR5SetterDefNewlineBody covers a paren-less setter `def x=` whose body
// begins on the next line (the setter `=` must not continue the line), and a
// setter defined with a keyword name (`def ensure=(v)`).
func TestR5SetterDefNewlineBody(t *testing.T) {
	for _, src := range []string{
		"def x=\n  foo do\n  end\nend\n",
		"def test_session_auth=\n  assert_raise(X) do\n    y\n  end\nend\n",
		"def x= y\n  y\nend\n",
		"def x=(v)\n  v\nend\n",
		"def ensure=(v)\n  v\nend\n",
		"def class=(v)\n  v\nend\n",
	} {
		mustParseR5(t, src)
	}
	// A hugging `=` outside a def is ordinary assignment and still continues a
	// line onto the next.
	mustParseR5(t, "x=\n5\n")
	mustParseR5(t, "h = {}\nh[:a]=\n5\n")
}

// TestR5ForLoop covers the `for VAR[, VAR…] in ITER [do] … end` loop, recording
// the loop variables and iterator in a For node.
func TestR5ForLoop(t *testing.T) {
	prog := mustParseR5(t, "for a, b in pairs\n  a\nend\n")
	f, ok := prog.Body[0].(*ast.For)
	if !ok {
		t.Fatalf("body[0] = %T, want *ast.For", prog.Body[0])
	}
	if len(f.Vars) != 2 || f.Vars[0] != "a" || f.Vars[1] != "b" {
		t.Fatalf("for vars = %v, want [a b]", f.Vars)
	}
	for _, src := range []string{
		"for i in 1..100\n  x\nend\n",
		"for i in [1, 2] do\n  x\nend\n",
		"for x in foo.bar do\n  z\nend\n",
		"for i in 1..3\n  y\nend\np i\n",
	} {
		mustParseR5(t, src)
	}
}

// TestR5SuperYieldDivision covers `super`/`yield` used as bare values with a
// division operator (`super / x`), distinguished from a regexp argument
// (`super /re/`) by the surrounding-space heuristic.
func TestR5SuperYieldDivision(t *testing.T) {
	for _, src := range []string{
		"def f\n  super / x\nend\n",
		"super / @step_size\n",
		"yield / 2\n",
		"super /foo/\n",
		"yield /foo/\n",
	} {
		mustParseR5(t, src)
	}
}

// TestR5PatternMatchCondAndMultiline covers a one-line pattern match in a
// condition (`if node in Foo[...]`) and a bracket pattern body spanning
// newlines.
func TestR5PatternMatchCondAndMultiline(t *testing.T) {
	for _, src := range []string{
		"if node in Foo[a: 1]\n  x\nend\n",
		"if node in Foo[\n  a: 1\n]\n  x\nend\n",
		"n in Foo[\n  a: 1\n]\n",
		"node in Foo[\n  name: :x,\n  args: Bar[a: [b]]\n]\n",
		"x in [\n  1,\n  2\n]\n",
	} {
		mustParseR5(t, src)
	}
}

// TestR5InterpolationLexerEdges exercises the verbatim-copy of an interpolation
// inside a regexp / %-literal: nested braces, a `'`/`"` string holding the
// delimiter, backslash escapes, and unterminated forms (which the parser
// surfaces as an error rather than crashing).
func TestR5InterpolationLexerEdges(t *testing.T) {
	for _, src := range []string{
		"x = /a#{ {b: 1} }/\n",      // nested braces in interpolation
		"x = /a#{ \"x/y\" }/\n",     // double-quoted string holding the delimiter
		"x = /a#{ 'x/y' }/\n",       // single-quoted string holding the delimiter
		"x = /a#{ \"x\\\"y\" }b/\n", // escaped quote inside the string
		"x = /a#{ \"x\\\" }/\n",     // backslash escape inside the interpolation string
		"x = /a#{ b\\\nc }/\n",      // backslash escape inside the interpolation itself
		"%r\"#{ \"a/b\" }\"\n",      // %r with a delimiter-bearing string in interp
		"x[/\\xff/]\n",              // hex escape consumed fully in a regexp index
	} {
		mustParseR5(t, src)
	}
	// Unterminated interpolation / quoted string inside a regexp must not crash:
	// the lexer copy-loops stop at EOF and the parser still produces a result
	// (the literal is taken as terminated at EOF, matching the bare-`/` behaviour).
	for _, src := range []string{
		"x = /a#{ \"unterminated\n",
		"x = /a#{ 'unterminated\n",
		"x = /a#{ unterminated\n",
		"x = /a#{ \"x\\",
	} {
		if _, err := parser.Parse(src); err != nil {
			// A reported error is acceptable too; the point is no panic.
			_ = err
		}
	}
}

// TestR5CharEscapeShortForms exercises the char-escape consumers on truncated
// input: a `\x` with fewer than two hex digits and a bare `\u{` without a close.
func TestR5CharEscapeShortForms(t *testing.T) {
	for _, src := range []string{
		"x = ?\\x9\n", // single hex digit (consumeHexEscape stops early)
		"x = ?\\0\n",  // single octal digit
		"x = ?\\C-\\M-a\n",
	} {
		mustParseR5(t, src)
	}
}

// TestR5RangeEndpointAssign covers an assignment / op-assignment as a range's
// high bound (`a..b = c`, `@i...@i += n`), which MRI parses as the endpoint.
func TestR5RangeEndpointAssign(t *testing.T) {
	for _, src := range []string{
		"x = (@a...@a += b.size)\n",
		"self << (@bind_index...@bind_index += binds.size).map(&block).join(\", \")\n",
		"x = (a..b = c)\n",
		"x = (1..5)\n",
		"x = (1..)\n",
	} {
		mustParseR5(t, src)
	}
}
