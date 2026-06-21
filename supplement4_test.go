package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
)

// extraValid4 hits the un-bracketed pattern grammar and other last branches.
var extraValid4 = []string{
	// un-bracketed (implicit) array patterns -> parsePattern/parseArrayPatternRest
	"case [1, 2]\nin a, b\n  a\nend",
	"case [1]\nin *rest\n  rest\nend",
	"case [1, 2, 3]\nin x, *rest, y\n  rest\nend",
	// un-bracketed (implicit) hash patterns
	"case {a: 1}\nin a:\n  a\nend",
	"case {a: 1, b: 2}\nin a:, b:\n  a\nend",
	"case {a: 1}\nin **rest\n  rest\nend",
	"case {a: 1}\nin **nil\n  :ok\nend",
}

func TestExtraValid4(t *testing.T) {
	for _, src := range extraValid4 {
		if _, err := parser.Parse(src); err != nil {
			t.Errorf("Parse(%q) returned error: %v", src, err)
		}
	}
}

// extraErrors4 hits lexer/parser error branches (e.g. atPercentArray falses).
var extraErrors4 = []string{
	"%w",  // %w at EOF (no delimiter)
	"%wz", // %w followed by a non-delimiter
	"x = %q(unterminated",
	"x = %Q[unterminated",
}

func TestExtraErrors4(t *testing.T) {
	for _, src := range extraErrors4 {
		if _, err := parser.Parse(src); err == nil {
			t.Errorf("expected a parse error for %q, got none", src)
		}
	}
}

// extraValid9 covers the parser-feature batch: %q/%Q/%() string literals,
// {x:} hash shorthand, and adjacent string-literal concatenation.
var extraValid9 = []string{
	`x = %q(hi there)`,
	`x = %q[a\]b]`,
	`x = %q{nest (parens) ok}`,
	`x = %q!bang!`,
	`n = 1; x = %Q(val #{n})`,
	`n = 1; x = %(plain #{n})`,
	`x = %Q{a #{1 + 2} b}`,
	`x = 1; h = {x:}`,
	`def m(a, b); {a:, b:}; end`,
	`h = {foo:}`, // foo is a method call (not local)
	`p "a" "b"`,
	`x = "one" "two" "three"`,
}

func TestExtraValid9(t *testing.T) {
	for _, src := range extraValid9 {
		if _, err := parser.Parse(src); err != nil {
			t.Errorf("Parse(%q) returned error: %v", src, err)
		}
	}
}
