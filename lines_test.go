// Copyright (c) the go-ruby-parser authors
//
// SPDX-License-Identifier: BSD-3-Clause

package parser

import (
	"testing"

	"github.com/go-ruby-parser/parser/ast"
)

// lineOf walks the statement list and returns the recorded line of the nth
// top-level statement.
func lineOf(t *testing.T, lines map[ast.Node]int, n ast.Node) int {
	t.Helper()
	l, ok := lines[n]
	if !ok {
		t.Fatalf("no line recorded for %T", n)
	}
	return l
}

// TestParsedLinesTopLevel pins the line of each top-level statement,
// including the blank lines and comments MRI skips over.
func TestParsedLinesTopLevel(t *testing.T) {
	src := "a = 1\n\n# comment\nb = 2\nc = 3; d = 4\n"
	prog, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	lines := prog.Lines
	want := []int{1, 4, 5, 5}
	if len(prog.Body) != len(want) {
		t.Fatalf("got %d statements, want %d", len(prog.Body), len(want))
	}
	for i, w := range want {
		if got := lineOf(t, lines, prog.Body[i]); got != w {
			t.Errorf("statement %d: line %d, want %d", i, got, w)
		}
	}
}

// TestParsedLinesNested checks that a body nested inside a def, a class and
// a block is stamped with its own line, not the enclosing construct's.
func TestParsedLinesNested(t *testing.T) {
	src := "def foo\n  bar\nend\nclass C\n  def baz\n    qux\n  end\nend\n[1].each do |v|\n  p v\nend\n"
	prog, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	lines := prog.Lines
	def, ok := prog.Body[0].(*ast.MethodDef)
	if !ok {
		t.Fatalf("statement 0 is %T, want *ast.MethodDef", prog.Body[0])
	}
	if got := lineOf(t, lines, def); got != 1 {
		t.Errorf("def line %d, want 1", got)
	}
	if got := lineOf(t, lines, def.Body[0]); got != 2 {
		t.Errorf("def body line %d, want 2", got)
	}

	cls, ok := prog.Body[1].(*ast.ClassDef)
	if !ok {
		t.Fatalf("statement 1 is %T, want *ast.ClassDef", prog.Body[1])
	}
	if got := lineOf(t, lines, cls); got != 4 {
		t.Errorf("class line %d, want 4", got)
	}
	inner, ok := cls.Body[0].(*ast.MethodDef)
	if !ok {
		t.Fatalf("class body 0 is %T, want *ast.MethodDef", cls.Body[0])
	}
	if got := lineOf(t, lines, inner); got != 5 {
		t.Errorf("inner def line %d, want 5", got)
	}
	if got := lineOf(t, lines, inner.Body[0]); got != 6 {
		t.Errorf("inner def body line %d, want 6", got)
	}

	call, ok := prog.Body[2].(*ast.Call)
	if !ok {
		t.Fatalf("statement 2 is %T, want *ast.Call", prog.Body[2])
	}
	if got := lineOf(t, lines, call); got != 9 {
		t.Errorf("each call line %d, want 9", got)
	}
	if got := lineOf(t, lines, call.Block.Body[0]); got != 10 {
		t.Errorf("block body line %d, want 10", got)
	}
}

// TestParsedLinesFirstWriteWins covers the pass-through construct: `begin;
// foo; end` hands foo's node straight back, and the stamp must stay foo's line
// (the inner one, written first) rather than the begin's.
func TestParsedLinesFirstWriteWins(t *testing.T) {
	src := "x = 0\nbegin\n  foo\nend\n"
	prog, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	lines := prog.Lines
	begin, ok := prog.Body[1].(*ast.Begin)
	if !ok {
		t.Fatalf("statement 1 is %T, want *ast.Begin", prog.Body[1])
	}
	if got := lineOf(t, lines, begin.Body[0]); got != 3 {
		t.Errorf("begin body line %d, want 3", got)
	}
}

// TestParsedLinesMultiline pins the rule a consumer depends on: a statement
// spanning several lines carries the line it STARTED on, which is the line MRI
// attributes to the call node too.
func TestParsedLinesMultiline(t *testing.T) {
	src := "foo(1,\n    2,\n    3)\n"
	prog, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	lines := prog.Lines
	if got := lineOf(t, lines, prog.Body[0]); got != 1 {
		t.Errorf("multi-line call line %d, want 1", got)
	}
}

// TestParsedLinesModifier checks a statement rewritten by applyModifiers:
// `foo if bar` becomes an *ast.If, and that If is what carries the line.
func TestParsedLinesModifier(t *testing.T) {
	src := "\n\nfoo if bar\n"
	prog, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	lines := prog.Lines
	if _, ok := prog.Body[0].(*ast.If); !ok {
		t.Fatalf("statement 0 is %T, want *ast.If", prog.Body[0])
	}
	if got := lineOf(t, lines, prog.Body[0]); got != 3 {
		t.Errorf("modifier-if line %d, want 3", got)
	}
}

// TestParsedLinesKeywordStatements covers the non-default arms of
// parseStatement1 (return/break/next/retry/alias/undef), each of which is
// stamped through the same wrapper.
func TestParsedLinesKeywordStatements(t *testing.T) {
	src := "def m\n  return 1\nend\n[1].each do\n  break\nend\n[1].each do\n  next\nend\nbegin\n  retry\nrescue\nend\nalias a b\nundef c\n"
	prog, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	lines := prog.Lines
	def := prog.Body[0].(*ast.MethodDef)
	if got := lineOf(t, lines, def.Body[0]); got != 2 {
		t.Errorf("return line %d, want 2", got)
	}
	brk := prog.Body[1].(*ast.Call)
	if got := lineOf(t, lines, brk.Block.Body[0]); got != 5 {
		t.Errorf("break line %d, want 5", got)
	}
	nxt := prog.Body[2].(*ast.Call)
	if got := lineOf(t, lines, nxt.Block.Body[0]); got != 8 {
		t.Errorf("next line %d, want 8", got)
	}
	bg := prog.Body[3].(*ast.Begin)
	if got := lineOf(t, lines, bg.Body[0]); got != 11 {
		t.Errorf("retry line %d, want 11", got)
	}
	if got := lineOf(t, lines, prog.Body[4]); got != 14 {
		t.Errorf("alias line %d, want 14", got)
	}
	if got := lineOf(t, lines, prog.Body[5]); got != 15 {
		t.Errorf("undef line %d, want 15", got)
	}
}

// TestParsedLinesError checks that a malformed input yields no program and no
// map, like Parse's error contract.
func TestParsedLinesError(t *testing.T) {
	prog, err := Parse("def\n")
	if err == nil {
		t.Fatal("want a parse error")
	}
	if prog != nil {
		t.Errorf("got prog=%v, want nil", prog)
	}
}

// TestParsedLinesEmptyProgram pins that an empty source still yields a usable
// (non-nil, empty) map rather than one a consumer must nil-check.
func TestParsedLinesEmptyProgram(t *testing.T) {
	prog, err := Parse("\n# just a comment\n")
	if err != nil {
		t.Fatal(err)
	}
	if prog.Lines == nil {
		t.Error("Lines is nil for an empty program")
	}
	if len(prog.Lines) != 0 {
		t.Errorf("Lines has %d entries, want 0", len(prog.Lines))
	}
}

// TestParsedLinesInternalError drives the recover path of the shared parse
// core through ParsedLines, so the nil-map assignment there is exercised.
func TestParsedLinesInternalError(t *testing.T) {
	parseHook = func() { panic("boom") }
	defer func() { parseHook = nil }()
	prog, err := Parse("a = 1\n")
	if err == nil {
		t.Fatal("want an internal parse error")
	}
	if prog != nil {
		t.Errorf("got prog=%v, want nil", prog)
	}
}
