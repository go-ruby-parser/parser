package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// casePattern parses `case … in <pat> …` and returns the clause's pattern.
func casePattern(t *testing.T, src string) ast.Pattern {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q) returned error: %v", src, err)
	}
	c, ok := prog.Body[0].(*ast.CaseIn)
	if !ok {
		t.Fatalf("Parse(%q): node = %T, want *ast.CaseIn", src, prog.Body[0])
	}
	if len(c.Clauses) != 1 {
		t.Fatalf("Parse(%q): %d in-clauses, want 1", src, len(c.Clauses))
	}
	return c.Clauses[0].Pattern
}

// TestConstPatternParenthesesTakeTheSameBodies covers MRI's p_expr_basic
// (parse.y v3_4_0): `p_const p_lparen p_args rparen`, `… p_find rparen`,
// `… p_kwargs rparen` and `p_const '(' rparen` are the same four alternatives
// as the p_lbracket/rbracket quartet, so `Array(0, 1, 2)` is an array pattern
// and `Point(x:, y:)` a hash pattern — the body decides, not the bracket.
func TestConstPatternParenthesesTakeTheSameBodies(t *testing.T) {
	t.Parallel()
	if _, ok := casePattern(t, `case x; in Array(0, 1, 2); 1; end`).(*ast.ArrayPattern); !ok {
		t.Error("Array(0, 1, 2) is not an *ast.ArrayPattern")
	}
	if _, ok := casePattern(t, `case x; in Point(a:); 1; end`).(*ast.HashPattern); !ok {
		t.Error("Point(a:) is not an *ast.HashPattern")
	}
	if _, ok := casePattern(t, `case x; in Array(); 1; end`).(*ast.ArrayPattern); !ok {
		t.Error("Array() is not an *ast.ArrayPattern")
	}
	if _, ok := casePattern(t, `case x; in Array(*, 1, *); 1; end`).(*ast.FindPattern); !ok {
		t.Error("Array(*, 1, *) is not an *ast.FindPattern")
	}
}

// TestPatternTrailingCommaIsAnAnonymousRest covers MRI's
// `p_top_expr_body: p_expr ','` (parse.y v3_4_0). A trailing comma makes the
// array pattern match any array STARTING with the listed elements, so
// `in [0, 1, ]` matches [0, 1, 2, 3] — before this the comma was dropped, which
// silently turned a partial match into an exact one.
func TestPatternTrailingCommaIsAnAnonymousRest(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		`case x; in [0, 1, ]; 1; end`,
		"case x\nin 0, 1,;\n1\nend",
		`case x; in [0, 1, ] then 1; end`,
	} {
		ap, ok := casePattern(t, src).(*ast.ArrayPattern)
		if !ok {
			t.Fatalf("Parse(%q): pattern = %T, want *ast.ArrayPattern", src, casePattern(t, src))
		}
		if !ap.HasSplat || ap.SplatName != "" {
			t.Errorf("Parse(%q): HasSplat=%v SplatName=%q, want true and \"\"", src, ap.HasSplat, ap.SplatName)
		}
		if len(ap.Pre) != 2 || len(ap.Post) != 0 {
			t.Errorf("Parse(%q): %d pre / %d post elements, want 2 / 0", src, len(ap.Pre), len(ap.Post))
		}
	}
	// Without the comma the pattern stays exact.
	ap, ok := casePattern(t, `case x; in [0, 1]; 1; end`).(*ast.ArrayPattern)
	if !ok || ap.HasSplat {
		t.Error("in [0, 1] must not carry a rest")
	}
}

// TestHashPatternStringKey covers p_kw_label's second spelling,
// `tSTRING_BEG string_contents tLABEL_END` (parse.y v3_4_0): a quoted string
// followed by `:` names the same symbol key as a bare label. A `"a" => 1`
// rocket entry is not a pattern key, and ruby/spec requires it to be rejected.
func TestHashPatternStringKey(t *testing.T) {
	t.Parallel()
	hp, ok := casePattern(t, `case x; in {"a": 0}; 1; end`).(*ast.HashPattern)
	if !ok {
		t.Fatalf(`in {"a": 0} is not an *ast.HashPattern`)
	}
	if len(hp.Keys) != 1 || hp.Keys[0] != "a" {
		t.Errorf(`in {"a": 0}: keys = %q, want ["a"]`, hp.Keys)
	}
	if _, err := parser.Parse(`case x; in {"a" => 1}; 1; end`); err == nil {
		t.Error(`in {"a" => 1}: want a parse error`)
	}
}

// TestPinNonLocalVariable covers `p_var_ref: '^' tIDENTIFIER | '^' nonlocal_var`
// with `nonlocal_var: tIVAR | tGVAR | tCVAR` (parse.y v3_4_0, 5883-5897): a pin
// may name an instance, global or class variable, not only a local.
func TestPinNonLocalVariable(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		src  string
		want any
	}{
		{`case x; in ^@a; 1; end`, (*ast.IvarRef)(nil)},
		{`case x; in ^$a; 1; end`, (*ast.GVarRef)(nil)},
		{`case x; in ^@@a; 1; end`, (*ast.CVarRef)(nil)},
		{`case x; in ^a; 1; end`, (*ast.VarRef)(nil)},
	} {
		vp, ok := casePattern(t, tc.src).(*ast.ValuePattern)
		if !ok {
			t.Fatalf("Parse(%q): pattern is not an *ast.ValuePattern", tc.src)
		}
		if got, want := nodeType(vp.Value), nodeType(tc.want); got != want {
			t.Errorf("Parse(%q): pinned value = %s, want %s", tc.src, got, want)
		}
	}
}
