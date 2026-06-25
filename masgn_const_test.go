package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

func masgn(t *testing.T, src string) *ast.MultiAssign {
	t.Helper()
	m, ok := parseOne(t, src).(*ast.MultiAssign)
	if !ok {
		t.Fatalf("Parse(%q): node = %T, want *ast.MultiAssign", src, parseOne(t, src))
	}
	return m
}

func TestMasgnConstTargets(t *testing.T) {
	m := masgn(t, "A, B = 1, 2")
	if len(m.Targets) != 2 {
		t.Fatalf("Targets len = %d, want 2", len(m.Targets))
	}
	for i, want := range []string{"A", "B"} {
		cr, ok := m.Targets[i].(*ast.ConstRef)
		if !ok || cr.Name != want {
			t.Errorf("Targets[%d] = %#v, want ConstRef %q", i, m.Targets[i], want)
		}
		if m.Names[i] != "" {
			t.Errorf("Names[%d] = %q, want empty for a constant target", i, m.Names[i])
		}
	}
}

func TestMasgnMixedConstLocal(t *testing.T) {
	m := masgn(t, "A, b = 1, 2")
	if _, ok := m.Targets[0].(*ast.ConstRef); !ok {
		t.Errorf("Targets[0] = %T, want *ast.ConstRef", m.Targets[0])
	}
	vr, ok := m.Targets[1].(*ast.VarRef)
	if !ok || vr.Name != "b" {
		t.Errorf("Targets[1] = %#v, want VarRef b", m.Targets[1])
	}
	if m.Names[1] != "b" {
		t.Errorf("Names[1] = %q, want b", m.Names[1])
	}
}

func TestMasgnConstSplat(t *testing.T) {
	m := masgn(t, "a, *B = 1, 2, 3")
	if m.SplatIndex != 1 {
		t.Errorf("SplatIndex = %d, want 1", m.SplatIndex)
	}
	if _, ok := m.Targets[1].(*ast.ConstRef); !ok {
		t.Errorf("Targets[1] = %T, want *ast.ConstRef", m.Targets[1])
	}
}

func TestMasgnAllLocalsTargetsNil(t *testing.T) {
	// The all-locals fast path leaves Targets nil so existing consumers are
	// unaffected.
	m := masgn(t, "a, b = 1, 2")
	if m.Targets != nil {
		t.Errorf("all-locals Targets = %#v, want nil", m.Targets)
	}
	if m.Names[0] != "a" || m.Names[1] != "b" {
		t.Errorf("Names = %v, want [a b]", m.Names)
	}
}

func TestMasgnConstValidPrograms(t *testing.T) {
	for _, src := range []string{"A, B = 1, 2", "A, b = 1, 2", "a, *B = 1, 2, 3", "X, Y, Z = foo"} {
		if _, err := parser.Parse(src); err != nil {
			t.Errorf("Parse(%q) returned error: %v", src, err)
		}
	}
}
