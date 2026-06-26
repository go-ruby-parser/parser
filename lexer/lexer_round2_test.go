package lexer

import (
	"testing"

	"github.com/go-ruby-parser/parser/token"
)

// countNewlines tokenizes src and returns the number of NEWLINE tokens it emits.
func countNewlines(src string) int {
	n := 0
	for _, t := range New(src).Tokenize() {
		if t.Type == token.NEWLINE {
			n++
		}
	}
	return n
}

// TestKeywordOperatorContinuation: a line ending in a trailing low-precedence
// keyword operator (`and`/`or`) or a trailing modifier keyword (`if`/`unless`/
// `while`/`until`) joins the next line, so the intervening newline is suppressed.
func TestKeywordOperatorContinuation(t *testing.T) {
	for _, src := range []string{
		"a and\nb",
		"a or\nb",
		"foo if\nbar",
		"foo unless\nbar",
		"foo while\nbar",
		"foo until\nbar",
	} {
		if got := countNewlines(src); got != 0 {
			t.Errorf("%q: %d NEWLINE tokens, want 0 (the line should continue)", src, got)
		}
	}
}

// TestKeywordContinuationDoesNotEatRealNewlines: a keyword that is NOT the last
// token on its line (the usual `if cond` form) leaves the statement newline
// intact.
func TestKeywordContinuationLeavesNormalNewlines(t *testing.T) {
	// `if x` ends in `x`, not `if`, so the following newline is preserved.
	if got := countNewlines("if x\ny"); got != 1 {
		t.Errorf("`if x\\ny`: %d NEWLINE tokens, want 1", got)
	}
}
