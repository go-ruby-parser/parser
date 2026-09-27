package parser

import (
	"strings"
	"testing"

	"github.com/go-ruby-parser/parser/ast"
	"github.com/go-ruby-parser/parser/token"
)

// TestNegateLiteralRefusesANonNumericNode covers negateLiteral's default arm,
// which no Ruby source can reach: parseUnary calls the helper only after
// parsePrimary consumed an INT or FLOAT token, so the node is always an IntLit, a
// BignumLit or a FloatLit, possibly wrapped by applyNumSuffix in a RationalLit
// and/or an ImaginaryLit — the five arms above it.
//
// The arm is kept rather than deleted because deleting it means going back to the
// trailing `n.(*ast.FloatLit)` assertion it replaced, which panicked on every
// `-3r`, `-3i` and `-3ri`. Its job is to preserve the parser's never-panics
// contract if some future caller reaches here with another node kind: a refusal,
// not an assertion. This white-box test is how that job is witnessed — the same
// reason parseHook exists in robustness_internal_test.go.
func TestNegateLiteralRefusesANonNumericNode(t *testing.T) {
	// p.fail reads p.cur() for the line number, and the cursor clamps at the
	// trailing EOF, so a one-token stream is the minimum valid state.
	p := &Parser{toks: []token.Token{{Type: token.EOF}}}
	var err error
	func() {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("negateLiteral(NilLit): returned normally, want a parseError")
			}
			pe, ok := r.(parseError)
			if !ok {
				t.Fatalf("negateLiteral(NilLit): panicked with %T (%v), want a parseError", r, r)
			}
			err = pe
		}()
		p.negateLiteral(&ast.NilLit{})
	}()
	if !strings.Contains(err.Error(), "cannot negate") {
		t.Errorf("error = %q, want it to name the refusal", err)
	}
}
