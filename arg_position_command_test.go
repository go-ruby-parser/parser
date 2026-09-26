package parser

import (
	"strings"
	"testing"
)

// A PAREN-LESS COMMAND CALL — MRI's `command` (parse.y v3_4_0 3464-3536) — is
// legal only where the grammar reaches `command_call`. It reaches it from
// `stmt`/`expr` (3116, 3334, 3437), from `compstmt`, which every statement body
// and every parenthesised group is (3070), from `expr_value`, which the
// if/unless/while/until/for/case headers take, and from `call_args: command`
// (4223), which `paren_args` (4171) and an index bracket
// (`primary_value '[' opt_call_args rbracket`) go through.
//
// It does NOT reach it through `arg_value: arg` (4133). An array LITERAL's
// contents are `aref_args: args` → `args: arg_value` (4427, 4140, 4314); a hash
// entry is `assoc: arg_value tASSOC arg_value | tLABEL arg_value | tDSTAR
// arg_value` (6789); a splat is `arg_splat: tSTAR arg_value`; a block-pass is
// `block_arg: tAMPER arg_value`; every argument AFTER the first comes through
// `args ',' arg_value`; a `when` candidate, a `rescue` class, a parameter
// default, an operator operand and a ternary arm are all `arg` too. `arg` has no
// `command` alternative, so a paren-less command call is a SyntaxError in every
// one of those places — which is why `[foo bar]` and `{k: foo bar}` are
// SyntaxErrors while `(foo bar)`, `f(foo bar)` and `a[foo bar]` are legal.
//
// Every source in both tables was judged on ruby 4.0.5 by BOTH its parsers,
// `ruby -c` and `ruby --parser=parse.y -c`, which agree on all of them. The one
// shape where they do NOT agree is `a[foo bar do end]` — prism accepts it, parse.y
// refuses it — so it is asserted in neither table; this parser accepts it, which
// is the prism answer.
//
// This is the one direction in which a parser change can break real code, so the
// second table is the load-bearing one: it is the set of shapes MRI admits that
// must keep parsing.
func TestParenLessCommandCallRefusedInAnArgPosition(t *testing.T) {
	for _, src := range []string{
		`+foo bar`,
		`-foo bar`,
		`1 + foo bar`,
		`1 .. foo bar`,
		`[!foo bar]`,
		`[$x = foo bar]`,
		`[*a, foo bar]`,
		`[*foo bar]`,
		`[1, foo bar]`,
		`[@x = foo bar]`,
		`[A = foo bar]`,
		`[Foo 1]`,
		`[Foo :a]`,
		`[Foo.bar baz]`,
		`[[1][0] baz]`,
		`[[foo bar]]`,
		`[a += foo bar]`,
		`[a = foo bar]`,
		`[a, *foo bar]`,
		`[a, foo bar do end]`,
		`[a.b = foo bar]`,
		`[a.b.c d]`,
		`[a::B = foo bar]`,
		`[a[0] = foo bar]`,
		`[foo 1]`,
		`[foo :sym]`,
		`[foo bar and baz]`,
		`[foo bar do end]`,
		`[foo bar rescue 1]`,
		`[foo bar { }]`,
		`[foo bar, 1]`,
		`[foo bar, baz qux]`,
		`[foo bar]`,
		`[foo"bar"]`,
		`[foo(bar) baz]`,
		`[foo.bar baz]`,
		`[foo.y"z"]`,
		`[not foo bar]`,
		`[obj::Const 1]`,
		`[obj::Meth 1]`,
		`[super 1]`,
		`[yield 1]`,
		`a ? 1 : foo bar`,
		`a ? foo bar : 1`,
		`a[1, foo bar]`,
		`case x; when foo bar then 1; end`,
		`def m(a = 1, b = foo bar); end`,
		`def m(a = foo bar); end`,
		`def m(a: foo bar); end`,
		`def m; [yield 1]; end`,
		`def m; return 1, foo bar; end`,
		`defined? foo bar`,
		`f(*foo bar)`,
		`f(1, foo bar)`,
		`f(a, *foo bar)`,
		`f(a, b, foo bar)`,
		`f(k: foo bar)`,
		`foo = 1; [foo "x"]`,
		`foo a, bar baz`,
		`foo(&bar baz)`,
		`foo(**bar baz)`,
		`p [foo bar]`,
		`p({k: foo bar})`,
		`until [foo bar]; end`,
		`while [foo bar]; end`,
		`while begin; y do end; end; end`,
		`while true; break 1, foo bar; end`,
		`while x = y do end; end`,
		`while y do end; end`,
		`x = *foo bar`,
		`x = 1, foo bar`,
		`x = [foo bar do end]`,
		`x = [foo bar]`,
		`x, y = *foo bar`,
		`x, y = 1, foo bar`,
		`{**foo bar}`,
		`{1 => foo bar}`,
		`{foo bar => 1}`,
		`{k: a = foo bar}`,
		`{k: foo bar rescue 1}`,
		`{k: foo bar}`,
		`{k: foo.bar baz}`,
		`~foo bar`,
	} {
		_, err := Parse(src)
		if err == nil {
			t.Errorf("Parse(%q) = nil error, want a parse error (MRI 4.0.5 refuses it)", src)
			continue
		}
		if !strings.Contains(err.Error(), "parse error") {
			t.Errorf("Parse(%q) error = %q, want a parse error", src, err)
		}
	}
}

// The other direction, and the gate on the change: every shape here is one MRI
// ACCEPTS, and refusing any of them would be a regression against real Ruby. The
// contrasts that name the rule sit next to each other on purpose: `(foo bar)` vs
// `[foo bar]`, `a[foo bar]` vs `[foo bar]`, `f(foo bar, 1)` vs `f(1, foo bar)`,
// `[foo(bar baz)]` vs `[foo bar]`, `[begin; foo bar; end]` vs `[foo bar]`.
func TestParenLessCommandCallStillAcceptedWhereMRIAdmitsIt(t *testing.T) {
	for _, src := range []string{
		`!foo bar`,
		`(foo bar do end)`,
		`(foo bar)`,
		`(foo bar; qux)`,
		`[(foo bar)]`,
		`[(foo bar; baz)]`,
		`[*(foo bar)]`,
		`[**a]`,
		`[1 => a]`,
		`[1].each do foo bar end`,
		`[[1, foo(bar baz)]]`,
		`[a ? foo(b c) : 1]`,
		`[a[foo bar]]`,
		`[begin; foo bar; end]`,
		`[def m; foo bar; end]`,
		`[defined?(break 1)]`,
		`[defined?(foo bar)]`,
		`[foo(bar baz), 1]`,
		`[foo(bar baz)]`,
		`[if foo bar then 1 end]`,
		`[lambda { foo bar }]`,
		`[proc do foo bar end]`,
		`[while foo bar; end]`,
		`a = [1][foo bar]`,
		`a&.b foo bar`,
		`a.b = foo bar`,
		`a.b foo bar`,
		`a[1] = foo bar`,
		`a[foo bar, 1]`,
		`a[foo bar]`,
		`a[foo bar] += 1`,
		`a[foo bar] = 1`,
		`begin; foo bar; end`,
		`case foo bar` + "\n" + `when 1` + "\n" + `end`,
		`def m; defined? yield; end`,
		`def m; return foo bar, 1; end`,
		`def m; return foo bar; end`,
		`def m; yield foo bar; end`,
		`def m; yield(foo bar); end`,
		`defined? @x`,
		`defined? Foo::Bar`,
		`defined? foo.bar`,
		`defined? super`,
		`defined? x`,
		`defined? x = 1`,
		`defined?(foo bar)`,
		`f(foo bar)`,
		`f(foo bar) { }`,
		`f(foo bar) { } .z`,
		`f(foo bar, 1)`,
		`foo :a, (y do end)`,
		`foo :a, [y do end]`,
		`foo :a, {k: y do end}`,
		`foo = 1; [foo -1]`,
		`foo bar`,
		`foo bar ? 1 : 2`,
		`foo bar and baz`,
		`foo bar baz`,
		`foo bar baz, 1`,
		`foo bar rescue baz`,
		`foo bar, baz`,
		`for i in foo bar; end`,
		`h.each { |k, v| p k }`,
		`if foo bar; end`,
		`not foo bar`,
		`p defined? x`,
		`p foo bar`,
		`p(foo bar)`,
		`puts [1].map { |x| foo bar }`,
		`return foo bar`,
		`super foo bar`,
		`super(foo bar)`,
		`unless foo bar; end`,
		`until [y do end]; end`,
		`until foo bar; end`,
		`while (y do end); end`,
		`while [y do end]; end`,
		`while a[y do end]; end`,
		`while foo bar; end`,
		`while true; break foo bar; end`,
		`while true; next foo bar; end`,
		`while {k: y do end}; end`,
		`x += foo bar`,
		`x = (y do end)`,
		`x = 1; x[foo bar]`,
		`x = [foo(bar baz)]`,
		`x = defined? y`,
		`x = foo bar`,
		`x = foo bar, 1`,
		`x = foo bar, baz`,
		`x ||= foo bar`,
		`x, y = foo bar`,
		`{k: (foo bar)}`,
		`{k: -> { foo bar }}`,
		`{k: a[foo bar]}`,
		`{k: defined?(foo bar)}`,
		`{k: foo(bar baz)}`,
	} {
		if _, err := Parse(src); err != nil {
			t.Errorf("Parse(%q) = %v, want no error (MRI 4.0.5 accepts it)", src, err)
		}
	}
}
