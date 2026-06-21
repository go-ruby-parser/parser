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
	"x = %q(hi)", // %q not an array letter -> % treated as modulo -> error
	"%w",         // %w at EOF (no delimiter)
	"%wz",        // %w followed by a non-delimiter
}

func TestExtraErrors4(t *testing.T) {
	for _, src := range extraErrors4 {
		if _, err := parser.Parse(src); err == nil {
			t.Errorf("expected a parse error for %q, got none", src)
		}
	}
}
