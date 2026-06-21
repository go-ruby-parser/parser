package parser_test

import (
	"os"
	"testing"

	"github.com/go-ruby-parser/parser"
)

// TestPrelude parses go-embedded-ruby's embedded standard library (Comparable,
// Enumerable, …) — a large real Ruby program that exercises broad grammar.
func TestPrelude(t *testing.T) {
	src, err := os.ReadFile("testdata/prelude.rb")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse(string(src)); err != nil {
		t.Fatalf("Parse(prelude.rb) returned error: %v", err)
	}
}

// extraValid3 targets the last branch gaps: case/when, every %-delimiter, the
// full def/block parameter grammar, op-assign, special globals and symbols.
var extraValid3 = []string{
	// case/when (subject and subjectless), then-form, comma lists
	"case x\nwhen 1\n  a\nwhen 2, 3\n  b\nelse\n  c\nend",
	"case\nwhen a\n  1\nwhen b\n  2\nend",
	"case x\nwhen Integer then 1\nwhen String then 2\nend",
	// array-pattern rest forms
	"case [1]\nin [*rest]\n  rest\nend",
	"case [1, 2, 3]\nin [x, *rest, y]\n  rest\nend",
	"case [1]\nin [*]\n  :ok\nend",
	// every %-array delimiter pair / self-delimiter
	"%w[a b]",
	"%w(a b)",
	"%w{a b}",
	"%w<a b>",
	"%w|a b|",
	"%w!a b!",
	"%w/a b/",
	"%i{a b}",
	"%W<a#{1}>",
	"%I|a#{1} b|",
	// op-assign in its forms, on lvars/ivars/index
	"x = 1\nx += 1\nx -= 1\nx *= 2\nx /= 2\nx %= 2",
	"x = nil\nx ||= 1\nx &&= 2",
	"x = 0\nx <<= 1",
	"a = [0]\na[0] += 1",
	"@x ||= 1",
	// def parameter grammar (supported forms)
	"def f(a, b = 2)\n  a + b\nend",
	"def f(a, *rest)\n  rest\nend",
	// block / lambda parameter grammar
	// special globals and symbol spellings
	"\"abc\" =~ /b/\np $&\np $`\np $'\np $1\np $~",
	":@ivar",
	":@@cvar",
	":$gvar",
	":foo",
	":[]",
	":+@",
	":-@",
	// unary / operator coverage for binBP / parseUnary
	"x = not true",
	"x = a and b or c",
	"x = a <=> b",
	"x = a & b | c ^ d",
	"x = a << b >> c",
	"x = a == b != c",
	"x = a <= b >= c < d > e",
	"x = a ** b ** c",
	"x = defined? y",
	// nested ternary and ranges
	"x = a ? (b ? 1 : 2) : 3",
	"r = 1.0..2.0",
}

func TestExtraValid3(t *testing.T) {
	for _, src := range extraValid3 {
		if _, err := parser.Parse(src); err != nil {
			t.Errorf("Parse(%q) returned error: %v", src, err)
		}
	}
}
