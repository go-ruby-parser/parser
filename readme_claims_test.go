package parser_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"

	"github.com/go-ruby-parser/parser"
)

// TestReadmeClaimsStillHold makes the README's "Known limitations" section
// executable. That section listed three constructs as fixed and two --
// `BEGIN { }` and `END { }` -- as still failing, and claimed 46 of 47 CRuby
// stdlib files parsed with `mkmf.rb` the one refusal.
//
// Measured on 2026-10-04 at v0.11.0, every one of those was obsolete: BEGIN and
// END parse, `def self::name` (the construct mkmf.rb actually tripped on)
// parses, and the stdlib sweep is 47 of 47. A section naming limitations that
// no longer exist is worse than a stale number -- it sends a reader round a
// bug that is fixed.
//
// So the constructs the README names are pinned here. A future change that
// breaks one fails this rather than quietly contradicting the page.
func TestReadmeClaimsStillHold(t *testing.T) {
	for _, tc := range []struct{ label, src string }{
		{"paren-less command call with kwargs", "foo a: 1"},
		{"default block parameter", "[1].each { |a = 1| a }"},
		{"positional find-pattern", "case x\nin Point(a, b)\n  a\nend"},
		{"BEGIN block", "BEGIN { puts 1 }\n"},
		{"END block", "END { puts 1 }\n"},
		{"def self::name", "class C\n  def self::log_open; 1; end\nend\n"},
	} {
		t.Run(tc.label, func(t *testing.T) {
			if _, err := parser.Parse(tc.src); err != nil {
				t.Errorf("README says this parses; it does not: %v", err)
			}
		})
	}
}

// TestUnderPermissiveNotOver is the design claim the README makes and the one
// that matters: the parser aims never to accept Ruby that MRI rejects. The row
// below is the discriminating one -- MRI refuses BEGIN inside a method body,
// and so must this.
func TestUnderPermissiveNotOver(t *testing.T) {
	const src = "def f; BEGIN { 1 }; end\n"
	if _, err := parser.Parse(src); err == nil {
		t.Errorf("MRI refuses %q; accepting it is over-permissive", src)
	}
}

// TestCRubyStdlibTopLevelParses sweeps the top-level .rb files of a real CRuby
// 4.0.5 installation, which is where the README's 47-of-47 figure comes from.
//
// It SKIPS when no such tree is present, so CI (which has no CRuby) is not
// gated on a machine-specific path -- the figure in the README is explicitly a
// dated local measurement for that reason. When the tree IS there, this is the
// instrument that produced the number.
func TestCRubyStdlibTopLevelParses(t *testing.T) {
	dir := os.Getenv("RUBYLIBDIR")
	if dir == "" {
		out, err := exec.Command("ruby", "-e", `print RbConfig::CONFIG["rubylibdir"]`).Output()
		if err != nil {
			t.Skip("no ruby on PATH; the README's stdlib figure is a dated local measurement")
		}
		dir = string(out)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("cannot read %s: %v", dir, err)
	}
	var files []string
	for _, e := range ents {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".rb" {
			files = append(files, e.Name())
		}
	}
	if len(files) == 0 {
		t.Skipf("%s holds no top-level .rb files", dir)
	}
	sort.Strings(files)
	var refused []string
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			continue
		}
		if _, err := parser.Parse(string(b)); err != nil {
			refused = append(refused, f+": "+err.Error())
		}
	}
	t.Logf("%s: %d top-level .rb files, %d refused", dir, len(files), len(refused))
	for _, r := range refused {
		t.Errorf("refused %s", r)
	}
}
