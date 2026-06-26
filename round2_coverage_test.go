package parser

import "testing"

// A non-string hash key followed by a bare `:` (not `=>`) is a parse error — it
// exercises stringKeyColon's fall-through (a colon present, but the key is not a
// string), after which the `=>` requirement fails.
func TestNonStringKeyColonError(t *testing.T) {
	for _, src := range []string{
		`{1 + 1 : 2}`,
		`{foo : 2}`,
	} {
		if _, err := Parse(src); err == nil {
			t.Fatalf("expected a parse error for %q", src)
		}
	}
}

// alias/undef with a non-name item is a parse error (parseFitem's fail path).
func TestAliasUndefBadItemError(t *testing.T) {
	for _, src := range []string{
		`alias 1 2`,
		`undef 99`,
		`alias`, // missing both items
	} {
		if _, err := Parse(src); err == nil {
			t.Fatalf("expected a parse error for %q", src)
		}
	}
}

// An interpolated quoted symbol key in a call argument hits stringKeyColon's
// StrInterp branch (a `.to_sym` dynamic key) in the call path too.
func TestInterpolatedSymbolKeyInCall(t *testing.T) {
	if _, err := Parse(`foo("x#{y}": 1)`); err != nil {
		t.Fatalf("Parse error: %v", err)
	}
}
