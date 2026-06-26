package parser

import (
	"testing"

	"github.com/go-ruby-parser/parser/ast"
)

// A `do…end` block following a paren-less command call on a receiver binds to
// that command call, mirroring MRI (`obj.foo bar do … end`). Previously the
// receiver command-call path returned before it could attach the block.
func TestReceiverCommandDoBlock(t *testing.T) {
	cases := []string{
		`recv.set_callback :work, prepend: true do |x| end`,
		`Capybara.register_driver name do |app| end`,
		`obj.instrument "x", y do end`,
		`ActiveSupport.on_load(:action_cable) do; end`,
	}
	for _, src := range cases {
		if _, err := Parse(src); err != nil {
			t.Fatalf("Parse(%q) error: %v", src, err)
		}
	}
}

// The block-bearing receiver command call can itself be chained (the block
// attaches to the command, then the postfix chain continues).
func TestReceiverCommandDoBlockChained(t *testing.T) {
	prog, err := Parse(`app.foo bar do end.baz`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	outer, ok := prog.Body[0].(*ast.Call)
	if !ok || outer.Name != "baz" {
		t.Fatalf("expected outer .baz call, got %T", prog.Body[0])
	}
	inner, ok := outer.Recv.(*ast.Call)
	if !ok || inner.Name != "foo" || inner.Block == nil {
		t.Fatalf("expected inner foo call with block, got %#v", outer.Recv)
	}
}

// A braced block, by contrast, binds tighter (to the nearest call/arg), and a
// `do` inside a while/until condition still belongs to the loop, not the call.
func TestReceiverCommandBlockBoundaries(t *testing.T) {
	for _, src := range []string{
		`while obj.foo bar do end`,
		`recv.each x { |y| y }`,
	} {
		if _, err := Parse(src); err != nil {
			t.Fatalf("Parse(%q) error: %v", src, err)
		}
	}
}
