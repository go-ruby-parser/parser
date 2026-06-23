package parser_test

import (
	"reflect"
	"testing"

	"github.com/go-ruby-parser/parser/ast"
)

// TestBeginAsCommandArg covers `begin…end` (and `case…end`) in paren-less
// command-argument position: the keyword block must become the call's argument,
// not trigger an "unexpected begin/case after statement" error.
func TestBeginAsCommandArg(t *testing.T) {
	tests := []struct {
		name string
		src  string
		call string
	}{
		{"p begin", "p begin; 1; end", "p"},
		{"foo begin", "foo begin; work; end", "foo"},
		{"puts begin", "puts begin; x; end", "puts"},
		{"begin with rescue", "foo begin; work; rescue; recover; end", "foo"},
		{"begin with ensure", "p begin; 1; ensure; 2; end", "p"},
		{"p case", "p case x; when 1; 2; end", "p"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arg := commandArg(t, tt.src, tt.call, 0)
			switch arg.(type) {
			case *ast.Begin, *ast.Case:
				// ok: value-producing keyword block became the argument.
			default:
				t.Fatalf("%q: arg = %T, want *ast.Begin or *ast.Case", tt.src, arg)
			}
		})
	}
}

// TestBeginCommandArgRescueClause checks the rescue clause inside a command-arg
// begin parses with the expected recovery body.
func TestBeginCommandArgRescueClause(t *testing.T) {
	src := "foo begin; work; rescue; recover; end"
	begin, ok := commandArg(t, src, "foo", 0).(*ast.Begin)
	if !ok {
		t.Fatalf("%q: arg = %T, want *ast.Begin", src, commandArg(t, src, "foo", 0))
	}
	if len(begin.Rescues) != 1 {
		t.Fatalf("%q: %d rescue clauses, want 1", src, len(begin.Rescues))
	}
	if len(begin.Body) != 1 {
		t.Errorf("%q: body has %d statements, want 1", src, len(begin.Body))
	}
	if len(begin.Rescues[0].Body) != 1 {
		t.Errorf("%q: rescue body has %d statements, want 1", src, len(begin.Rescues[0].Body))
	}
}

// TestBeginCommandArgNested checks a begin nested as an inner command-arg begin.
func TestBeginCommandArgNested(t *testing.T) {
	src := "p begin; q begin; 1; end; end"
	outer, ok := commandArg(t, src, "p", 0).(*ast.Begin)
	if !ok {
		t.Fatalf("%q: outer arg = %T, want *ast.Begin", src, commandArg(t, src, "p", 0))
	}
	if len(outer.Body) != 1 {
		t.Fatalf("%q: outer body has %d statements, want 1", src, len(outer.Body))
	}
	inner, ok := outer.Body[0].(*ast.Call)
	if !ok || inner.Name != "q" {
		t.Fatalf("%q: inner statement = %#v, want call to q", src, outer.Body[0])
	}
	if len(inner.Args) != 1 {
		t.Fatalf("%q: inner call has %d args, want 1", src, len(inner.Args))
	}
	if _, ok := inner.Args[0].(*ast.Begin); !ok {
		t.Errorf("%q: inner arg = %T, want *ast.Begin", src, inner.Args[0])
	}
}

// TestBeginCommandArgASTIdentical proves the begin node built in command-arg
// position is structurally identical to the same begin parsed in an already
// supported position (parenthesised argument). The fix only widens where the
// existing begin path is reached; it does not alter the produced AST.
func TestBeginCommandArgASTIdentical(t *testing.T) {
	cases := []struct{ cmd, paren string }{
		{"p begin; 1; end", "p(begin; 1; end)"},
		{"foo begin; work; rescue; recover; end", "foo(begin; work; rescue; recover; end)"},
		{"p case x; when 1; 2; end", "p(case x; when 1; 2; end)"},
	}
	for _, c := range cases {
		cmdArg := commandArg(t, c.cmd, callName(t, c.cmd), 0)
		parenArg := commandArg(t, c.paren, callName(t, c.paren), 0)
		if !reflect.DeepEqual(cmdArg, parenArg) {
			t.Errorf("AST differs between command-arg and paren form:\n cmd %q -> %#v\n paren %q -> %#v",
				c.cmd, cmdArg, c.paren, parenArg)
		}
	}
}

// callName returns the name of the top-level call in src.
func callName(t *testing.T, src string) string {
	t.Helper()
	call, ok := parseOne(t, src).(*ast.Call)
	if !ok {
		t.Fatalf("Parse(%q): top node = %T, want *ast.Call", src, parseOne(t, src))
	}
	return call.Name
}

// TestBeginRegressions guards the positions where begin/case already worked so
// the widened command-arg predicate does not change them.
func TestBeginRegressions(t *testing.T) {
	// begin as an assignment RHS.
	if _, ok := parseRHS(t, "x = begin; 5; end").(*ast.Begin); !ok {
		t.Errorf("x = begin: RHS = %T, want *ast.Begin", parseRHS(t, "x = begin; 5; end"))
	}
	// begin as a parenthesised argument.
	if _, ok := commandArg(t, "p(begin; 2; end)", "p", 0).(*ast.Begin); !ok {
		t.Errorf("p(begin): arg = %T, want *ast.Begin", commandArg(t, "p(begin; 2; end)", "p", 0))
	}
	// begin as a bare statement.
	if _, ok := parseOne(t, "begin; 1; end").(*ast.Begin); !ok {
		t.Errorf("bare begin: node = %T, want *ast.Begin", parseOne(t, "begin; 1; end"))
	}
	// `begin … end while cond` do-while modifier: the While loop wraps the begin.
	n := parseOne(t, "begin; work; end while cond")
	loop, ok := n.(*ast.While)
	if !ok {
		t.Fatalf("do-while: node = %T, want *ast.While", n)
	}
	if len(loop.Body) != 1 {
		t.Fatalf("do-while: loop body has %d statements, want 1", len(loop.Body))
	}
	if _, ok := loop.Body[0].(*ast.Begin); !ok {
		t.Errorf("do-while: loop body[0] = %T, want *ast.Begin", loop.Body[0])
	}
}
