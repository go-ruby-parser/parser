package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
)

var extraValid6 = []string{
	// single-quoted strings + escapes (scanQuotedRaw)
	// adjacent string concatenation
	// call-argument forms (parseArg / parseOneCallArg)
	"foo 1, 2",
	"foo *args",
	"foo &blk",
	"obj.meth(1) { |x| x }",
	// method-name variants (methodName)
	"x = a.foo?",
	"x = a.foo!",
	"a.bar = 1",
	"x = a.b.c.d",
	"x = a&.b&.c(1)",
	"x = A::B::C",
	"x = obj::Const",
	// hash literal forms (parseHashLiteral)
	"h = {a: 1, b: 2}",
	"h = {:s => 1, \"k\" => 2}",
	"h = {1 => 2, 3 => 4}",
	"h = {**base, c: 3}",
	// index get/set
	"a = [0]\na[0] = 1",
	"a = [0, 0]\na[0, 1] = 2",
	"x = a[1]",
	"x = a[1..2]",
	// begin/rescue full surface
	"begin\n  risky\nrescue TypeError => e\n  1\nrescue ArgumentError, NameError\n  2\nelse\n  3\nensure\n  4\nend",
	"begin\n  x\nrescue\n  retry\nend",
	// symbol spellings (lexSymbol)
	"x = :foo?",
	"x = :foo!",
	"x = :foo=",
	"x = :\"quoted sym\"",
	// special globals (lexGvar)
	"p $stdout",
	"p $0",
	// floats / numeric forms (lexNumber)
	"x = 1_000_000",
	"x = 3.14",
	// heredoc with trailing method + interpolation + escapes (wrapHeredocDQ)
	"x = <<E.upcase\nhi #{1}\nE",
	"x = <<~T\n  a\n\n  b #{1}\n  \\t tab\nT",
	// unary / defined? / not
	"x = defined?(@foo)",
	"x = not nil",
	"y = !!x",
	// proc/lambda keywords
	"f = proc { |x| x }",
	"g = lambda { 1 }",
	// ternary + range in arg position
	"foo(a ? 1 : 2)",
	"foo(1..10)",
	// splat in array literal, nested data
	"x = [*a, 1, *b]",
	"x = [[1, 2], {a: 3}, [4]]",
}

func TestExtraValid6(t *testing.T) {
	for _, src := range extraValid6 {
		if _, err := parser.Parse(src); err != nil {
			t.Errorf("Parse(%q) returned error: %v", src, err)
		}
	}
}
