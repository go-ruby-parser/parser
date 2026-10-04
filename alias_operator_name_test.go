package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
)

// TestAliasAndUndefNameAnOperator: `alias` and `undef` take METHOD NAMES, so an
// operator character after either spells a name rather than opening a literal.
// MRI's grammar says so --
//
//	k_alias fitem {SET_LEX_STATE(EXPR_FNAME|EXPR_FITEM);} fitem
//	k_undef undef_list
//
// -- and the lexer's atMethodName listed only `def`, `.`, `&.` and `::`. So
// `alias / +` was refused with "expected a method name": the `/` opened a
// regexp. Only `/` was affected, because it is the one operator character that
// also opens a literal in a value position -- `alias + plus`, `alias [] at` and
// `alias <=> cmp` all parsed.
//
// It is how Pathname aliases its `/`, and CRuby's own pathname.rb line 358 is
// that exact line, which is how this was found: the stdlib sweep added for the
// README ran on CI's Ruby 3.2 and refused one file.
func TestAliasAndUndefNameAnOperator(t *testing.T) {
	for _, tc := range []struct{ label, src string }{
		{"alias / +", "class C\n  def +(o); o; end\n  alias / +\nend\n"},
		{"alias % mod", "class C\n  def mod(o); o; end\n  alias % mod\nend\n"},
		{"undef /", "class C\n  def /(o); o; end\n  undef /\nend\n"},
		{"undef /, %", "class C\n  def /(o); o; end\n  def %(o); o; end\n  undef /, %\nend\n"},
		// These parsed before and must keep parsing: the operator as the OLD name,
		// and the non-slash operators, which never opened a literal.
		{"alias plus +", "class C\n  def +(o); o; end\n  alias plus +\nend\n"},
		{"alias + plus", "class C\n  def plus(o); o; end\n  alias + plus\nend\n"},
		{"alias [] at", "class C\n  def at(i); i; end\n  alias [] at\nend\n"},
		{"alias <=> cmp", "class C\n  def cmp(o); 0; end\n  alias <=> cmp\nend\n"},
		{"alias :/ :+", "class C\n  def +(o); o; end\n  alias :/ :+\nend\n"},
	} {
		t.Run(tc.label, func(t *testing.T) {
			if _, err := parser.Parse(tc.src); err != nil {
				t.Errorf("MRI accepts this; we refuse it: %v", err)
			}
		})
	}
}

// TestASlashIsStillARegexpWhereItShouldBe is the control for the fix above: the
// lexer state must change for an alias/undef ITEM and nowhere else, or every
// regexp literal in the program would turn into a division operator.
func TestASlashIsStillARegexpWhereItShouldBe(t *testing.T) {
	for _, tc := range []struct{ label, src string }{
		{"regexp in an assignment", "x = /re/\n"},
		{"regexp as an argument", "p [\"a\"].grep(/a/)\n"},
		{"division", "a = 6 / 2\n"},
		{"a regexp AFTER an alias of /", "class C\n  def +(o); o; end\n  alias / +\n  def m; /re/; end\nend\n"},
		{"division after an undef", "class C\n  def a; 1; end\n  undef a\n  def m; 6 / 2; end\nend\n"},
		{"regexp after an undef of /", "class C\n  def /(o); o; end\n  undef /\n  def m; /re/ =~ \"re\"; end\nend\n"},
	} {
		t.Run(tc.label, func(t *testing.T) {
			if _, err := parser.Parse(tc.src); err != nil {
				t.Errorf("the lexer state leaked past the alias/undef item: %v", err)
			}
		})
	}
}
