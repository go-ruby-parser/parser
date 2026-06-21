package token

import "testing"

func TestTypeString(t *testing.T) {
	if INT.String() != "INT" {
		t.Errorf("INT.String()=%q", INT.String())
	}
	if PLUS.String() != "+" {
		t.Errorf("PLUS.String()=%q", PLUS.String())
	}
	if Type(9999).String() != "Type?" {
		t.Errorf("unknown Type.String()=%q", Type(9999).String())
	}
}

func TestLookupIdent(t *testing.T) {
	if LookupIdent("def") != DEF {
		t.Error("def should be DEF")
	}
	if LookupIdent("Foo") != CONST {
		t.Error("Foo should be CONST")
	}
	if LookupIdent("foo") != IDENT {
		t.Error("foo should be IDENT")
	}
}
