package parser_test

import (
	"fmt"
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// TestDefinedTakesAnExpr covers MRI's dedicated `defined?` productions
// (parse.y v3_4_0): `keyword_defined '\n'? '(' begin_defined expr rparen` is an
// alternative of `primary`, and `keyword_defined '\n'? begin_defined arg` an
// alternative of `arg`. The parenthesised form takes a full `expr`, so the
// low-precedence keyword operators are allowed inside it — unlike an ordinary
// call argument.
func TestDefinedTakesAnExpr(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		`defined?($zz and true)`,
		`defined?($zz or true)`,
		`defined?(not $zz)`,
		`defined?(a and b or c)`,
		"defined?(\n  $zz and true\n)",
	} {
		n := parseOne(t, src)
		call, ok := n.(*ast.Call)
		if !ok {
			t.Fatalf("Parse(%q): node = %T, want *ast.Call", src, n)
		}
		if call.Name != "defined?" || len(call.Args) != 1 {
			t.Errorf("Parse(%q): call %q with %d args, want defined? with 1", src, call.Name, len(call.Args))
		}
	}
}

// TestDefinedParenMustHug pins the other side of the same pair of rules: only a
// hugging parenthesis selects the `primary` production. With a space the
// parenthesis opens the `arg` instead, so MRI answers "expression" to
// `defined? (1) && nil` but nil to `defined?(1) && nil`.
func TestDefinedParenMustHug(t *testing.T) {
	t.Parallel()
	hug := parseOne(t, `defined?(1) && nil`)
	if _, ok := hug.(*ast.BinaryExpr); !ok {
		t.Errorf(`defined?(1) && nil: node = %T, want *ast.BinaryExpr`, hug)
	}
	spaced := parseOne(t, `defined? (1) && nil`)
	call, ok := spaced.(*ast.Call)
	if !ok {
		t.Fatalf(`defined? (1) && nil: node = %T, want *ast.Call`, spaced)
	}
	if call.Name != "defined?" {
		t.Errorf(`defined? (1) && nil: call %q, want defined?`, call.Name)
	}
	if _, ok := call.Args[0].(*ast.BinaryExpr); !ok {
		t.Errorf(`defined? (1) && nil: argument = %T, want *ast.BinaryExpr`, call.Args[0])
	}
	if _, err := parser.Parse(`defined?($zz; $x)`); err == nil {
		t.Error(`defined?($zz; $x): want a parse error (MRI takes one expr, not a sequence)`)
	}
}

// TestJumpKeywordsArePrimaries covers the bare `break`/`next`/`retry`/`return`
// alternatives of MRI's `primary` (parse.y v3_4_0: `k_return` at 4438,
// `keyword_break`/`keyword_next`/`keyword_redo`/`keyword_retry` at 4682-4697),
// and the `command` forms that carry arguments. `defined?(break)` is what
// ruby/spec uses to reach them.
func TestJumpKeywordsArePrimaries(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		src  string
		want any
	}{
		{`defined?(break)`, (*ast.Break)(nil)},
		{`defined?(next)`, (*ast.Next)(nil)},
		{`defined?(retry)`, (*ast.Retry)(nil)},
		{`defined?(return)`, (*ast.Return)(nil)},
		{`defined?(break 1)`, (*ast.Break)(nil)},
		{`defined?(next 1)`, (*ast.Next)(nil)},
		{`defined?(return 1)`, (*ast.Return)(nil)},
	} {
		n := parseOne(t, tc.src)
		call, ok := n.(*ast.Call)
		if !ok {
			t.Fatalf("Parse(%q): node = %T, want *ast.Call", tc.src, n)
		}
		if got, want := nodeType(call.Args[0]), nodeType(tc.want); got != want {
			t.Errorf("Parse(%q): argument = %s, want %s", tc.src, got, want)
		}
	}
}

func nodeType(n any) string { return fmt.Sprintf("%T", n) }
