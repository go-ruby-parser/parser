package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
)

// knownParseErrors are the harvested snippets that are *meant* to fail parsing
// (malformed syntax / unterminated literals). They are kept in the corpus
// because they exercise the lexer's and parser's error paths.
var knownParseErrors = map[string]bool{
	"\"#{1 2}\"":                           true, // two expressions in one interpolation
	":":                                    true, // bare colon
	"case [1]\nin [a, *b, *c]\np 1\nend":   true, // two splats in an array pattern
	"case {a: 1}\nin {**rest, **nil}\nend": true, // **rest together with **nil
	"nil { 1 }":                            true, // block attached to a nil literal
	"p(%w[a b c)":                          true, // unterminated %w literal
	"p(/abc":                               true, // unterminated regexp
	"p(/abc\\":                             true, // unterminated regexp (trailing backslash)
	"x = 1 <<":                             true, // shift/heredoc marker at EOF
	"x = 1 <<-":                            true, // heredoc dash at EOF
	"x = :\"a\\":                           true, // unterminated quoted symbol
	"x = :\"unterminated":                  true, // unterminated quoted symbol
}

// TestCorpus parses the real Ruby snippets harvested from go-embedded-ruby's
// interpreter suite: the bulk must parse cleanly, while the known-bad fixtures
// must surface a parse error rather than crash.
func TestCorpus(t *testing.T) {
	for _, src := range validCorpus {
		_, err := parser.Parse(src)
		if knownParseErrors[src] {
			if err == nil {
				t.Errorf("expected a parse error for %q, got none", src)
			}
			continue
		}
		if err != nil {
			t.Errorf("Parse(%q) returned error: %v", src, err)
		}
	}
}
