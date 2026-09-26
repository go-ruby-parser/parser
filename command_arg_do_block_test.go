package parser

import (
	"strings"
	"testing"

	"github.com/go-ruby-parser/parser/ast"
)

// A `do…end` inside a nested body attaches to the call it follows even when that
// body is an argument of a PAREN-LESS command call. MRI resets its CMDARG
// bit-stack at every such entry — `k_begin` (parse.y v3_4_0 4392), the `lambda`
// rule between f_larglist and lambda_body (5182), `do_body` (5378),
// `tSTRING_DBEG` (6181) and the lexer's `(`, `[`, `{` (11193, 11221, 11245) — so
// the `do` there is a plain `keyword_do` and not the `keyword_do_block` that
// would belong to the command (10525-10526).
//
// Each source below was verified to parse under ruby 4.0.5 (`ruby -c`, and
// `ruby --parser=parse.y -c`).
func TestDoBlockInsideParenlessCommandArgument(t *testing.T) {
	for _, src := range []string{
		// the `->` body — the shape this test was written for
		`foo :a, -> { [1].each do |x| x end }`,
		`p foo :a, -> { [1].each do |x| x end }`,
		`foo :a, -> (x) { y do end }`,
		`foo :a, ->(x) { y do end }`,
		`foo :a, ->() { y do end }`,
		`foo :a, -> x { y do end }`,
		`foo :a, -> { y do end }, :b`,
		`foo :a, -> { y do end }.call`,
		`foo :a, -> { -> { y do end } }`,
		`foo :a, -> { z = -> { y do end } }`,
		`foo :a, -> { y do end; z do end }`,
		`foo -> { y do end }`,
		`def m; foo :a, -> { y do end }; end`,
		// the `-> do … end` body: the `lambda` rule's CMDARG_PUSH(0) covers it too
		`foo :a, -> do [1].each do |x| x end end`,
		`foo :a, -> do y do end end, :b`,
		`foo :a, -> do foo :b, [1].each do |x| x end end`,
		// a nested command call inside the lambda body still holds its own `do` back
		`foo :a, -> { begin; y do end; end }`,
		`foo :a, -> do begin; y do end; end end`,
		// k_begin (4392)
		`foo :a, begin; y do end; end`,
		`foo :a, begin; y do end; rescue; z do end; else; u do end; ensure; v do end; end`,
		// tSTRING_DBEG (6181)
		`foo :a, "#{y do end}"`,
		// the lexer's `(`, `[`, `{` (11193, 11221, 11245)
		`foo :a, (y do end)`,
		`foo :a, (y do end; z do end)`,
		`foo (y do end)`,
		`foo :a, [y do end]`,
		`foo :a, {k: y do end}`,
		`foo :a, bar(y do end)`,
		// the other statement bodies MRI also accepts a `do` in
		`foo :a, if c; y do end; end`,
		`foo :a, if c then y do end else z do end end`,
		`foo :a, unless c; y do end; end`,
		`foo :a, while c; y do end; end`,
		`foo :a, until c; y do end; end`,
		`foo :a, case 1 when 1 then y do end end`,
		`foo :a, case x; in 1; y do end; end`,
		`foo :a, def m; y do end; end`,
		`foo :a, class C; y do end; end`,
		`foo :a, module M; y do end; end`,
	} {
		if _, err := Parse(src); err != nil {
			t.Errorf("Parse(%q): %v", src, err)
		}
	}
}

// The block must land on the INNER call, not merely somewhere: the point of the
// CMDARG reset is that `each` — not `foo` — gets the `do…end`.
func TestDoBlockInLambdaBodyAttachesToTheInnerCall(t *testing.T) {
	prog, err := Parse(`foo :a, -> { [1].each do |x| x end }`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cmd, ok := prog.Body[0].(*ast.Call)
	if !ok || cmd.Name != "foo" {
		t.Fatalf("top level = %#v, want a call to foo", prog.Body[0])
	}
	if cmd.Block != nil {
		t.Fatalf("foo took the do…end block; it belongs to `each`")
	}
	if len(cmd.Args) != 2 {
		t.Fatalf("foo args = %d, want 2", len(cmd.Args))
	}
	lam, ok := cmd.Args[1].(*ast.Call)
	if !ok || lam.Name != "lambda" || lam.Block == nil {
		t.Fatalf("second argument = %#v, want a lambda", cmd.Args[1])
	}
	each, ok := lam.Block.Body[0].(*ast.Call)
	if !ok || each.Name != "each" {
		t.Fatalf("lambda body = %#v, want a call to each", lam.Block.Body[0])
	}
	if each.Block == nil {
		t.Fatalf("each has no block")
	}
	if got := each.Block.Params; len(got) != 1 || got[0] != "x" {
		t.Fatalf("each block params = %v, want [x]", got)
	}
}

// A `do…end` AFTER the lambda still belongs to the command call: the CMDARG reset
// ends with the lambda body. `foo :a, -> { } do end` gives the block to foo.
func TestTrailingDoAfterLambdaArgumentBindsToTheCommand(t *testing.T) {
	prog, err := Parse(`foo :a, -> { } do end`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cmd := prog.Body[0].(*ast.Call)
	if cmd.Name != "foo" || cmd.Block == nil {
		t.Fatalf("foo = %#v, want the trailing do…end block", cmd)
	}
	lam := cmd.Args[1].(*ast.Call)
	if lam.Name != "lambda" || len(lam.Block.Body) != 0 {
		t.Fatalf("lambda argument = %#v, want an empty body", lam)
	}
}

// A `-> { … }` body escapes the COND suppression as well, because the `{` is
// lexed as tLAMBEG and that lexer case still runs COND_PUSH(0) (parse.y v3_4_0
// 11226-11246). So a lambda written in a while/until/for header keeps its own
// inner `do…end`.
func TestDoBlockInsideLambdaBraceBodyInALoopHeader(t *testing.T) {
	for _, src := range []string{
		`while -> { y do end }.call; end`,
		`until -> { y do end }.call; end`,
		`for i in -> { y do end }.call; end`,
		`while -> { -> do y do end end }.call; end`,
	} {
		if _, err := Parse(src); err != nil {
			t.Errorf("Parse(%q): %v", src, err)
		}
	}
	prog, err := Parse(`while -> { y do end }.call; end`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	w, ok := prog.Body[0].(*ast.While)
	if !ok {
		t.Fatalf("top level = %#v, want a while", prog.Body[0])
	}
	call := w.Cond.(*ast.Call) // .call
	lam := call.Recv.(*ast.Call)
	y := lam.Block.Body[0].(*ast.Call)
	if y.Name != "y" || y.Block == nil {
		t.Fatalf("lambda body = %#v, want `y do end`", lam.Block.Body[0])
	}
}

// The suppressions must still refuse what MRI refuses. A `-> do … end` body is
// reached through CMDARG_PUSH(0) ALONE (parse.y v3_4_0 5182): nothing pushes COND
// there, so a `do` in it still closes an enclosing loop header and the source is a
// SyntaxError — as are the ordinary cases where a body resets neither stack.
// Every source below was verified to be rejected by ruby 4.0.5.
func TestDoStillRefusedWhereMRIRefusesIt(t *testing.T) {
	for _, src := range []string{
		// a `-> do … end` body does not clear COND
		`while -> do y do end end.call; end`,
		`until -> do y do end end.call; end`,
		`for i in -> do y do end end.call; end`,
		// `k_begin` and `if` push CMDARG 0 / nothing, never COND 0
		`while begin; y do end; end; end`,
		`while begin; y do end; end.call; end`,
		`while if true then y do end end; end`,
		// a call that already has a block cannot take a second one
		`foo :a, -> { y { } do end }`,
		`y { } do end`,
		`foo do end do end`,
		// an mlhs-shaped argument list inside the lambda body is still refused
		`foo :a, -> { bar(:b), baz do end }`,
		// and a plain syntax error stays one
		`foo :a, -> { y do end`,
		`foo :a, -> { y do end } end`,
		`x = = 1`,
	} {
		_, err := Parse(src)
		if err == nil {
			t.Errorf("Parse(%q) = nil error, want a parse error", src)
			continue
		}
		if !strings.Contains(err.Error(), "parse error") {
			t.Errorf("Parse(%q) error = %q, want a parse error", src, err)
		}
	}
}

// The suppression is RESTORED on the way out of a nested body, so a paren-less
// command call written INSIDE a lambda body still holds its own trailing `do`
// back: in `-> { bar :b, baz do end }` the block is bar's, not baz's — MRI's
// `block_call: command do_block` with the `keyword_do_block` that CMDARG_PUSH(1)
// in `command_args` (parse.y v3_4_0 4252-4265) produces. Confirmed by running it:
// `foo :a, -> { bar :b, baz do |w| p w end }.call` prints bar's yield.
func TestCommandArgSuppressionIsRestoredInsideTheBody(t *testing.T) {
	prog, err := Parse(`foo :a, -> { bar :b, baz do end }`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	lam := prog.Body[0].(*ast.Call).Args[1].(*ast.Call)
	bar, ok := lam.Block.Body[0].(*ast.Call)
	if !ok || bar.Name != "bar" {
		t.Fatalf("lambda body = %#v, want a call to bar", lam.Block.Body[0])
	}
	if bar.Block == nil {
		t.Fatalf("bar did not take the do…end block")
	}
	baz, ok := bar.Args[1].(*ast.Call)
	if !ok || baz.Name != "baz" {
		t.Fatalf("bar's second argument = %#v, want a call to baz", bar.Args[1])
	}
	if baz.Block != nil {
		t.Fatalf("baz took the do…end block; it belongs to bar")
	}
	// The same restore lets a `do` after a closed `(…)` argument reach the command.
	prog, err = Parse(`foo :a, (y do end), bar do end`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cmd := prog.Body[0].(*ast.Call)
	if cmd.Name != "foo" || cmd.Block == nil {
		t.Fatalf("foo = %#v, want the trailing do…end block", cmd)
	}
}
