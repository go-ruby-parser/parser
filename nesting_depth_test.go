// Copyright (c) the go-ruby-parser/parser authors
//
// SPDX-License-Identifier: BSD-3-Clause

package parser_test

import (
	"strings"
	"testing"

	"github.com/go-ruby-parser/parser"
)

// nested builds a source with n levels of one syntactic production. Each entry
// is a shape whose recursion was measured to reach the Go stack: array, call,
// interpolation and block all overflowed the stack fatally before the limit
// existed, at 50000-55000 levels.
func nested(kind string, n int) string {
	switch kind {
	case "array":
		return "x = " + strings.Repeat("[", n) + strings.Repeat("]", n) + "\n"
	case "hash":
		return "x = " + strings.Repeat("{a: ", n) + "1" + strings.Repeat("}", n) + "\n"
	case "paren":
		return "x = " + strings.Repeat("(", n) + "1" + strings.Repeat(")", n) + "\n"
	case "call":
		return "x = " + strings.Repeat("f(", n) + "1" + strings.Repeat(")", n) + "\n"
	case "interp":
		return "x = " + strings.Repeat(`"#{`, n) + "1" + strings.Repeat(`}"`, n) + "\n"
	case "block":
		return "x = " + strings.Repeat("f { ", n) + "1" + strings.Repeat(" }", n) + "\n"
	case "ternary":
		return "x = " + strings.Repeat("1 ? ", n) + "1" + strings.Repeat(" : 2", n) + "\n"
	}
	t := "unknown production " + kind
	panic(t)
}

var deepKinds = []string{"array", "hash", "paren", "call", "interp", "block", "ternary"}

// Source deep enough to have overflowed the Go stack must now come back as an
// ordinary error. A Go stack overflow is a FATAL runtime error rather than a
// panic, so this test cannot assert it with recover(): before the limit existed
// the process died and the test binary went with it. That is why the assertion
// is "an error, and we are still running".
func TestDeepNestingIsAnErrorNotAStackOverflow(t *testing.T) {
	for _, kind := range deepKinds {
		t.Run(kind, func(t *testing.T) {
			_, err := parser.Parse(nested(kind, 100_000))
			if err == nil {
				t.Fatalf("%s nested 100000 deep parsed without error; the depth limit did not fire", kind)
			}
			if !strings.Contains(err.Error(), "too deep") {
				t.Fatalf("%s: error is %q, want one naming the nesting limit", kind, err)
			}
		})
	}
	t.Log("reached the end, so no production took the process down")
}

// The other end, and the reason it is here: the limit counts FRAMES ENTERED,
// which for every bracket-like production is twice the source nesting, because
// parseExprOrAssign and parseTernary nest within each other and share the
// counter. A first revision of this limit set it to 16384 believing it counted
// brackets -- which capped nesting at 8192 and rejected 9500 levels of array
// literal, a shape MRI 4.0.5 ACCEPTS (`ruby -c` reports "Syntax OK").
//
// So a limit that is too low is as much a defect as no limit at all, just a
// quieter one, and this test is what would have caught it.
func TestNestingMriAcceptsIsStillAccepted(t *testing.T) {
	for _, n := range []int{1_000, 9_500, 16_000} {
		if _, err := parser.Parse(nested("array", n)); err != nil {
			t.Errorf("array nested %d deep: %v; MRI accepts this depth, so we must too", n, err)
		}
	}
}

// Ordinary source must not pay for the limit in behaviour: a program nested a
// normal amount parses, and the counter does not accumulate across the many
// expressions of one parse -- which it would if the decrement were skipped on
// any ordinary return path. 40000 statements, each with a little nesting, is
// far more total frames than the limit allows at once.
func TestDepthDoesNotAccumulateAcrossStatements(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 40_000; i++ {
		b.WriteString("x = f(g([1, {a: 2}]))\n")
	}
	if _, err := parser.Parse(b.String()); err != nil {
		t.Fatalf("40000 shallow statements: %v; the depth counter is leaking per statement", err)
	}
}

func BenchmarkParseNestedShallow(b *testing.B) {
	src := nested("call", 16)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := parser.Parse(src); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseOrdinaryProgram(b *testing.B) {
	var sb strings.Builder
	for i := 0; i < 200; i++ {
		sb.WriteString("def m(a, b = 1, *r, k: 2, **kw, &blk)\n  x = f(g([a, {k => b}])) ? a : b\n  x.map { |v| v + 1 }\nend\n")
	}
	src := sb.String()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := parser.Parse(src); err != nil {
			b.Fatal(err)
		}
	}
}
