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
