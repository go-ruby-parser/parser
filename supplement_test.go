package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
)

// extraValid covers grammar the harvested corpus underexercises (the harvest
// skews to one-line expression tests). Each must parse cleanly.
var extraValid = []string{
	// if / unless / while / until blocks
	"if a\n  b\nelsif c\n  d\nelse\n  e\nend",
	"unless a\n  b\nelse\n  c\nend",
	"while a\n  b\nend",
	"until a\n  b\nend",
	"begin\n  x\nend while a",
	"if a then b else c end",
	// return / break / next, with and without a value
	"def f\n  return\nend",
	"def f\n  return 5\nend",
	"while true\n  break\nend",
	"while true\n  break 5\nend",
	"[1].each { next }",
	"[1].each { next 1 }",
	"loop { redo }",
	// module
	"module M\n  X = 1\nend",
	"module M\n  def self.f; 1; end\nend",
	// lambdas and do-blocks
	"f = ->(x) { x }",
	"g = -> { 1 }",
	"h = ->(x, y) { x + y }",
	"[1].each do |x|\n  puts x\nend",
	"[1].reduce(0) do |acc, x|\n  acc + x\nend",
	// numbered params / it
	"[1].map { _1 }",
	"[1].map { _1 + _2 }",
	"[1].map { it }",
	// radix integer literals
	"0xFF",
	"0XaB_cD",
	"0o17",
	"0O17",
	"0b1010",
	"0B1010",
	"0d99",
	"0777",
	// global and instance/class variables
	"x = $foo",
	"\"abc\" =~ /b/\np $~",
	"@x = 1\n@x",
	// interpolating percent arrays
	"%W[a#{1} b c]",
	"%I[a#{1} b c]",
	"%w[a b c]",
	"%i[a b c]",
	// keyword params and keyword arguments
	"def f(a:, b: 2)\n  a + b\nend",
	"foo(a: 1, b: 2)",
	"def f(**opts)\n  opts\nend",
	"foo(**h)",
	// def name variants: setter, operators, index, class method
	"def name=(v)\n  @name = v\nend",
	"def +(o)\n  o\nend",
	"def [](i)\n  i\nend",
	"def []=(i, v)\n  v\nend",
	"def <=>(o)\n  0\nend",
	"def self.bar\n  1\nend",
	"def foo = 42",
	// one-line pattern match
	"x = 1\nx => Integer",
	"x = 1\nx in Integer",
	// statement modifiers
	"b if a",
	"b unless a",
	"b while a",
	"b until a",
	"risky rescue fallback",
	// block params with splat / destructuring
	"[[1, 2]].each { |a, b| a }",
	"[[1, 2]].each { |(a, b)| a }",
}

func TestExtraValid(t *testing.T) {
	for _, src := range extraValid {
		if _, err := parser.Parse(src); err != nil {
			t.Errorf("Parse(%q) returned error: %v", src, err)
		}
	}
}
