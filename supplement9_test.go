package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
)

// extraValidGVar covers global-variable assignment (plain and compound), which
// complements the already-supported global reads.
var extraValidGVar = []string{
	"$g = 5",               // plain global assignment
	"$g = $g + 1",          // read and assign a global
	"$count += 1",          // compound global assignment
	"$flag ||= true",       // compound (logical) global assignment
	"$a = $b = 1",          // chained global assignment
	"def f; $log = 1; end", // global assignment inside a method body
}

func TestGlobalVarAssign(t *testing.T) {
	for _, src := range extraValidGVar {
		if _, err := parser.Parse(src); err != nil {
			t.Errorf("Parse(%q) returned error: %v", src, err)
		}
	}
}

// extraValidCVar covers class-variable reads and assignment (plain and compound).
var extraValidCVar = []string{
	"class C; @@n = 0; end",       // class-variable assignment in a class body
	"class C; @@n = 0; def m; @@n; end; end", // read inside a method
	"class C; def inc; @@n += 1; end; end",    // compound class-variable assignment
	"@@top = 1",                  // class variable at the top level (Object)
	"p @@n",                      // bare class-variable read
	"@@n ||= 5",                  // compound (logical) class-variable assignment
}

func TestClassVarAssign(t *testing.T) {
	for _, src := range extraValidCVar {
		if _, err := parser.Parse(src); err != nil {
			t.Errorf("Parse(%q) returned error: %v", src, err)
		}
	}
}

// extraErrorsCVar covers the illegal bare-marker branch.
func TestClassVarErrors(t *testing.T) {
	for _, src := range []string{"p @@", "@@ = 1"} {
		if _, err := parser.Parse(src); err == nil {
			t.Errorf("expected a parse error for %q, got none", src)
		}
	}
}
