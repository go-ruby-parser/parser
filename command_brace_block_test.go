package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// parseAny parses src and reports only whether it was accepted.
func parseAny(src string) (*ast.Program, error) { return parser.Parse(src) }

// TestBraceBlockAfterSpacedParenArgument covers MRI's tLPAREN_ARG/EXPR_ENDARG
// pair. A SPACED `(` in argument position lexes as tLPAREN_ARG, and its `)`
// sets EXPR_ENDARG (parse.y v3_4_0: `primary: tLPAREN_ARG compstmt
// {SET_LEX_STATE(EXPR_ENDARG);} ')'`), so the `{` right after it lexes as
// tLBRACE_ARG and the grammar gives it to the COMMAND through
// `command: primary_value call_op operation2 command_args cmd_brace_block`.
// A space before an argument list therefore does NOT detach a following brace
// block from the call.
func TestBraceBlockAfterSpacedParenArgument(t *testing.T) {
	t.Parallel()
	t.Run("receiver command", func(t *testing.T) {
		t.Parallel()
		src := `o.s (:a){ 1 }`
		call, ok := parseOne(t, src).(*ast.Call)
		if !ok || call.Name != "s" {
			t.Fatalf("Parse(%q): want a call to s", src)
		}
		if call.Block == nil {
			t.Fatalf("Parse(%q): the brace block did not reach the call", src)
		}
		if len(call.Args) != 1 {
			t.Errorf("Parse(%q): %d args, want 1", src, len(call.Args))
		}
		if _, ok := call.Args[0].(*ast.SymbolLit); !ok {
			t.Errorf("Parse(%q): argument = %T, want *ast.SymbolLit", src, call.Args[0])
		}
	})

	t.Run("self command", func(t *testing.T) {
		t.Parallel()
		src := `f (1){ 2 }`
		call, ok := parseOne(t, src).(*ast.Call)
		if !ok || call.Name != "f" || call.Block == nil {
			t.Fatalf("Parse(%q): want a call to f carrying a block", src)
		}
	})

	t.Run("a hugging paren keeps the old binding", func(t *testing.T) {
		t.Parallel()
		// `f g(1) { 2 }`: g's paren hugs, so its `)` leaves EXPR_END, not
		// EXPR_ENDARG — MRI gives the block to g(1) and f raises LocalJumpError.
		src := `f g(1) { 2 }`
		call, ok := parseOne(t, src).(*ast.Call)
		if !ok || call.Name != "f" {
			t.Fatalf("Parse(%q): want a call to f", src)
		}
		if call.Block != nil {
			t.Errorf("Parse(%q): the block reached f; MRI gives it to g(1)", src)
		}
		inner, ok := call.Args[0].(*ast.Call)
		if !ok || inner.Name != "g" || inner.Block == nil {
			t.Errorf("Parse(%q): the block did not reach g(1)", src)
		}
	})

	t.Run("ordinary blocks are unaffected", func(t *testing.T) {
		t.Parallel()
		for _, src := range []string{`[1].map { |x| x }`, `f(1) { 2 }`, `f 1 { 2 }`} {
			if _, err := parseAny(src); err != nil {
				t.Errorf("Parse(%q) returned error: %v", src, err)
			}
		}
	})
}

// TestNestedDestructuringBlockParameter covers MRI's
// `f_marg: f_norm_arg | tLPAREN f_margs rparen` (parse.y v3_4_0): a
// destructuring block parameter contains f_margs, which contains f_marg again,
// so the parentheses nest to any depth and may also be the outermost parameter.
func TestNestedDestructuringBlockParameter(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		`f { |a, (b, c)| }`,
		`f { |a, (b, (c, d))| }`,
		`f { |((a, b), c)| }`,
		`f { |(a, *b)| }`,
		`->(a, (b, (c, d))) { }`,
	} {
		if _, err := parseAny(src); err != nil {
			t.Errorf("Parse(%q) returned error: %v", src, err)
		}
	}
}
