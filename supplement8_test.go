package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
)

// extraValid8 targets the last specific uncovered branches identified from the
// coverage profile (implicit params, pattern atoms, block-param groups,
// attribute op-assign, nested-brace interpolation, regexp/heredoc edges).
var extraValid8 = []string{
	"x = _1",                            // _1 in a hard (top-level) scope -> plain ident
	"[1].each { |x| _1 }",               // _1 with explicit params -> plain ident
	"o = nil\no.x += 1",                 // compound attribute assignment
	"x = -2 ** 3",                       // negative literal with ** (precedence)
	"[1].each { |(a, *b)| a }",          // splat inside a grouped block param
	"def f()\n  1\nend",                 // empty parameter list
	"f = ->() do 1 end",                 // lambda with a do/end block
	"case 1\nin (1)\n  :a\nend",         // parenthesised (grouped) pattern
	"case [1]\nin [a,]\n  a\nend",       // array pattern with a trailing comma
	"case 1\nin x unless false\n  :a\nend", // pattern guard with `unless`
	"def f\n  super 1, 2\nend",          // super with command args (no parens)
	"foo(:a => 1)",                      // hashrocket call argument
	"x = /abc/imx",                      // regexp flags
	"x = %w[a [b] c]",                   // nested brackets in a %w literal
	"x = 1<<2",                          // shift (atHeredoc false path)
	"p 99999999999999999999999999999",  // bignum literal
	"x = \"#{ {a: 1} }\"",               // nested braces in string interpolation
	"x = :\"a#{ {b: 1} }\"",             // nested braces in quoted-symbol interpolation
	"x = <<E\nsay \"hi\" #{ {a: 1} }\nE", // heredoc: a quote + nested-brace interp
	"x = %W[a#{[1].map { |y| y }}]",     // nested braces in a %W interpolation
	"x = :\"a\\nb\"",                    // quoted symbol with an escape
	"x = <<~T\nflush\nT",                // squiggly heredoc, a zero-indent line
	"x = /a\\/b/",                       // regexp with an escaped slash
	"case 5\nin ^(1 + 1)\n  :a\nend",   // ^(expr) pin pattern
	"case [1]\nin Array[a]\n  a\nend",  // const array (deconstruct) pattern
	"[1].each { | | 1 }",                  // empty (spaced) block params
}

func TestExtraValid8(t *testing.T) {
	for _, src := range extraValid8 {
		if _, err := parser.Parse(src); err != nil {
			t.Errorf("Parse(%q) returned error: %v", src, err)
		}
	}
}

// extraErrors8 targets uncovered error/illegal branches.
var extraErrors8 = []string{
	"x = a.",                          // method name missing after '.'
	"p 08",                            // invalid octal literal
	"x = `cmd`",                       // backtick is an illegal token
	"x = 0x",                          // radix prefix with no digits
	"p $",                             // bare '$' with no name
	"x = 'oops",                       // unterminated single-quoted string
	"[1].each { _1 + it }",            // `it` mixed with numbered params
	"[1].each { _1; [2].each { _1 } }", // nested numbered parameters
}

func TestExtraErrors8(t *testing.T) {
	for _, src := range extraErrors8 {
		if _, err := parser.Parse(src); err == nil {
			t.Errorf("expected a parse error for %q, got none", src)
		}
	}
}
