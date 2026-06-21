package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
)

// extraValid7 covers literals added after the extraction: single-quoted strings
// and scientific float notation.
var extraValid7 = []string{
	"x = 'plain'",
	"x = 'a\\'b'",
	"x = 'a\\\\b'",
	"x = 'no #{interp} here'",
	"x = ''",
	"x = 'a' + 'b'",
	"x = 1.5e3",
	"x = 1e3",
	"x = 1.0e-3",
	"x = 2E2",
	"x = 1e+2",
	"x = 1_000.5e1",
}

func TestExtraValid7(t *testing.T) {
	for _, src := range extraValid7 {
		if _, err := parser.Parse(src); err != nil {
			t.Errorf("Parse(%q) returned error: %v", src, err)
		}
	}
}
