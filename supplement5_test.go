package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
)

// errorBranches drives the parser's and lexer's expect()/raise paths.
var errorBranches = []string{
	"def 1",                 // bad def name
	"def f(a",               // unterminated def params
	"def f(1)",              // non-identifier param
	"class lower",           // class name must be a constant
	"class 1",               // bad class name
	"module 1",              // bad module name
	"case 1\nin\nend",       // in with no pattern
	"[1].each { |a",         // unterminated block params
	"x = ->(a",              // unterminated lambda params
	"x = (1",                // unterminated parenthesized expr
	"foo[1",                 // unterminated index
	"x = @",                 // ivar with no name
	"until",                 // until with no condition
	"unless",                // unless with no condition
	"while",                 // while with no condition
	"super(",                // unterminated super args
	"yield(",                // unterminated yield args
	"x = a ? b",             // ternary missing ':'
	"begin\nrescue =>\nend", // rescue binding with no var
	"def f\nrescue =>\nend", // method rescue, bad binding
	"{ 1 => }",              // hash pair missing value
	"[1,",                   // trailing comma, then EOF
	"x = !",                 // unary bang with no operand
	"x = -",                 // unary minus with no operand
	"1 2",                   // two primaries, no operator
	"x = %Q",                // %Q at EOF
	"<<~",                   // squiggly heredoc marker, nothing
}

func TestErrorBranches(t *testing.T) {
	for _, src := range errorBranches {
		if _, err := parser.Parse(src); err == nil {
			t.Errorf("expected a parse error for %q, got none", src)
		}
	}
}
