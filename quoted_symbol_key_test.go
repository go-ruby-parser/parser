package parser

import (
	"testing"

	"github.com/go-ruby-parser/parser/ast"
)

// A quoted string key followed by `:` is a symbol key, the quoted form of a
// `name:` label: `{'desc': 'x'}`, `{"d-a-s-h": ""}`.
func TestQuotedSymbolKeyInHash(t *testing.T) {
	cases := []struct {
		src string
		key string
	}{
		{`{'desc': 'x'}`, "desc"},
		{`{"d-a-s-h": ""}`, "d-a-s-h"},
		{`{'0': {}}`, "0"},
		{`{"a": 1, "b": 2}`, "a"},
	}
	for _, c := range cases {
		prog, err := Parse(c.src)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", c.src, err)
		}
		h, ok := prog.Body[0].(*ast.HashLit)
		if !ok {
			t.Fatalf("%q: expected *ast.HashLit, got %T", c.src, prog.Body[0])
		}
		sym, ok := h.Keys[0].(*ast.SymbolLit)
		if !ok {
			t.Fatalf("%q: key0 = %T, want *ast.SymbolLit", c.src, h.Keys[0])
		}
		if sym.Name != c.key {
			t.Fatalf("%q: key0 name=%q, want %q", c.src, sym.Name, c.key)
		}
	}
}

// The same shorthand works in a method call's keyword arguments.
func TestQuotedSymbolKeyInCall(t *testing.T) {
	prog, err := Parse(`tag(:div, "@click": "f()")`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	call := prog.Body[0].(*ast.Call)
	last := call.Args[len(call.Args)-1]
	h, ok := last.(*ast.HashLit)
	if !ok {
		t.Fatalf("expected trailing *ast.HashLit, got %T", last)
	}
	if sym, ok := h.Keys[0].(*ast.SymbolLit); !ok || sym.Name != "@click" {
		t.Fatalf("expected symbol key @click, got %#v", h.Keys[0])
	}
}

// An interpolated string key becomes a dynamic `"…".to_sym` symbol key.
func TestInterpolatedSymbolKey(t *testing.T) {
	prog, err := Parse(`{"with_#{name}": 1}`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	h := prog.Body[0].(*ast.HashLit)
	call, ok := h.Keys[0].(*ast.Call)
	if !ok || call.Name != "to_sym" {
		t.Fatalf("expected key0 to be a .to_sym call, got %#v", h.Keys[0])
	}
	if _, ok := call.Recv.(*ast.StrInterp); !ok {
		t.Fatalf("expected to_sym receiver to be *ast.StrInterp, got %T", call.Recv)
	}
}

// A ternary whose condition/branches are strings is unaffected: the `:` belongs
// to the ternary, not a symbol key.
func TestQuotedSymbolKeyTernaryNoRegression(t *testing.T) {
	for _, src := range []string{
		`x = cond ? "a" : "b"`,
		`foo("a" ? 1 : 2)`,
	} {
		if _, err := Parse(src); err != nil {
			t.Fatalf("Parse(%q) error: %v", src, err)
		}
	}
}
