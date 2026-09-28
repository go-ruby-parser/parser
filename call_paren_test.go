package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// lastNode parses src and returns its last top-level node, so a case may set up
// a local variable first (`x = 1; x()`).
func lastNode(t *testing.T, src string) ast.Node {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	if len(prog.Body) == 0 {
		t.Fatalf("Parse(%q): empty program", src)
	}
	return prog.Body[len(prog.Body)-1]
}

// TestCallParen is the bit itself: whether the call site was written with a
// parenthesised argument list. It is what separates MRI's FCALL from its
// VCALL, built by different productions — `fcall: operation` into
// `method_call: fcall paren_args` (parse.y-ruby_4_0:3572-3577, 5242-5248)
// versus a bare name reaching `gettable` (parse.y-ruby_4_0:13086) — and
// observable as NoMethodError versus NameError.
func TestCallParen(t *testing.T) {
	cases := []struct {
		src       string
		wantName  string
		wantParen bool
		wantArgs  int
	}{
		// The pair from issue #41. Same name, same (empty) arguments.
		{"foo", "foo", false, 0},
		{"foo()", "foo", true, 0},
		{"foo(1)", "foo", true, 1},
		// `foo ()` is not a parenthesised argument list: the space makes the
		// `()` a grouped expression passed as a command argument, so Paren is
		// false while an argument appears. It is an FCALL in MRI, and it stays
		// one for a consumer, because a VCALL also requires no arguments.
		{"foo ()", "foo", false, 1},
		{"puts 1", "puts", false, 1},
		// With a receiver.
		{"obj.foo", "foo", false, 0},
		{"obj.foo()", "foo", true, 0},
		{"obj.foo(1)", "foo", true, 1},
		{"obj.foo bar", "foo", false, 1},
		// Safe navigation, both spellings.
		{"obj&.foo", "foo", false, 0},
		{"obj&.foo()", "foo", true, 0},
		// The `.()` shorthand is parenthesised by construction.
		{"foo.()", "call", true, 0},
		{"foo.(1)", "call", true, 1},
		// Scope resolution, both spellings, lowercase and capitalised name.
		{"Foo::bar", "bar", false, 0},
		{"Foo::bar(1)", "bar", true, 1},
		{"Foo::Bar(1)", "Bar", true, 1},
		// A capitalised method call.
		{`Integer("42")`, "Integer", true, 1},
		// Brackets are not parentheses: an index is `recv.[](…)` with no
		// written parens (MRI: `primary_value '[' opt_call_args rbracket`,
		// parse.y-ruby_4_0:5292-5297).
		{"a[0]", "[]", false, 1},
		// A name that is also a local variable: with parens it is a call, and
		// the bare form is not a Call at all (covered below).
		{"x = 1; x()", "x", true, 0},
		// tFID: `foo!` and `foo?` carry no parentheses, so Paren is false —
		// MRI nonetheless makes them FCALLs (`primary: tFID`,
		// parse.y-ruby_4_0:4370-4374), which is why a consumer deciding VCALL
		// must also look at the name. Pinned here so that stays visible.
		{"foo!", "foo!", false, 0},
		{"foo?", "foo?", false, 0},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			node := lastNode(t, tc.src)
			call, ok := node.(*ast.Call)
			if !ok {
				t.Fatalf("Parse(%q): node is %T, want *ast.Call", tc.src, node)
			}
			if call.Name != tc.wantName {
				t.Fatalf("Parse(%q): Name = %q, want %q", tc.src, call.Name, tc.wantName)
			}
			if call.Paren != tc.wantParen {
				t.Errorf("Parse(%q): Paren = %v, want %v", tc.src, call.Paren, tc.wantParen)
			}
			if len(call.Args) != tc.wantArgs {
				t.Errorf("Parse(%q): %d args, want %d", tc.src, len(call.Args), tc.wantArgs)
			}
		})
	}
}

// TestCallParenBareLocalIsNotACall is the other half of the local-variable
// case: a bare name that IS a visible local reads as a variable, so there is no
// Call to ask Paren of. `x = 1; x()` above is the same name with parentheses.
func TestCallParenBareLocalIsNotACall(t *testing.T) {
	node := lastNode(t, "x = 1; x")
	if _, ok := node.(*ast.VarRef); !ok {
		t.Fatalf(`Parse("x = 1; x"): node is %T, want *ast.VarRef`, node)
	}
}

// TestCallParenBlockIsNotAParen pins the remaining input to a consumer's VCALL
// test: a literal block makes an FCALL in MRI (`primary: fcall brace_block`,
// parse.y-ruby_4_0:4458-4463) although no parentheses were written. Paren is
// false and Block is set, so the two are distinguishable.
func TestCallParenBlockIsNotAParen(t *testing.T) {
	for _, src := range []string{"foo {}", "foo do end"} {
		node := lastNode(t, src)
		call, ok := node.(*ast.Call)
		if !ok {
			t.Fatalf("Parse(%q): node is %T, want *ast.Call", src, node)
		}
		if call.Paren {
			t.Errorf("Parse(%q): Paren = true, want false", src)
		}
		if call.Block == nil {
			t.Errorf("Parse(%q): Block = nil, want a block", src)
		}
	}
}

// TestCallParenVCallRule exercises the rule the documentation on Call.Paren
// gives a consumer, over the whole edge set at once: a VCALL is a bare plain
// identifier with no receiver, no parentheses, no arguments and no block. The
// expectations are MRI 4.0.5's, which raises NameError for a VCALL and
// NoMethodError for an FCALL.
func TestCallParenVCallRule(t *testing.T) {
	isVCall := func(n ast.Node) bool {
		c, ok := n.(*ast.Call)
		if !ok || c.Recv != nil || c.Paren || len(c.Args) != 0 || c.Block != nil {
			return false
		}
		last := c.Name[len(c.Name)-1]
		return last != '!' && last != '?' && last != '='
	}
	cases := []struct {
		src  string
		want bool // true == VCALL (MRI raises NameError)
	}{
		{"nope", true},
		{"nope()", false},
		{"nope(1)", false},
		{"nope!", false},
		{"nope?", false},
		{"nope {}", false},
		{"nope do end", false},
		{"nope 1", false},
		{"self.nope", false},
		{"obj.nope", false},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			if got := isVCall(lastNode(t, tc.src)); got != tc.want {
				t.Errorf("Parse(%q): VCALL = %v, want %v", tc.src, got, tc.want)
			}
		})
	}
}
