package parser_test

import (
	"strings"
	"testing"

	"github.com/go-ruby-parser/parser"
)

// extraValid2 fills the long tail: heredocs, multiple assignment, the full
// pattern-matching grammar, yield/super, quoted symbols and call-argument forms.
var extraValid2 = []string{
	// heredocs: plain, dash, squiggly, literal, interpolating, stacked
	"x = <<HEREDOC\nbody\nHEREDOC",
	"x = <<-HEREDOC\n  body\n  HEREDOC",
	"x = <<~SQL\n  select 1\n  from t\nSQL",
	"x = <<'LIT'\nno #{interp}\nLIT",
	"n = 1\nx = <<~SQL\n  val #{n}\nSQL",
	"a, b = <<A, <<B\nfirst\nA\nsecond\nB",
	// multiple assignment / destructuring
	"a, b = 1, 2",
	"a, b = b, a",
	"a, *b = 1, 2, 3",
	"*a, b = 1, 2, 3",
	"a, b, c = [1, 2, 3]",
	// pattern matching: array, find, hash, pin, alternative, range, nil/bool, guard
	"case [1, 2]\nin [a, b]\n  a\nend",
	"case [1, 2, 3]\nin [a, *rest]\n  rest\nend",
	"case [1, 2, 3]\nin [*pre, x]\n  x\nend",
	"case [1, 2, 3]\nin [*, 2, *]\n  :found\nend",
	"case {a: 1}\nin {a: Integer => n}\n  n\nend",
	"case {a: 1, b: 2}\nin {a:, **rest}\n  rest\nend",
	"case {a: 1}\nin {a: 1, **nil}\n  :ok\nend",
	"x = 5\ncase 5\nin ^x\n  :pin\nend",
	"case 3\nin 1 | 2 | 3\n  :alt\nend",
	"case 5\nin 1..10\n  :range\nend",
	"case nil\nin nil\n  :nil\nend",
	"case 5\nin Integer => n if n > 0\n  n\nend",
	"case 5\nin x\n  x\nelse\n  :none\nend",
	"case {a: 1}\nin Hash(a:)\n  a\nend",
	// yield / super
	"def f\n  yield\nend",
	"def f\n  yield 1, 2\nend",
	"def f\n  super\nend",
	"def f\n  super(1, 2)\nend",
	"def f\n  super()\nend",
	// quoted / interpolated symbols
	"x = :\"a b\"",
	"n = 1\nx = :\"a#{n}\"",
	":+",
	":<<",
	":[]=",
	":<=>",
	// call-argument forms: splat, double-splat, block-pass, mixed
	"foo(*args)",
	"foo(1, *args, 2)",
	"foo(&blk)",
	"foo(1, *a, b: 2, &c)",
	"foo(**opts)",
	// misc primaries / operators
	"x = defined?(y)",
	"r = (1..)",
	"r = (..10)",
	"r = (1...10)",
	"x = a&.b&.c",
	"x = !true",
	"x = -5",
	"x = ~3",
	"x = +5",
	"x = a ? b : c",
	"x = \"a#{1 + 2}b#{3}c\"",
}

// parseErrorCorpus exercises the parser's and lexer's error paths.
var parseErrorCorpus = []string{
	":",                            // bare colon
	"\"#{1 2}\"",                   // two expressions in one interpolation
	"p(/abc",                       // unterminated regexp
	"p(%w[a b c)",                  // unterminated %w
	"x = :\"unterminated",          // unterminated quoted symbol
	"def",                          // def with no name
	"class",                        // class with no name
	"if",                           // if with no condition/body
	"case 1\nin [a, *b, *c]\nend",  // two splats in array pattern
	"case 1\nin {**a, **nil}\nend", // **rest with **nil
	"1 +",                          // dangling binary operator
	"foo(",                         // unterminated call args
	"[1, 2",                        // unterminated array
	"{a: 1",                        // unterminated hash
	"begin\n  x",                   // begin without end
}

func TestExtraValid2(t *testing.T) {
	for _, src := range extraValid2 {
		if _, err := parser.Parse(src); err != nil {
			t.Errorf("Parse(%q) returned error: %v", src, err)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for _, src := range parseErrorCorpus {
		_, err := parser.Parse(src)
		if err == nil {
			t.Errorf("expected a parse error for %q, got none", src)
			continue
		}
		// exercise parseError.Error()
		if !strings.Contains(err.Error(), "parse error") {
			t.Errorf("error for %q = %q, want it to mention \"parse error\"", src, err.Error())
		}
	}
}
