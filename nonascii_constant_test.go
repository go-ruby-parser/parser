package parser

import (
	"testing"

	"github.com/go-ruby-parser/parser/ast"
)

// TestNonASCIIConstantIdentifier covers the parse-level consequence of Ruby
// deciding constant-ness from the first character's CASE rather than from its
// being in 'A'..'Z': a name beginning with a non-ASCII uppercase or titlecase
// letter is a CONSTANT, and one beginning with a lowercase or CASELESS letter is
// a local variable.
//
// ⛔ `LookupIdent` tested `s[0]`, a BYTE, so every multibyte name came back
// IDENT. `ἍBB = 1` parsed as a local-variable assignment, which is why
// `defined?(ἍBB)` answered nil in rbgo where MRI answers "constant", and why
// ruby/spec's `language/variables_spec.rb` "parses a non-ASCII upcased
// character as a constant identifier" could not pass
// (go-embedded-ruby/ruby#799).
//
// Each classification was measured against /usr/bin/ruby; the per-character
// table and its citations live in token.IsConstStart.
func TestNonASCIIConstantIdentifier(t *testing.T) {
	for _, tc := range []struct {
		src  string
		kind string // "const" or "local"
	}{
		{"ἍBB = 1", "const"},  // GREEK CAPITAL ALPHA WITH PSILI AND OXIA (Lu)
		{"Дeta = 1", "const"}, // CYRILLIC CAPITAL DE (Lu)
		{"École = 1", "const"},
		{"ǅx = 1", "const"}, // titlecase digraph (Lt)
		{"μ = 1", "local"},  // GREEK SMALL MU (Ll)
		{"日x = 1", "local"}, // CJK, caseless (Lo)
		{"אx = 1", "local"}, // HEBREW ALEF, caseless (Lo)
		{"Ascii = 1", "const"},
		{"ascii = 1", "local"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			prog, err := Parse(tc.src)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tc.src, err)
			}
			if len(prog.Body) != 1 {
				t.Fatalf("Parse(%q) gave %d statements, want 1", tc.src, len(prog.Body))
			}
			switch node := prog.Body[0].(type) {
			case *ast.ConstAssign:
				if tc.kind != "const" {
					t.Errorf("Parse(%q) gave *ast.ConstAssign, want a local assignment", tc.src)
				}
			case *ast.Assign:
				if tc.kind != "local" {
					t.Errorf("Parse(%q) gave *ast.Assign, want *ast.ConstAssign", tc.src)
				}
			default:
				t.Errorf("Parse(%q) gave %T, want *ast.ConstAssign or *ast.Assign", tc.src, node)
			}
		})
	}
}

// A non-ASCII constant must work everywhere an ASCII one does, or the fix would
// only have moved the problem into the parser's other constant positions.
func TestNonASCIIConstantInEveryPosition(t *testing.T) {
	for _, src := range []string{
		"ἍBB",                   // a read
		"ἍBB = 1",               // an assignment
		"ἍBB ||= 1",             // an op-assign
		"ἍBB::Inner",            // as a scope
		"Outer::ἍBB",            // as a scoped name
		"ἍBB, x = 1, 2",         // a masgn target
		"class ἍBB; end",        // a class name
		"module ἍBB; end",       // a module name
		"class ἍBB < Base; end", // and with a superclass
		"case x; in ἍBB; end",   // a pattern
		"def ἍBB.m; end",        // a singleton definee
		"ἍBB.new",               // a receiver
	} {
		if _, err := Parse(src); err != nil {
			t.Errorf("Parse(%q) error: %v", src, err)
		}
	}
	if _, err := Parse("begin\n  x\nrescue ἍBB => e\nend"); err != nil {
		t.Errorf("non-ASCII constant as a rescue class: %v", err)
	}
}
