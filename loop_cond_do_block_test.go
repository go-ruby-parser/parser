package parser

import (
	"strings"
	"testing"

	"github.com/go-ruby-parser/parser/ast"
)

// A `do…end` inside a LOOP CONDITION attaches to the call it follows as soon as
// the condition descends into a grouping construct. MRI pushes COND 1 only
// around a loop header — `expr_value_do: {COND_PUSH(1);} expr_value do
// {COND_POP();}` (parse.y v3_4_0 3430), reached from `k_while`, `k_until` and
// `k_for` (4505, 4514, 4557) and from nowhere else, which is why
// `if (y do end); end` never needed a fix — and clears it again with
// COND_PUSH(0) at exactly five places:
//
//	the lexer's `(`   11193   \
//	the lexer's `[`   11221    >  all three: `++paren_nest; COND_PUSH(0); CMDARG_PUSH(0)`
//	the lexer's `{`   11245   /
//	tSTRING_DBEG      6182       the `#{` of an interpolation (popped at 6198)
//	local_push        14937      a def/class/module/singleton-class body (popped at 14978)
//
// With COND back at 0 the lexer answers plain `keyword_do` instead of
// `keyword_do_cond` (10519-10527), so the `do` opens a block.
//
// Every source below was measured to parse under ruby 4.0.5 with BOTH its
// parsers (`ruby -c` and `ruby --parser=parse.y -c`), except the one row where
// they disagree, which is called out where it appears.
func TestDoBlockInsideLoopConditionGrouping(t *testing.T) {
	for _, src := range []string{
		// --- the eight shapes of the report, one per grouping construct ---
		`while (y do end); end`,
		`while [y do end]; end`,
		`while "#{y do end}"; end`,
		`while {a: y do end}[:a]; end`,
		`while (y do end) && true; end`,
		`until (y do end); end`,
		`for i in (y do end); end`,
		`for i in [y do end]; end`,
		// --- and the two that already passed, kept as the diagnostic key: no
		// COND_PUSH(1) happens for `if`/`unless`, so nothing has to be cleared ---
		`if (y do end); end`,
		`unless (y do end); end`,

		// the lexer's `(` (11193): a parenthesised group, at any depth
		`while ((y do end)); end`,
		`while (y do end; z do end); end`,
		`while (y do |a| a end); end`,
		`while (y(1) do end); end`,
		`while (y.z do end); end`,
		`while (y :a do end); end`,
		`while (foo bar do end); end`,
		`while (y do z do end end); end`,
		`while (y do end rescue z); end`,
		`while (y do end).foo; end`,
		`while (y do end)[0]; end`,
		`while (y do end).nil?; end`,
		`while (y do end) == 1; end`,
		`while y(z do end); end`,
		`while y.z(w do end); end`,
		`until (y do end) && true; end`,
		`for i in (y do end) && true; end`,

		// the lexer's `[` (11221): an array literal AND an index (tAREF)
		`while [y do end]; end`,
		`while [[y do end]]; end`,
		`while [(y do end)]; end`,
		`while ([y do end]); end`,
		`while [y do end, z do end]; end`,
		`while [y do |a| a end]; end`,
		`while [y do z do end end]; end`,
		`while [y do end].first; end`,
		`while [*[y do end]]; end`,
		`while a[y do end]; end`,
		`while a[y do end].b; end`,
		`while y[(z do end)]; end`,
		`until [y do end]; end`,
		`for i, j in [y do end]; end`,
		`for i in [1, y do end]; end`,

		// the lexer's `{` (11245): a hash literal, as a condition or a receiver
		`while {a: y do end}; end`,
		`while ({a: y do end})[:a]; end`,
		`while {a: y do end, b: z do end}[:a]; end`,
		`while {**{a: y do end}}[:a]; end`,
		`until {a: y do end}[:a]; end`,
		`for i in {a: y do end}[:a]; end`,

		// tSTRING_DBEG (6182): `#{…}` in every literal that carries one
		`while "#{y do end}"; end`,
		`while "#{y do end}#{z do end}"; end`,
		`while "#{y do end}".size; end`,
		`while "#{(y do end)}"; end`,
		`while "#{[y do end]}"; end`,
		`while "#{ [ {a: y do end} ] }"; end`,
		`while "#{y do |a| a end}"; end`,
		`while :"#{y do end}"; end`,
		`while /#{y do end}/; end`,
		`while %W[#{y do end}]; end`,
		`until "#{y do end}"; end`,
		`for i in "#{y do end}"; end`,

		// local_push (14937): a def/class/module/singleton-class body. This family
		// is NOT one of the five grouping constructs of the report — it was found by
		// reading every COND_PUSH(0) site in parse.y, and MRI accepts it too.
		`while def f; y do end; end; end`,
		`while def self.f; y do end; end; end`,
		`while def (obj).f; y do end; end; end`,
		`while (def f; y do end; end); end`,
		`while class C; y do end; end; end`,
		`while class << self; y do end; end; end`,
		// ruby 4.0.5's two parsers DISAGREE on this one and only this one: parse.y
		// accepts it (the NEW_SCOPE macro local_pushes, 1709) while PRISM reports
		// "unexpected 'do'". `class` in the same position is accepted by both, so
		// the asymmetry is on PRISM's side; parse.y is the grammar this parser
		// follows.
		`while module M; y do end; end; end`,

		// a `{…}` or `do…end` BLOCK body written in the condition is a fresh COND
		// context of its own, so the nesting composes
		`while [1].each { |x| y do end }; end`,
		`while -> { y do end }.call; end`,
		`while ->(){ y do end }.call; end`,
		`while -> { y do end }; end`,
		`while (-> do y do end end); end`,
		`while -> (a = (y do end)) { a }.call; end`,

		// a grouping construct nested inside a body that clears CMDARG only still
		// clears COND: the paren/bracket/brace is what does it
		`while (begin; y do end; end); end`,
		`while begin (y do end) end; end`,
		`while begin; (y do end); end; end`,
		`while (if true then y do end end); end`,
		`while if true then (y do end) end; end`,
		`while (case 1 when 1 then y do end end); end`,
		`while case 1 when 1 then (y do end) end; end`,
		`while (begin; y do end; end) && true; end`,
		`while [begin; y do end; end]; end`,
		`while {a: begin; y do end; end}[:a]; end`,
		`while "#{begin; y do end; end}"; end`,

		// an operand of a boolean or ternary operator is NOT itself a clear site —
		// the parentheses in these are what clear COND (see the refusal table for
		// the witness), but they must of course keep parsing
		`while true && (y do end); end`,
		`while (true) && (y do end); end`,
		`while (y do end) || true; end`,
		`while false || (y do end); end`,
		`while true and (y do end); end`,
		`while !(y do end); end`,
		`while ((y do end) && (z do end)); end`,
		`while (y do end) && (z do end); end`,
		`while [y do end] && [z do end]; end`,
		`while ((y do end)) && ((z do end)); end`,
		`while true ? (y do end) : 1; end`,
		`while (y do end) ? 1 : 2; end`,
		`while ([{a: y do end}]); end`,

		// the loop's own `do` is still available after the condition closes, and the
		// suppression is restored for the loop BODY and for whatever follows
		`while (y do end) do end`,
		`while [y do end] do z end`,
		`until (y do end) do end`,
		`for i in (y do end) do end`,
		`while (y do end); z do end; end`,
		`until (y do end); z do end; end`,
		`for i in (y do end); z do end; end`,
		`while (y do end); end; z do end`,
		`while (y do end); while (z do end); end; end`,
		`while [y do end]; while [z do end]; end; end`,
		`a = while (y do end); end`,

		// a modifier `while`/`until` never pushes COND at all (parse.y has no
		// COND_PUSH in the `stmt modifier_while expr_value` rules), so both the
		// grouped and the bare form parse
		`x = 1 while (y do end)`,
		`x = 1 while y do end`,
		`x = 1 until (y do end)`,
		`begin; end while (y do end)`,
	} {
		if _, err := Parse(src); err != nil {
			t.Errorf("Parse(%q): %v", src, err)
		}
	}
}

// The COND suppression must still refuse everything MRI refuses. This is the
// control that a blanket clear would break: `begin`, `if`, `unless`, `case`, a
// `do…end` body and a `-> do … end` body reset CMDARG and NOT COND, so a `do`
// written directly in them still closes the loop header. Every source below was
// measured to be rejected by ruby 4.0.5 under both its parsers.
func TestDoBlockInLoopConditionStillRefusedWhereMRIRefusesIt(t *testing.T) {
	for _, src := range []string{
		// THE control of the report: `k_begin` pushes CMDARG 0 (parse.y v3_4_0
		// 4392) and nothing pushes COND, so this is a SyntaxError in MRI. It is why
		// parseStatements must not clear noDo.
		`while begin; y do end; end; end`,
		`until begin; y do end; end; end`,
		`for i in begin; y do end; end; end`,
		`while true && begin; y do end; end; end`,
		// `if`/`unless`/`case` bodies reset CMDARG only, exactly like `begin`
		`while if true then y do end end; end`,
		`while unless true then y do end end; end`,
		`while case 1 when 1 then y do end end; end`,
		// a `-> do … end` body is reached through CMDARG_PUSH(0) alone (5182); there
		// is no `{` there, so nothing pushes COND
		`while -> do y do end end.call; end`,
		// an inner loop re-pushes COND 1 and pops it at its own header terminator,
		// so the inner BODY is back under the outer suppression
		`while while true; y do end; end; end`,
		// an operand of `&&` is NOT a COND_PUSH(0) site — this is the witness. If it
		// were one, the `do` here would open a block for `y` and the source would
		// parse; instead the `do` closes the loop header and the trailing `&& true`
		// has nothing to attach to.
		`while y do end && true; end`,
		`while y do end.foo; end`,
		`while true && y do end; end`,
		// the bare form: the `do` is the loop's, so the following `end` is spare
		`while y do end; end`,
		`while [1].map do end; end`,
		// a modifier `if` cannot follow a `while` header
		`while (y do end) if true; end`,
		// #35: clearing COND at `[` made the eighth member of the family REACHABLE,
		// and it then turned out to be refused for a second, older reason — an array
		// literal's contents are `arg_value`, which admits no paren-less command call
		// at all. Both of these are SyntaxErrors in MRI and both are accepted by
		// v0.4.0; see arg_position_command_test.go for the whole boundary.
		`while [foo bar do end]; end`,
		`while [foo bar]; end`,
		`until [foo bar]; end`,
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

// The CMDARG half that #32 fixed must stay fixed: clearing COND at the grouping
// constructs is a separate bit-stack and must not disturb it. These are accepted
// by ruby 4.0.5 and by this parser both before and after the COND change.
func TestCommandArgumentDoBlockStillAccepted(t *testing.T) {
	for _, src := range []string{
		`foo :a, (y do end)`,
		`x = (y do end)`,
		`foo :a, [y do end]`,
		`foo :a, {k: y do end}`,
		`foo :a, "#{y do end}"`,
		`foo :a, begin; y do end; end`,
		`foo :a, bar(y do end)`,
		`foo :a, -> { y do end }`,
		`foo :a, if c; y do end; end`,
		`foo bar do end`,
		`x = [y do end]`,
		`x = {k: y do end}`,
		`x = "#{y do end}"`,
		`def f; y do end; end`,
		`class C; y do end; end`,
		// the loop body was never suppressed and must stay that way
		`while true; y do end; end`,
		`until true; y do end; end`,
		`for i in [1]; y do end; end`,
		`for i in [1] do y do end end`,
		// noBraceBlock is the paren_nest half of the same lexer cases and must keep
		// its own meaning
		`-> a=a() { a }`,
		`-> x = ([1].map { |v| v }) { x }`,
	} {
		if _, err := Parse(src); err != nil {
			t.Errorf("Parse(%q): %v", src, err)
		}
	}
}

// Parsing is not enough: the block must land on the receiver MRI gives it to. In
// `while (bar :b, baz do end); end` the paren clears COND, then `bar`'s
// paren-less argument list pushes CMDARG 1 again, so the `do` is
// `keyword_do_block` and belongs to BAR, not to the `baz` it directly follows.
// Measured by running the shape under ruby 4.0.5 with two distinguishable
// receivers, each recording whether it was handed a block: bar:BLOCK,
// baz:noblock.
func TestDoBlockInLoopConditionBindsToTheOuterCommand(t *testing.T) {
	prog, err := Parse(`while (bar :b, baz do end); end`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	w, ok := prog.Body[0].(*ast.While)
	if !ok {
		t.Fatalf("top level = %#v, want a while", prog.Body[0])
	}
	bar, ok := w.Cond.(*ast.Call)
	if !ok || bar.Name != "bar" {
		t.Fatalf("condition = %#v, want a call to bar", w.Cond)
	}
	if bar.Block == nil {
		t.Fatalf("bar has no block; the do…end is bar's")
	}
	if len(bar.Args) != 2 {
		t.Fatalf("bar args = %d, want 2", len(bar.Args))
	}
	baz, ok := bar.Args[1].(*ast.Call)
	if !ok || baz.Name != "baz" {
		t.Fatalf("bar's second argument = %#v, want a call to baz", bar.Args[1])
	}
	if baz.Block != nil {
		t.Fatalf("baz took the do…end block; it belongs to bar")
	}
}

// And the loop's own `do` is still the loop's. `while (y do end) do end` has two
// `do`s: the first opens y's block inside the cleared condition, the second
// closes the header. Measured under ruby 4.0.5: y is handed a block, and the
// loop runs.
func TestLoopKeepsItsOwnDoAfterAGroupedCondition(t *testing.T) {
	prog, err := Parse(`while (y do end) do z end`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	w, ok := prog.Body[0].(*ast.While)
	if !ok {
		t.Fatalf("top level = %#v, want a while", prog.Body[0])
	}
	y, ok := w.Cond.(*ast.Call)
	if !ok || y.Name != "y" || y.Block == nil {
		t.Fatalf("condition = %#v, want `y do end` with its block", w.Cond)
	}
	if len(w.Body) != 1 {
		t.Fatalf("loop body = %d statements, want 1", len(w.Body))
	}
	z, ok := w.Body[0].(*ast.Call)
	if !ok || z.Name != "z" {
		t.Fatalf("loop body = %#v, want a call to z", w.Body[0])
	}
}

// The `def`/`class`/`module` family goes through pushScope, which is MRI's
// local_push: it must RESTORE the flags on the way out, not leave them cleared,
// or a `do` after the body would stop closing the loop header.
func TestHardScopeRestoresTheLoopSuppressionOnExit(t *testing.T) {
	prog, err := Parse(`while def f; y do end; end do z end`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	w, ok := prog.Body[0].(*ast.While)
	if !ok {
		t.Fatalf("top level = %#v, want a while", prog.Body[0])
	}
	def, ok := w.Cond.(*ast.MethodDef)
	if !ok || def.Name != "f" {
		t.Fatalf("condition = %#v, want a def of f", w.Cond)
	}
	y, ok := def.Body[0].(*ast.Call)
	if !ok || y.Name != "y" || y.Block == nil {
		t.Fatalf("def body = %#v, want `y do end` with its block", def.Body[0])
	}
	if len(w.Body) != 1 {
		t.Fatalf("loop body = %d statements, want 1", len(w.Body))
	}
}
