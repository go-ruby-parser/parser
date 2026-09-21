package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// TestPercentLiteralInterpolationHidesDelimiter covers the %-literal terminator
// scan: MRI's tokadd_string (parse.y v3_4_0) leaves the scan at `#{` and hands
// the embedded expression to the main lexer, so a delimiter character occurring
// inside an interpolation cannot close the literal.
func TestPercentLiteralInterpolationHidesDelimiter(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		src  string
		want []string // the string-valued parts, in order
	}{
		{"ivar in at-delimited string", `%@hey #{@ip}@`, []string{"hey ", ""}},
		{"delimiter inside interpolation", `%@a #{"@"} b@`, []string{"a ", "@", " b"}},
		{"Q form", `%Q!a #{"!"} b!`, []string{"a ", "!", " b"}},
		{"regexp form keeps working", `%r@a #{"@"} b@`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := parser.Parse(tc.src); err != nil {
				t.Fatalf("Parse(%q) returned error: %v", tc.src, err)
			}
			if tc.want == nil {
				return
			}
			n := parseOne(t, tc.src)
			si, ok := n.(*ast.StrInterp)
			if !ok {
				t.Fatalf("Parse(%q): node = %T, want *ast.StrInterp", tc.src, n)
			}
			var got []string
			for _, part := range si.Parts {
				if s, ok := part.(*ast.StringLit); ok {
					got = append(got, s.Value)
				}
			}
			if len(got) != len(tc.want) {
				t.Fatalf("Parse(%q): %d literal parts %q, want %d", tc.src, len(got), got, len(tc.want))
			}
			for i, w := range tc.want {
				if got[i] != w {
					t.Errorf("Parse(%q): part %d = %q, want %q", tc.src, i, got[i], w)
				}
			}
		})
	}
}

// TestPercentWordListInterpolationHidesDelimiter is the %W/%I half of the same
// rule: `%W@a#{"@"}b@` is the one word `a@b`, not a list cut at the inner `@`.
func TestPercentWordListInterpolationHidesDelimiter(t *testing.T) {
	t.Parallel()
	for _, src := range []string{`p %W@a#{"@"}b@`, `p %I@a#{"@"}b@`} {
		if _, err := parser.Parse(src); err != nil {
			t.Fatalf("Parse(%q) returned error: %v", src, err)
		}
		arr, ok := commandArg(t, src, "p", 0).(*ast.ArrayLit)
		if !ok {
			t.Fatalf("Parse(%q): argument is not an *ast.ArrayLit", src)
		}
		if len(arr.Elems) != 1 {
			t.Errorf("Parse(%q): %d elements, want 1", src, len(arr.Elems))
		}
	}
}

// TestPercentEqualsDelimiter covers MRI's parse_percent ordering: at
// expression-begin every non-alphanumeric byte delimits a %-literal, so `%=…=`
// is a string; away from expression-begin the op-assign test runs first, so
// `a %= 2` stays the modulo op-assign.
func TestPercentEqualsDelimiter(t *testing.T) {
	t.Parallel()
	t.Run("literal at expression begin", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct{ src, want string }{
			{`x = %=hey=`, "hey"},
			{`x = %q=hey=`, "hey"},
		} {
			n := parseOne(t, tc.src)
			asg, ok := n.(*ast.Assign)
			if !ok {
				t.Fatalf("Parse(%q): node = %T, want *ast.Assign", tc.src, n)
			}
			s, ok := asg.Value.(*ast.StringLit)
			if !ok {
				t.Fatalf("Parse(%q): value = %T, want *ast.StringLit", tc.src, asg.Value)
			}
			if s.Value != tc.want {
				t.Errorf("Parse(%q) = %q, want %q", tc.src, s.Value, tc.want)
			}
		}
	})
	t.Run("word list at expression begin", func(t *testing.T) {
		t.Parallel()
		src := `x = %w=a b=`
		asg, ok := parseOne(t, src).(*ast.Assign)
		if !ok {
			t.Fatalf("Parse(%q): want *ast.Assign", src)
		}
		wantStrings(t, src, asg.Value, []string{"a", "b"})
	})
	t.Run("regexp at expression begin", func(t *testing.T) {
		t.Parallel()
		src := `x = %r=a=`
		asg, ok := parseOne(t, src).(*ast.Assign)
		if !ok {
			t.Fatalf("Parse(%q): want *ast.Assign", src)
		}
		if _, ok := asg.Value.(*ast.RegexpLit); !ok {
			t.Fatalf("Parse(%q): value = %T, want *ast.RegexpLit", src, asg.Value)
		}
	})
	t.Run("op-assign after a value", func(t *testing.T) {
		t.Parallel()
		src := `a %= 2`
		n := parseOne(t, src)
		if _, ok := n.(*ast.OpAssign); !ok {
			t.Fatalf("Parse(%q): node = %T, want *ast.OpAssign", src, n)
		}
	})
}

// TestPercentBackslashDelimiter covers MRI's tokadd_string loop order: the
// terminator test precedes the backslash-escape test, so a backslash used as
// the delimiter closes the literal instead of escaping the next byte. An
// escaped delimiter inside a literal delimited by something else still escapes.
func TestPercentBackslashDelimiter(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ src, want string }{
		{`x = %\hey\`, "hey"},
		{`x = %(a\)b)`, "a)b"},
		{`x = %q(a\)b)`, "a)b"},
		{`x = %(a(b)c)`, "a(b)c"},
	} {
		asg, ok := parseOne(t, tc.src).(*ast.Assign)
		if !ok {
			t.Fatalf("Parse(%q): want *ast.Assign", tc.src)
		}
		s, ok := asg.Value.(*ast.StringLit)
		if !ok {
			t.Fatalf("Parse(%q): value = %T, want *ast.StringLit", tc.src, asg.Value)
		}
		if s.Value != tc.want {
			t.Errorf("Parse(%q) = %q, want %q", tc.src, s.Value, tc.want)
		}
	}
}
