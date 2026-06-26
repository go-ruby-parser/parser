package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

func TestKeywordMethodNames(t *testing.T) {
	for _, tc := range []struct{ src, name string }{
		{"def do; end", "do"},
		{"def then; end", "then"},
		{"def in; end", "in"},
		{"def class; end", "class"},
		{"def begin; end", "begin"},
		{"def if; end", "if"},
		{"def while; end", "while"},
		{"def nil; end", "nil"},
		{"def end; end", "end"},
	} {
		m, ok := parseOne(t, tc.src).(*ast.MethodDef)
		if !ok {
			t.Errorf("Parse(%q): node = %T, want *ast.MethodDef", tc.src, parseOne(t, tc.src))
			continue
		}
		if m.Name != tc.name {
			t.Errorf("Parse(%q): Name = %q, want %q", tc.src, m.Name, tc.name)
		}
	}
}

func TestKeywordMethodSingleton(t *testing.T) {
	m := parseOne(t, "def self.do; end").(*ast.MethodDef)
	if !m.Singleton || m.Name != "do" {
		t.Errorf("got Singleton=%v Name=%q, want true/do", m.Singleton, m.Name)
	}
}

func TestKeywordMethodOnReceiver(t *testing.T) {
	m := parseOne(t, "def obj.then; end").(*ast.MethodDef)
	if m.Name != "then" {
		t.Errorf("Name = %q, want then", m.Name)
	}
	if _, ok := m.Recv.(*ast.VarRef); !ok {
		t.Errorf("Recv = %T, want *ast.VarRef", m.Recv)
	}
}

// TestNoPanicOnUnterminatedInterpolation guards the parser against panicking on
// malformed input: an unterminated string interpolation must yield a clean
// error, never a runtime panic (the EOF-clamped cursor makes this safe).
func TestNoPanicOnUnterminatedInterpolation(t *testing.T) {
	for _, src := range []string{
		`"#{`,
		`"#{a`,
		`a("#{b`,
		`"#{1 +`,
		`"#{x}#{`,
	} {
		assertCleanError(t, src)
	}
}

// TestNoPanicOnTruncatedInput exercises a broad set of truncated/garbled inputs;
// each must return a clean parse error rather than panic.
func TestNoPanicOnTruncatedInput(t *testing.T) {
	for _, src := range []string{
		`:"#{x`,
		`def f(`,
		`class`,
		`[1,`,
		`{a:`,
		`(`,
		`foo(`,
		`if`,
	} {
		assertCleanError(t, src)
	}
}

// assertCleanError parses src expecting a returned error and explicitly fails if
// Parse panics instead of returning.
func assertCleanError(t *testing.T, src string) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Parse(%q) panicked: %v", src, r)
		}
	}()
	if _, err := parser.Parse(src); err == nil {
		t.Errorf("Parse(%q) = no error, want a clean parse error", src)
	}
}
