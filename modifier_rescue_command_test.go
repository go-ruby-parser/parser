package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser/ast"
)

// A modifier `rescue` after a paren-less command call binds to the whole call,
// not to the last argument. MRI parses `raise "x" rescue 42` as
// `(raise "x") rescue 42` (Ripper: rescue_mod(command(raise, ["x"]), 42)), so the
// AST must be Begin{ Body:[ Call{raise, ["x"]} ], Rescues:[{ Body:[42] }] } — the
// rescue wrapping the Call, never Call{raise, [Begin{"x" rescue 42}]}.

// beginWrapping asserts n is a *ast.Begin with a single-clause rescue and returns
// its sole body node plus the sole fallback node, so a test can inspect what the
// rescue wraps and what it falls back to.
func beginWrapping(t *testing.T, src string, n ast.Node) (body ast.Node, fallback ast.Node) {
	t.Helper()
	b, ok := n.(*ast.Begin)
	if !ok {
		t.Fatalf("%q: node = %T, want *ast.Begin", src, n)
	}
	if len(b.Body) != 1 {
		t.Fatalf("%q: Begin.Body has %d nodes, want 1", src, len(b.Body))
	}
	if len(b.Rescues) != 1 {
		t.Fatalf("%q: Begin.Rescues has %d clauses, want 1", src, len(b.Rescues))
	}
	if len(b.Rescues[0].Body) != 1 {
		t.Fatalf("%q: rescue body has %d nodes, want 1", src, len(b.Rescues[0].Body))
	}
	if len(b.Rescues[0].Classes) != 0 {
		t.Fatalf("%q: rescue has %d classes, want 0 (bare StandardError)", src, len(b.Rescues[0].Classes))
	}
	return b.Body[0], b.Rescues[0].Body[0]
}

// wantCall asserts n is a *ast.Call with the given name and argument count and
// returns it.
func wantCall(t *testing.T, src string, n ast.Node, name string, nargs int) *ast.Call {
	t.Helper()
	c, ok := n.(*ast.Call)
	if !ok {
		t.Fatalf("%q: node = %T, want *ast.Call %q", src, n, name)
	}
	if c.Name != name {
		t.Fatalf("%q: Call.Name = %q, want %q", src, c.Name, name)
	}
	if len(c.Args) != nargs {
		t.Fatalf("%q: Call %q has %d args, want %d", src, name, len(c.Args), nargs)
	}
	return c
}

// TestModifierRescueWrapsCommandCall is the core fix: the rescue must wrap the
// whole command call, and each argument must remain a plain literal (not a Begin).
func TestModifierRescueWrapsCommandCall(t *testing.T) {
	t.Run("single arg", func(t *testing.T) {
		const src = `raise "x" rescue 42`
		body, fallback := beginWrapping(t, src, parseOne(t, src))
		call := wantCall(t, src, body, "raise", 1)
		if _, ok := call.Args[0].(*ast.StringLit); !ok {
			t.Fatalf("%q: arg 0 = %T, want *ast.StringLit (rescue must not wrap the arg)", src, call.Args[0])
		}
		if i, ok := fallback.(*ast.IntLit); !ok || i.Value != 42 {
			t.Fatalf("%q: fallback = %#v, want IntLit 42", src, fallback)
		}
	})

	t.Run("multi arg", func(t *testing.T) {
		const src = `raise "x", "y" rescue 7`
		body, fallback := beginWrapping(t, src, parseOne(t, src))
		call := wantCall(t, src, body, "raise", 2)
		for i, a := range call.Args {
			if _, ok := a.(*ast.StringLit); !ok {
				t.Fatalf("%q: arg %d = %T, want *ast.StringLit", src, i, a)
			}
		}
		if i, ok := fallback.(*ast.IntLit); !ok || i.Value != 7 {
			t.Fatalf("%q: fallback = %#v, want IntLit 7", src, fallback)
		}
	})

	t.Run("side-effect command", func(t *testing.T) {
		const src = `puts "a" rescue nil`
		body, fallback := beginWrapping(t, src, parseOne(t, src))
		call := wantCall(t, src, body, "puts", 1)
		if _, ok := call.Args[0].(*ast.StringLit); !ok {
			t.Fatalf("%q: arg 0 = %T, want *ast.StringLit", src, call.Args[0])
		}
		if _, ok := fallback.(*ast.NilLit); !ok {
			t.Fatalf("%q: fallback = %T, want *ast.NilLit", src, fallback)
		}
	})
}

// TestModifierRescueBeginBodyLastStatement is the originally-reported shape: a
// command-call rescue as the last statement of a begin…end must now catch (the
// rescue wraps the Call inside the begin body).
func TestModifierRescueBeginBodyLastStatement(t *testing.T) {
	const src = "begin\n  raise \"x\" rescue nil\nend"
	outer, ok := parseOne(t, src).(*ast.Begin)
	if !ok {
		t.Fatalf("%q: top = %T, want *ast.Begin", src, parseOne(t, src))
	}
	if len(outer.Body) != 1 {
		t.Fatalf("%q: outer begin body has %d nodes, want 1", src, len(outer.Body))
	}
	body, fallback := beginWrapping(t, src, outer.Body[0])
	wantCall(t, src, body, "raise", 1)
	if _, ok := fallback.(*ast.NilLit); !ok {
		t.Fatalf("%q: fallback = %T, want *ast.NilLit", src, fallback)
	}
}

// TestModifierRescueUnchangedNonCommand covers the cases that must keep their
// existing (already-correct) behaviour.
func TestModifierRescueUnchangedNonCommand(t *testing.T) {
	// A bare parenthesised expression: the rescue stays inside the group and wraps
	// the literal, and the group evaluates to that Begin.
	t.Run("paren group expr", func(t *testing.T) {
		const src = `("x" rescue 42)`
		body, fallback := beginWrapping(t, src, parseOne(t, src))
		if _, ok := body.(*ast.StringLit); !ok {
			t.Fatalf("%q: wrapped node = %T, want *ast.StringLit", src, body)
		}
		if _, ok := fallback.(*ast.IntLit); !ok {
			t.Fatalf("%q: fallback = %T, want *ast.IntLit", src, fallback)
		}
	})

	// Assignment RHS: the rescue binds to the RHS, inside the Assign (MRI:
	// assign(x, rescue_mod(bar, 9))), so the top node stays an Assign.
	t.Run("assignment rhs", func(t *testing.T) {
		const src = `foo = bar rescue 9`
		asn, ok := parseOne(t, src).(*ast.Assign)
		if !ok {
			t.Fatalf("%q: top = %T, want *ast.Assign", src, parseOne(t, src))
		}
		if asn.Name != "foo" {
			t.Fatalf("%q: assign target = %q, want foo", src, asn.Name)
		}
		body, fallback := beginWrapping(t, src, asn.Value)
		wantCall(t, src, body, "bar", 0)
		if _, ok := fallback.(*ast.IntLit); !ok {
			t.Fatalf("%q: fallback = %T, want *ast.IntLit", src, fallback)
		}
	})

	// Parenthesised method call: already parses as (method(args)) rescue x.
	t.Run("paren call", func(t *testing.T) {
		const src = `method(args) rescue x`
		body, _ := beginWrapping(t, src, parseOne(t, src))
		wantCall(t, src, body, "method", 1)
	})
}

// TestModifierRescueInsideDelimitedArg checks that a modifier rescue inside a
// nested delimited context (an explicit paren group, or a parenthesised argument
// list) is still consumed there — the command-call suppression must not leak in.
func TestModifierRescueInsideDelimitedArg(t *testing.T) {
	// Explicit paren group as a command argument: the rescue binds inside the
	// group, so the group becomes a Begin argument of the command.
	t.Run("paren group as command arg", func(t *testing.T) {
		const src = `p ("x" rescue 42)`
		call := wantCall(t, src, parseOne(t, src), "p", 1)
		body, _ := beginWrapping(t, src, call.Args[0])
		if _, ok := body.(*ast.StringLit); !ok {
			t.Fatalf("%q: wrapped node = %T, want *ast.StringLit", src, body)
		}
	})

	// Parenthesised argument list nested inside a command call: the inner rescue
	// belongs to that argument (`foo(bar rescue baz)`), while `outer` keeps both.
	t.Run("paren arglist inside command", func(t *testing.T) {
		const src = `outer foo(bar rescue baz)`
		outer := wantCall(t, src, parseOne(t, src), "outer", 1)
		foo := wantCall(t, src, outer.Args[0], "foo", 1)
		body, _ := beginWrapping(t, src, foo.Args[0])
		wantCall(t, src, body, "bar", 0)
	})
}

// TestModifierRescueAfterDefEnd covers `def…end rescue nil`, which MRI accepts
// (Ripper: rescue_mod(def(...), nil)) — the rescue wraps the whole method def.
func TestModifierRescueAfterDefEnd(t *testing.T) {
	const src = "def foo; end rescue nil"
	body, fallback := beginWrapping(t, src, parseOne(t, src))
	def, ok := body.(*ast.MethodDef)
	if !ok {
		t.Fatalf("%q: wrapped node = %T, want *ast.MethodDef", src, body)
	}
	if def.Name != "foo" {
		t.Fatalf("%q: def name = %q, want foo", src, def.Name)
	}
	if _, ok := fallback.(*ast.NilLit); !ok {
		t.Fatalf("%q: fallback = %T, want *ast.NilLit", src, fallback)
	}
}

// TestModifierRescueBindsTighterThanIf checks precedence against MRI: a command
// call with a trailing rescue and a trailing if modifier is
// `(cmd args rescue fb) if cond` (Ripper: if_mod(rescue_mod(command,…),cond)).
func TestModifierRescueBindsTighterThanIf(t *testing.T) {
	const src = `foo a, b rescue 7 if cond`
	iff, ok := parseOne(t, src).(*ast.If)
	if !ok {
		t.Fatalf("%q: top = %T, want *ast.If", src, parseOne(t, src))
	}
	if len(iff.Then) != 1 {
		t.Fatalf("%q: If.Then has %d nodes, want 1", src, len(iff.Then))
	}
	body, fallback := beginWrapping(t, src, iff.Then[0])
	wantCall(t, src, body, "foo", 2)
	if _, ok := fallback.(*ast.IntLit); !ok {
		t.Fatalf("%q: fallback = %T, want *ast.IntLit", src, fallback)
	}
}
