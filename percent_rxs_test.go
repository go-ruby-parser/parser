package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

func TestPercentRegexp(t *testing.T) {
	rx, ok := parseOne(t, "%r{ab}i").(*ast.RegexpLit)
	if !ok {
		t.Fatalf("node = %T, want *ast.RegexpLit", parseOne(t, "%r{ab}i"))
	}
	if rx.Source != "ab" {
		t.Errorf("Source = %q, want ab", rx.Source)
	}
	if rx.Flags != "i" {
		t.Errorf("Flags = %q, want i", rx.Flags)
	}
}

func TestPercentRegexpParenDelim(t *testing.T) {
	rx := parseOne(t, "%r(foo)").(*ast.RegexpLit)
	if rx.Source != "foo" || rx.Flags != "" {
		t.Errorf("got %q/%q, want foo/(empty)", rx.Source, rx.Flags)
	}
}

func TestPercentRegexpBracketFlag(t *testing.T) {
	rx := parseOne(t, "%r[bar]m").(*ast.RegexpLit)
	if rx.Source != "bar" || rx.Flags != "m" {
		t.Errorf("got %q/%q, want bar/m", rx.Source, rx.Flags)
	}
}

func TestPercentRegexpKeepsInterpolationRaw(t *testing.T) {
	// Matching the existing /…/ behaviour, interpolation markers stay raw source.
	rx := parseOne(t, "%r{a#{x}b}").(*ast.RegexpLit)
	if rx.Source != "a#{x}b" {
		t.Errorf("Source = %q, want a#{x}b", rx.Source)
	}
}

func TestPercentRegexpNoFlags(t *testing.T) {
	rx := parseOne(t, "%r{ab}").(*ast.RegexpLit)
	if rx.Flags != "" {
		t.Errorf("Flags = %q, want empty", rx.Flags)
	}
}

func TestPercentXString(t *testing.T) {
	xs, ok := parseOne(t, "%x{ls}").(*ast.XStr)
	if !ok {
		t.Fatalf("node = %T, want *ast.XStr", parseOne(t, "%x{ls}"))
	}
	if xs.Command != "ls" {
		t.Errorf("Command = %q, want ls", xs.Command)
	}
}

func TestPercentXStringBracket(t *testing.T) {
	xs := parseOne(t, "%x[cmd]").(*ast.XStr)
	if xs.Command != "cmd" {
		t.Errorf("Command = %q, want cmd", xs.Command)
	}
}

func TestPercentSymbol(t *testing.T) {
	sym, ok := parseOne(t, "%s{foo}").(*ast.SymbolLit)
	if !ok {
		t.Fatalf("node = %T, want *ast.SymbolLit", parseOne(t, "%s{foo}"))
	}
	if sym.Name != "foo" {
		t.Errorf("Name = %q, want foo", sym.Name)
	}
}

func TestPercentSymbolWithSpace(t *testing.T) {
	sym := parseOne(t, "%s(with space)").(*ast.SymbolLit)
	if sym.Name != "with space" {
		t.Errorf("Name = %q, want 'with space'", sym.Name)
	}
}

func TestPercentRXSNestedDelimiters(t *testing.T) {
	rx := parseOne(t, "%r{a{b}c}").(*ast.RegexpLit)
	if rx.Source != "a{b}c" {
		t.Errorf("Source = %q, want a{b}c", rx.Source)
	}
}

func TestPercentRXSEscapedDelimiter(t *testing.T) {
	// An escaped closing delimiter does not terminate the literal; %x drops the
	// backslash for the escaped delimiter.
	xs := parseOne(t, "%x{a\\}b}").(*ast.XStr)
	if xs.Command != "a}b" {
		t.Errorf("Command = %q, want a}b", xs.Command)
	}
}

func TestPercentRegexpEscapedDelimiter(t *testing.T) {
	// MRI keeps the backslash in a %r source for an escaped delimiter:
	// `%r{a\}b}.source == "a\\}b"`. The escape only stops `}` from closing it.
	rx := parseOne(t, "%r{a\\}b}").(*ast.RegexpLit)
	if rx.Source != "a\\}b" {
		t.Errorf("Source = %q, want a\\}b", rx.Source)
	}
}

func TestPercentRegexpKeepsOtherEscape(t *testing.T) {
	// A non-delimiter escape is kept verbatim for the regexp engine.
	rx := parseOne(t, "%r{a\\db}").(*ast.RegexpLit)
	if rx.Source != "a\\db" {
		t.Errorf("Source = %q, want a\\db", rx.Source)
	}
}

func TestPercentXStringKeepsNonDelimiterEscape(t *testing.T) {
	// A non-delimiter escape in %x/%s is kept verbatim (the backslash stays).
	xs := parseOne(t, "%x{a\\db}").(*ast.XStr)
	if xs.Command != "a\\db" {
		t.Errorf("Command = %q, want a\\db", xs.Command)
	}
	sym := parseOne(t, "%s{a\\db}").(*ast.SymbolLit)
	if sym.Name != "a\\db" {
		t.Errorf("Name = %q, want a\\db", sym.Name)
	}
}

func TestPercentRXSAsCommandArg(t *testing.T) {
	call := parseOne(t, "puts %x{ls}").(*ast.Call)
	if _, ok := call.Args[0].(*ast.XStr); !ok {
		t.Errorf("arg = %T, want *ast.XStr", call.Args[0])
	}
}

func TestPercentRXSUnterminated(t *testing.T) {
	for _, src := range []string{"%r{ab", "%x{ls", "%s{foo"} {
		if _, err := parser.Parse(src); err == nil {
			t.Errorf("Parse(%q) = no error, want a clean error", src)
		}
	}
}

func TestPercentRXSTrailingBackslash(t *testing.T) {
	// A backslash at end-of-input inside the literal keeps the lone backslash and
	// ends the token rather than reading past EOF.
	rx := parseOne(t, "%r{a\\").(*ast.RegexpLit)
	if rx.Source != "a\\" {
		t.Errorf("Source = %q, want a\\", rx.Source)
	}
	xs := parseOne(t, "%x{a\\").(*ast.XStr)
	if xs.Command != "a\\" {
		t.Errorf("Command = %q, want a\\", xs.Command)
	}
}

func TestPercentRXSValidPrograms(t *testing.T) {
	for _, src := range []string{"%r{ab}i", "%x{ls}", "%s{foo}", "%r(foo)", "%r[bar]m", "%s(with space)", "%x[cmd]"} {
		if _, err := parser.Parse(src); err != nil {
			t.Errorf("Parse(%q) returned error: %v", src, err)
		}
	}
}
