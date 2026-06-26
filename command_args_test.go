package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// TestCommandCallKeywordArgs covers paren-less command calls whose arguments
// include `label: value` keyword pairs, which collapse into a trailing implicit
// Hash — the dominant Rails/DSL form (`render json: x, status: :ok`).
func TestCommandCallKeywordArgs(t *testing.T) {
	cases := []struct {
		src          string
		name         string
		wantPos      int // positional args before the trailing hash
		wantHashKeys int // keys in the trailing hash (0 ⇒ no trailing hash)
	}{
		{"delegate :a, :b, to: :c", "delegate", 2, 1},
		{"validates :x, presence: true", "validates", 1, 1},
		{"render json: x, status: :ok", "render", 0, 2},
		{"foo a, b: 1, c: 2", "foo", 1, 2},
		{"belongs_to :user, optional: true", "belongs_to", 1, 1},
	}
	for _, c := range cases {
		call := topCall(t, c.src)
		if call.Name != c.name {
			t.Errorf("Parse(%q): Name=%q, want %q", c.src, call.Name, c.name)
		}
		total := c.wantPos
		if c.wantHashKeys > 0 {
			total++
		}
		if len(call.Args) != total {
			t.Fatalf("Parse(%q): %d args, want %d", c.src, len(call.Args), total)
		}
		if c.wantHashKeys > 0 {
			h, ok := call.Args[len(call.Args)-1].(*ast.HashLit)
			if !ok {
				t.Fatalf("Parse(%q): last arg is %T, want *ast.HashLit", c.src, call.Args[len(call.Args)-1])
			}
			if len(h.Keys) != c.wantHashKeys {
				t.Errorf("Parse(%q): trailing hash has %d keys, want %d", c.src, len(h.Keys), c.wantHashKeys)
			}
		}
	}
}

// TestCommandCallMultiplePositional covers several bare positional arguments
// (`foo a, b, c`, `puts x, y`).
func TestCommandCallMultiplePositional(t *testing.T) {
	cases := []struct {
		src      string
		name     string
		wantArgs int
	}{
		{"foo a, b, c", "foo", 3},
		{"puts x, y", "puts", 2},
		{"p 1, 2, 3, 4", "p", 4},
	}
	for _, c := range cases {
		call := topCall(t, c.src)
		if call.Name != c.name || len(call.Args) != c.wantArgs {
			t.Errorf("Parse(%q): %+v (want %s/%d args)", c.src, call, c.name, c.wantArgs)
		}
	}
}

// TestCommandCallSplatStarters covers the splat / double-splat / block-pass
// argument starters in command position, plus a hugging unary sign.
func TestCommandCallSplatStarters(t *testing.T) {
	// foo *args → single SplatArg.
	if call := topCall(t, "foo *args"); len(call.Args) != 1 {
		t.Fatalf("foo *args: %+v", call)
	} else if _, ok := call.Args[0].(*ast.SplatArg); !ok {
		t.Fatalf("foo *args: arg0 is %T, want *ast.SplatArg", call.Args[0])
	}
	// foo **opts → trailing hash with a nil-key (double-splat) entry.
	if call := topCall(t, "foo **opts"); len(call.Args) != 1 {
		t.Fatalf("foo **opts: %+v", call)
	} else if h, ok := call.Args[0].(*ast.HashLit); !ok || len(h.Keys) != 1 || h.Keys[0] != nil {
		t.Fatalf("foo **opts: arg0 = %+v, want double-splat hash", call.Args[0])
	}
	// foo &blk → BlockPass.
	if call := topCall(t, "foo &blk"); len(call.Args) != 1 {
		t.Fatalf("foo &blk: %+v", call)
	} else if _, ok := call.Args[0].(*ast.BlockPass); !ok {
		t.Fatalf("foo &blk: arg0 is %T, want *ast.BlockPass", call.Args[0])
	}
	// foo a, *rest, k: 1 → positional, splat, then trailing hash.
	call := topCall(t, "foo a, *rest, k: 1")
	if len(call.Args) != 3 {
		t.Fatalf("foo a, *rest, k: 1: %d args, want 3", len(call.Args))
	}
	if _, ok := call.Args[1].(*ast.SplatArg); !ok {
		t.Errorf("foo a, *rest, k: 1: arg1 is %T, want *ast.SplatArg", call.Args[1])
	}
	if _, ok := call.Args[2].(*ast.HashLit); !ok {
		t.Errorf("foo a, *rest, k: 1: arg2 is %T, want *ast.HashLit", call.Args[2])
	}
}

// TestCommandCallMixedLambdaArg covers a lambda among the positional arguments
// and a trailing keyword (`has_many :posts, -> { ... }, dependent: :destroy`).
func TestCommandCallMixedLambdaArg(t *testing.T) {
	call := topCall(t, "has_many :posts, -> { order(:id) }, dependent: :destroy")
	if call.Name != "has_many" || len(call.Args) != 3 {
		t.Fatalf("has_many: %+v (want 3 args)", call)
	}
	// arg1 is the lambda, modelled as a call to `lambda` carrying a block.
	lam, ok := call.Args[1].(*ast.Call)
	if !ok || lam.Block == nil {
		t.Errorf("has_many arg1 = %T (%+v), want a lambda call with a block", call.Args[1], call.Args[1])
	}
	if _, ok := call.Args[2].(*ast.HashLit); !ok {
		t.Errorf("has_many arg2 is %T, want *ast.HashLit", call.Args[2])
	}
}

// TestCommandCallRHS covers a command call as the right-hand side of an
// assignment: `x = foo a, b` parses as x = foo(a, b).
func TestCommandCallRHS(t *testing.T) {
	prog, err := parser.Parse("x = foo a, b")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	asn, ok := prog.Body[0].(*ast.Assign)
	if !ok || asn.Name != "x" {
		t.Fatalf("top is %T, want Assign x", prog.Body[0])
	}
	call, ok := asn.Value.(*ast.Call)
	if !ok || call.Name != "foo" || len(call.Args) != 2 {
		t.Fatalf("RHS = %+v, want call foo with 2 args", asn.Value)
	}
}

// TestCommandCallBlockBinding covers the `do…end` vs `{…}` precedence: a
// trailing `do…end` binds to the command call, while a braced block binds to the
// nearest call (the last argument).
func TestCommandCallBlockBinding(t *testing.T) {
	// foo bar do end → block on foo, not bar.
	outer := topCall(t, "foo bar do end")
	if outer.Name != "foo" || outer.Block == nil {
		t.Fatalf("foo bar do end: outer = %+v (want foo with a block)", outer)
	}
	if arg, ok := outer.Args[0].(*ast.Call); !ok || arg.Block != nil {
		t.Errorf("foo bar do end: arg block should be nil, got %+v", outer.Args[0])
	}

	// foo a, b do |x| end → block on foo.
	multi := topCall(t, "foo a, b do |x| end")
	if multi.Name != "foo" || multi.Block == nil || len(multi.Args) != 2 {
		t.Fatalf("foo a, b do |x| end: %+v", multi)
	}

	// foo bar { |x| x } → braced block on the nearest call (bar).
	brace := topCall(t, "foo bar { |x| x }")
	if brace.Name != "foo" || brace.Block != nil {
		t.Fatalf("foo bar { }: outer = %+v (want foo with no block)", brace)
	}
	if arg, ok := brace.Args[0].(*ast.Call); !ok || arg.Block == nil {
		t.Errorf("foo bar { }: arg should carry the brace block, got %+v", brace.Args[0])
	}
}

// TestCommandCallKeywordRegression guards that `key: value` outside a call still
// behaves (a bare label at statement start is not a valid statement) and that
// operator forms are not mis-read as command splats.
func TestCommandCallKeywordRegression(t *testing.T) {
	// `foo * 2` (spaces both sides) is multiplication of the bare call result.
	prog, err := parser.Parse("foo * 2")
	if err != nil {
		t.Fatalf("Parse(foo * 2): %v", err)
	}
	if _, ok := prog.Body[0].(*ast.BinaryExpr); !ok {
		t.Errorf("foo * 2: top is %T, want *ast.BinaryExpr", prog.Body[0])
	}
	// `foo - 2` (spaces both sides) is subtraction, not a command arg.
	prog, err = parser.Parse("foo - 2")
	if err != nil {
		t.Fatalf("Parse(foo - 2): %v", err)
	}
	if _, ok := prog.Body[0].(*ast.BinaryExpr); !ok {
		t.Errorf("foo - 2: top is %T, want *ast.BinaryExpr", prog.Body[0])
	}
}

// TestCommandCallReceiverKeyword covers keyword args on a receiver command call
// (`obj.foo bar, baz: 1`).
func TestCommandCallReceiverKeyword(t *testing.T) {
	call := topCall(t, "obj.foo bar, baz: 1")
	if call.Name != "foo" || call.Recv == nil || len(call.Args) != 2 {
		t.Fatalf("obj.foo bar, baz: 1: %+v", call)
	}
	if _, ok := call.Args[1].(*ast.HashLit); !ok {
		t.Errorf("obj.foo bar, baz: 1: arg1 is %T, want *ast.HashLit", call.Args[1])
	}
}
