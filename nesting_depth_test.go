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
	case "assign":
		return "x = " + strings.Repeat("y = ", n) + "1\n"
	}
	t := "unknown production " + kind
	panic(t)
}

// deepKinds pairs each production with a depth comfortably past the limit FOR
// THAT PRODUCTION. The depths differ because the limit counts frames entered,
// not brackets nested, and the ratio is not the same everywhere: every
// bracket-like production enters parseExprOrAssign and parseTernary in turn and
// so runs at 2x the source nesting, while a ternary chain re-enters only
// parseTernary and runs at 1x. Measured at nesting 2000: array 4001,
// hash/paren/call/interp/block 4003, ternary 2003.
//
// A single depth of 20000 for all of them looked right and was not: it is 40000
// frames for the first six and only 20001 for the ternary, which sits UNDER the
// 32768 ceiling, so the ternary case silently stopped testing anything. This
// test caught that, which is the argument for keeping the depths explicit
// rather than computing one from a constant.
var deepKinds = []struct {
	kind  string
	depth int
}{
	{"array", 20_000},
	{"hash", 20_000},
	{"paren", 20_000},
	{"call", 20_000},
	{"interp", 20_000},
	{"block", 20_000},
	{"ternary", 35_000},
	// An assignment chain is the one shape that recurses through
	// parseExprOrAssign ALONE -- the other eleven probed shapes all cross the
	// limit inside parseTernary. Without this row the check in
	// parseExprOrAssign is never exercised, the coverage gate says so, and
	// deleting it as dead code would leave `y = `*N unguarded. Ratio 1.0, so
	// it needs a depth past the frame limit like the ternary does.
	{"assign", 40_000},
}

// Source deep enough to have overflowed the Go stack must now come back as an
// ordinary error. A Go stack overflow is a FATAL runtime error rather than a
// panic, so this test cannot assert it with recover(): before the limit existed
// the process died and the test binary went with it. That is why the assertion
// is "an error, and we are still running".
// The depth is 20000 levels and not 100000. 20000 is already past the limit for
// every production here -- the ceiling is 32768 FRAMES, which is 16384 levels of
// any bracket production and 32768 of a ternary chain -- so a deeper input
// proves nothing more and costs real time: CI runs this with -race AND coverage
// instrumentation under go test's 10-minute default, and a first revision at
// 100000 took 2.8s locally, 33s locally under -race alone, and timed out the
// whole package on all three OS lanes at 600s. Deep enough to fire is the
// requirement; deeper is only slower.
func TestDeepNestingIsAnErrorNotAStackOverflow(t *testing.T) {
	for _, tc := range deepKinds {
		t.Run(tc.kind, func(t *testing.T) {
			_, err := parser.Parse(nested(tc.kind, tc.depth))
			if err == nil {
				t.Fatalf("%s nested %d deep parsed without error; the depth limit did not fire",
					tc.kind, tc.depth)
			}
			if !strings.Contains(err.Error(), "too deep") {
				t.Fatalf("%s: error is %q, want one naming the nesting limit", tc.kind, err)
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
// any ordinary return path. 10000 statements enter parseExprOrAssign tens of
// thousands of times in total, far past the 32768 ceiling, so a leaked
// decrement fails this; it is kept at 10000 rather than higher because this
// runs under -race and coverage in CI.
func TestDepthDoesNotAccumulateAcrossStatements(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 10_000; i++ {
		b.WriteString("x = f(g([1, {a: 2}]))\n")
	}
	if _, err := parser.Parse(b.String()); err != nil {
		t.Fatalf("10000 shallow statements: %v; the depth counter is leaking per statement", err)
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
