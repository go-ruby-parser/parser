package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// bracedTopCall parses src and returns its single top-level Call.
func bracedTopCall(t *testing.T, src string) *ast.Call {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	if len(prog.Body) != 1 {
		t.Fatalf("Parse(%q): want 1 top-level node, got %d", src, len(prog.Body))
	}
	call, ok := prog.Body[0].(*ast.Call)
	if !ok {
		t.Fatalf("Parse(%q): top node is %T, want *ast.Call", src, prog.Body[0])
	}
	return call
}

// TestHashLitBracedAtCallSite is the bit itself: at a call site a braced hash
// literal is a POSITIONAL Hash and bare `k: v` pairs are KEYWORDS, and the two
// build structurally identical HashLits that only Braced tells apart.
//
// MRI draws the same line in the same place: `primary: tLBRACE assoc_list '}'`
// sets nd_brace (parse.y-ruby_4_0:4415-4419) and nothing else does
// (rb_node_hash_new, parse.y-ruby_4_0:11784-11791).
func TestHashLitBracedAtCallSite(t *testing.T) {
	cases := []struct {
		src        string
		argIdx     int
		wantBraced bool
		wantPairs  int
	}{
		// The pair from issue #40. Same Keys/Values, different written form.
		{"f({k: 1})", 0, true, 1},
		{"f(k: 1)", 0, false, 1},
		// Empty braces are still a positional Hash — and `f()` below shows the
		// contrast is not "is there a hash", since there it is absent entirely.
		{"f({})", 0, true, 0},
		// A double-splat INSIDE braces builds a braced hash; bare at the call
		// site it is the keyword form. `assoc: tDSTAR arg_value`
		// (parse.y-ruby_4_0:6682) is reached from both, so only Braced differs.
		{"f({**h})", 0, true, 1},
		{"f(**h)", 0, false, 1},
		{"f(k: 1, **h)", 0, false, 2},
		// Hashrocket form is braced just the same.
		{"f({1 => 2})", 0, true, 1},
		// A braced literal after a splat is still positional.
		{"f(*a, {k: 1})", 1, true, 1},
		// Keywords collected after an ordinary positional argument.
		{"f(a, k: 1)", 1, false, 1},
		// An array literal shares the argument grammar, so the same two
		// spellings must stay distinguishable there too.
		{"[1, k: 2]", -1, false, 1},
		{"[1, {k: 2}]", -1, true, 1},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			var node ast.Node
			if tc.argIdx < 0 {
				prog, err := parser.Parse(tc.src)
				if err != nil {
					t.Fatalf("Parse(%q): %v", tc.src, err)
				}
				arr, ok := prog.Body[0].(*ast.ArrayLit)
				if !ok {
					t.Fatalf("Parse(%q): top node is %T, want *ast.ArrayLit", tc.src, prog.Body[0])
				}
				node = arr.Elems[len(arr.Elems)-1]
			} else {
				call := bracedTopCall(t, tc.src)
				if tc.argIdx >= len(call.Args) {
					t.Fatalf("Parse(%q): want an arg at %d, got %d args", tc.src, tc.argIdx, len(call.Args))
				}
				node = call.Args[tc.argIdx]
			}
			h, ok := node.(*ast.HashLit)
			if !ok {
				t.Fatalf("Parse(%q): node is %T, want *ast.HashLit", tc.src, node)
			}
			if h.Braced != tc.wantBraced {
				t.Errorf("Parse(%q): Braced = %v, want %v", tc.src, h.Braced, tc.wantBraced)
			}
			if len(h.Keys) != tc.wantPairs || len(h.Values) != tc.wantPairs {
				t.Errorf("Parse(%q): %d keys / %d values, want %d each",
					tc.src, len(h.Keys), len(h.Values), tc.wantPairs)
			}
		})
	}
}

// TestHashLitBracedOutsideArguments pins the bit down as WRITTEN FORM and not
// "is a keyword argument": a braced literal in a non-argument position is
// Braced as well, which is what MRI records too — nd_brace is set by the
// literal production with no call-site condition on it.
func TestHashLitBracedOutsideArguments(t *testing.T) {
	for _, src := range []string{"{a: 1}", "x = {a: 1}", "{1 => 2}", "{}"} {
		prog, err := parser.Parse(src)
		if err != nil {
			t.Fatalf("Parse(%q): %v", src, err)
		}
		node := prog.Body[0]
		if a, ok := node.(*ast.Assign); ok {
			node = a.Value
		}
		h, ok := node.(*ast.HashLit)
		if !ok {
			t.Fatalf("Parse(%q): node is %T, want *ast.HashLit", src, node)
		}
		if !h.Braced {
			t.Errorf("Parse(%q): Braced = false, want true", src)
		}
	}
}

// TestHashLitBracedDoesNotClaimBlocks is the case a careless rule breaks the
// language on: in `obj.m {}` the braces are a BLOCK, not a hash. Ours cannot
// confuse them because a `{` after a call never reaches parseHashLiteral —
// structurally the same defence as MRI's lexer, which decides between tLBRACE
// (hash) and '{' / tLBRACE_ARG (block) before the grammar ever sees it
// (parse.y-ruby_4_0:11141-11153). So there is no HashLit here to mislabel.
func TestHashLitBracedDoesNotClaimBlocks(t *testing.T) {
	for _, src := range []string{"obj.m {}", "obj.m { |x| x }", "f {}", "f { |x| x }"} {
		call := bracedTopCall(t, src)
		if call.Block == nil {
			t.Fatalf("Parse(%q): want a Block, got none", src)
		}
		for i, arg := range call.Args {
			if h, ok := arg.(*ast.HashLit); ok {
				t.Errorf("Parse(%q): arg %d is a HashLit{Braced:%v}; the braces are a block",
					src, i, h.Braced)
			}
		}
	}
	// And a real braced argument alongside a real block: both readings at once.
	call := bracedTopCall(t, "obj.m({k: 1}) { |y| y }")
	if call.Block == nil {
		t.Fatal(`Parse("obj.m({k: 1}) { |y| y }"): want a Block, got none`)
	}
	if len(call.Args) != 1 {
		t.Fatalf(`Parse("obj.m({k: 1}) { |y| y }"): want 1 arg, got %d`, len(call.Args))
	}
	h, ok := call.Args[0].(*ast.HashLit)
	if !ok {
		t.Fatalf(`Parse("obj.m({k: 1}) { |y| y }"): arg is %T, want *ast.HashLit`, call.Args[0])
	}
	if !h.Braced {
		t.Error(`Parse("obj.m({k: 1}) { |y| y }"): Braced = false, want true`)
	}
}

// TestHashLitBracedAbsentWhenNoHash guards the probe against measuring nothing:
// `f()` and `f(h)` must build NO HashLit at all, so the `f({})`/`f({k: 1})`
// results above are about the brace and not about the argument count.
func TestHashLitBracedAbsentWhenNoHash(t *testing.T) {
	for _, src := range []string{"f()", "f(h)", "f"} {
		call := bracedTopCall(t, src)
		for i, arg := range call.Args {
			if _, ok := arg.(*ast.HashLit); ok {
				t.Errorf("Parse(%q): arg %d is a HashLit, want none", src, i)
			}
		}
	}
}
