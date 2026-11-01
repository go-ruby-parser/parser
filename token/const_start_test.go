package token

import "testing"

// Ruby decides constant-ness from the first character's CASE, not from its being
// in 'A'..'Z'. Every row here was measured against /usr/bin/ruby through
// `Object.const_defined?(name.to_sym)`, which raises NameError for a name that
// is not a constant name — that is rb_is_const_name (symbol.c:1287), which is
// the same predicate parse.y consults.
//
// ⛔ Before this, the test was `s[0] >= 'A' && s[0] <= 'Z'` on a BYTE, so every
// multibyte name read as a local: `ἍBB = 1` created a local variable and no
// constant existed afterwards.
func TestIsConstStartFollowsRubysCaseRule(t *testing.T) {
	for _, tc := range []struct {
		name  string
		first string // the leading character, by name and category
		konst bool
	}{
		// ASCII, unchanged.
		{"A", "ASCII upper", true},
		{"Z", "ASCII upper", true},
		{"a", "ASCII lower", false},
		{"z", "ASCII lower", false},
		{"_", "ASCII underscore", false},
		{"_Foo", "ASCII underscore", false},

		// Lu — uppercase letters outside ASCII.
		{"ἍBB", "GREEK CAPITAL ALPHA WITH PSILI AND OXIA (Lu)", true},
		{"Дa", "CYRILLIC CAPITAL DE (Lu)", true},
		{"İstanbul", "LATIN CAPITAL I WITH DOT ABOVE (Lu)", true},
		{"École", "LATIN CAPITAL E WITH ACUTE (Lu)", true},

		// ⛔ Lt — titlecase letters. rb_sym_constant_char_p tests the
		// "titlecaseletter" Onigmo property explicitly, AFTER isupper and
		// islower have both said no, so these are constants although
		// unicode.IsUpper alone reports false for them.
		{"ǅx", "LATIN CAPITAL D WITH SMALL Z WITH CARON (Lt)", true},
		{"ǈx", "LATIN CAPITAL L WITH SMALL J (Lt)", true},

		// Ll — lowercase letters outside ASCII.
		{"μm", "GREEK SMALL MU (Ll)", false},
		{"école", "LATIN SMALL E WITH ACUTE (Ll)", false},

		// ⛔ Lo — CASELESS letters start a LOCAL variable, not a constant.
		// This is why neither clause may be written as "not lowercase".
		{"日x", "CJK UNIFIED IDEOGRAPH 65E5 (Lo)", false},
		{"אx", "HEBREW ALEF (Lo)", false},
		{"بx", "ARABIC BEH (Lo)", false},

		// Degenerate inputs: no character to read is not an uppercase one.
		{"", "empty", false},
		{"\xff\xfe", "invalid UTF-8", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsConstStart(tc.name); got != tc.konst {
				t.Errorf("IsConstStart(%q) = %v, want %v — %s", tc.name, got, tc.konst, tc.first)
			}
			// LookupIdent must agree, since that is what the lexer calls. A
			// keyword is neither, and no keyword is non-ASCII, so every row here
			// reaches the same branch.
			want := IDENT
			if tc.konst {
				want = CONST
			}
			if tc.name == "" {
				return // LookupIdent is never called with an empty identifier
			}
			if got := LookupIdent(tc.name); got != want {
				t.Errorf("LookupIdent(%q) = %v, want %v — %s", tc.name, got, want, tc.first)
			}
		})
	}
}

// The control: a keyword still wins over the case rule, and the rule has not
// swallowed the ASCII path. Without this, a version of IsConstStart that always
// answered false would still pass the `false` rows above.
func TestIsConstStartControls(t *testing.T) {
	if LookupIdent("def") != DEF {
		t.Error("a keyword must still be looked up as a keyword")
	}
	if !IsConstStart("Foo") {
		t.Fatal("the ASCII path is broken: Foo must be a constant start")
	}
	if IsConstStart("foo") {
		t.Fatal("the ASCII path is broken: foo must not be a constant start")
	}
	// And the planted positive for the branch this change added: a name that is
	// a constant ONLY because of the non-ASCII clause.
	if !IsConstStart("ἍBB") {
		t.Fatal("the non-ASCII clause is not reached")
	}
}
