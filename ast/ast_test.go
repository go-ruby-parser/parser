package ast

import "testing"

// TestNodeMarkers exercises the no-op node() marker methods so the interface
// witnesses are covered.
func TestNodeMarkers(t *testing.T) {
	nodes := []Node{
		&Program{}, &IntLit{}, &FloatLit{}, &StringLit{}, &BoolLit{}, &NilLit{},
		&SelfLit{}, &VarRef{}, &Assign{}, &BinaryExpr{}, &UnaryExpr{}, &Call{},
		&If{}, &While{}, &MethodDef{}, &Return{},
		&ConstRef{}, &IvarRef{}, &IvarAssign{}, &ClassDef{},
		&ModuleDef{}, &Super{}, &Yield{}, &SymbolLit{}, &ArrayLit{}, &HashLit{}, &RangeLit{},
		&Break{}, &Next{}, &OpAssign{}, &Begin{}, &StrInterp{}, &Case{}, &Retry{}, &SplatArg{},
		&BlockPass{}, &ConstAssign{}, &GVarRef{}, &GVarAssign{}, &CVarRef{}, &CVarAssign{}, &MultiAssign{}, &CaseIn{}, &MatchPattern{},
		&BignumLit{},
	}
	for _, n := range nodes {
		n.node()
	}
	if len(nodes) != 45 {
		t.Fatalf("expected 45 node kinds, got %d", len(nodes))
	}
}

// TestPatternMarkers exercises the no-op pattern() marker methods.
func TestPatternMarkers(t *testing.T) {
	pats := []Pattern{
		&ValuePattern{}, &BindPattern{}, &ConstPattern{}, &BindingPattern{}, &ArrayPattern{},
		&HashPattern{}, &AltPattern{}, &FindPattern{},
	}
	for _, p := range pats {
		p.pattern()
	}
	if len(pats) != 8 {
		t.Fatalf("expected 8 pattern kinds, got %d", len(pats))
	}
}
