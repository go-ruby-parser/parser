package parser

import (
	"testing"

	"github.com/go-ruby-parser/parser/ast"
)

// TestJumpValueSplat covers a `*splat` as the value of return/break/next, which
// MRI gathers into an implicit array (`return *ary` ≡ `return [*ary]`).
func TestJumpValueSplat(t *testing.T) {
	for _, src := range []string{
		`def r; return *ary; end`,
		`def r; return 1, *ary; end`,
		`while true; break *[1, 2]; end`,
		`x.each { next *nil }`,
		`loop { break *a, *b }`,
	} {
		if _, err := Parse(src); err != nil {
			t.Errorf("Parse(%q) error: %v", src, err)
		}
	}
	// `return *ary` wraps the splat in a one-element array.
	prog, err := Parse(`def r; return *ary; end`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	def := prog.Body[0].(*ast.MethodDef)
	ret := def.Body[len(def.Body)-1].(*ast.Return)
	arr, ok := ret.Value.(*ast.ArrayLit)
	if !ok || len(arr.Elems) != 1 {
		t.Fatalf("return *ary: expected one-element ArrayLit, got %#v", ret.Value)
	}
	if _, ok := arr.Elems[0].(*ast.SplatArg); !ok {
		t.Fatalf("return *ary: expected SplatArg element, got %T", arr.Elems[0])
	}
}

// TestBlockParamTrailingComma covers a trailing comma in a block parameter list
// (`{ |a,| }`), which MRI treats as forcing array destructuring of a single
// argument — equivalent to an anonymous rest `|a, *|`.
func TestBlockParamTrailingComma(t *testing.T) {
	for _, src := range []string{
		`m { |a,| a }`,
		`h.each_pair { |k,| k }`,
		`lambda { |a, | a }`,
		`define_method(:m) { |a,| a }`,
		`m { |a, b,| a }`,
	} {
		if _, err := Parse(src); err != nil {
			t.Errorf("Parse(%q) error: %v", src, err)
		}
	}
	// `{ |a,| }` records an anonymous rest so a single array argument destructures.
	prog, err := Parse(`m { |a,| a }`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	call := prog.Body[0].(*ast.Call)
	if call.Block == nil {
		t.Fatal("expected a block")
	}
	if call.Block.SplatIndex != 1 {
		t.Fatalf("expected an anonymous rest at index 1 (|a, *|), got SplatIndex=%d params=%v",
			call.Block.SplatIndex, call.Block.Params)
	}
}

// TestTernaryJumpArm covers a bare value-less control-flow jump as a ternary arm
// (`cond ? next : x`, `flag ? break : y`), which MRI permits.
func TestTernaryJumpArm(t *testing.T) {
	for _, src := range []string{
		`(x == 3 ? next : y) while i < 10`,
		`(x == 3 ? next : y) until i > 10`,
		`z = cond ? break : 5`,
		`a ? next : b`,
		`a ? b : redo`,
		`a ? return : c`,
	} {
		if _, err := Parse(src); err != nil {
			t.Errorf("Parse(%q) error: %v", src, err)
		}
	}
	// The ordinary (non-jump) ternary is unaffected.
	for _, src := range []string{`cond ? 1 : 2`, `x ? a = 1 : b = 2`} {
		if _, err := Parse(src); err != nil {
			t.Errorf("Parse(%q) regressed: %v", src, err)
		}
	}
}

// TestSpecialGvarSymbol covers symbols naming the special global variables
// (`:$~`, `:$!`, `:$&`), the option globals (`:$-w`) and match-group references
// (`:$1`), alongside the ordinary `:$name` / `:@ivar` symbol forms.
func TestSpecialGvarSymbol(t *testing.T) {
	for _, src := range []string{
		`bind.local_variable_get(:$~)`,
		`p [:$!, :$&, :$', :$/, :$;, :$,, :$., :$<, :$>, :$0, :$*, :$$, :$?, :$:, :$"]`,
		"p :$`",
		`p :$1`,
		`p :$12`,
		`p :$-w`,
		`p :$stdin`,
		`p :$LOAD_PATH`,
	} {
		if _, err := Parse(src); err != nil {
			t.Errorf("Parse(%q) error: %v", src, err)
		}
	}
	// The symbol names the special global including its `$`.
	prog, err := Parse(`p :$~`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	call := prog.Body[0].(*ast.Call)
	sym, ok := call.Args[0].(*ast.SymbolLit)
	if !ok || sym.Name != "$~" {
		t.Fatalf("expected SymbolLit :$~, got %#v", call.Args[0])
	}
}
