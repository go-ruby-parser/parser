package parser

import (
	"strings"
	"testing"
)

// TestParseRecoversInternalPanic verifies the never-panic contract: an
// unexpected internal panic (not a parseError) is converted into a clean parse
// error instead of propagating to the caller. The parseHook seam injects such a
// panic; outside tests it is nil.
func TestParseRecoversInternalPanic(t *testing.T) {
	defer func() { parseHook = nil }()
	parseHook = func() { panic("boom") }

	prog, err := Parse("1 + 1")
	if prog != nil {
		t.Errorf("Parse: prog = %+v, want nil on internal panic", prog)
	}
	if err == nil {
		t.Fatal("Parse: err is nil, want an internal parser error")
	}
	if !strings.Contains(err.Error(), "internal parser error") {
		t.Errorf("Parse: err = %q, want it to mention an internal parser error", err.Error())
	}
}

// TestParseRecoversParseError confirms the ordinary parse-error path still flows
// through the recover (a genuine syntax error remains a clean parseError).
func TestParseRecoversParseError(t *testing.T) {
	if _, err := Parse("def "); err == nil {
		t.Fatal("Parse(def ): want a parse error, got nil")
	}
}
