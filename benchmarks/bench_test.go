// Package benchmarks measures parse and lex throughput of github.com/go-ruby-parser/parser
// against MRI's reference C parser (Ripper / RubyVM::AbstractSyntaxTree). The Go
// side lives here; the Ruby side lives in mri_bench.rb. Both consume the SAME
// corpus so the numbers are comparable.
//
// Two corpora:
//
//   - snippets — the project's own harvested Ruby snippets, embedded in the
//     binary under corpus/snippets (always available, incl. in CI).
//   - stdlib   — real CRuby standard-library .rb files, discovered at RUN TIME
//     from the live Ruby install (its rubylibdir). Only the files listed in
//     corpus/stdlib_manifest.txt are used — the exact set both this parser and
//     MRI accept, so the comparison stays apples-to-apples and reproducible.
//     These third-party files are NOT vendored into the repo; the stdlib
//     benchmarks skip cleanly when Ruby is absent (e.g. CI).
//
// Run: go test -bench=. -benchmem -count=6 ./...   (use benchmarks/run.sh)
package benchmarks

import (
	"bufio"
	"embed"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/lexer"
)

//go:embed corpus/snippets corpus/stdlib_manifest.txt
var corpusFS embed.FS

// loadSnippets reads the embedded snippet corpus once.
func loadSnippets(tb testing.TB) (srcs []string, bytes int) {
	tb.Helper()
	err := fs.WalkDir(corpusFS, "corpus/snippets", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".rb") {
			return err
		}
		b, e := corpusFS.ReadFile(p)
		if e != nil {
			return e
		}
		srcs = append(srcs, string(b))
		bytes += len(b)
		return nil
	})
	if err != nil {
		tb.Fatalf("load snippets: %v", err)
	}
	if len(srcs) == 0 {
		tb.Fatal("snippet corpus is empty")
	}
	return srcs, bytes
}

// rubyLibDir asks the live Ruby for its standard-library directory, or returns
// "" if ruby is not on PATH.
func rubyLibDir() string {
	out, err := exec.Command("ruby", "-e", `print RbConfig::CONFIG["rubylibdir"]`).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// loadStdlib reads the manifest-listed stdlib files from the live Ruby install.
// It skips the benchmark (returns ok=false) when Ruby or any listed file is
// unavailable, so this never fails in a Ruby-less environment.
func loadStdlib(tb testing.TB) (srcs []string, bytes int, ok bool) {
	tb.Helper()
	libdir := rubyLibDir()
	if libdir == "" {
		return nil, 0, false
	}
	mf, err := corpusFS.Open("corpus/stdlib_manifest.txt")
	if err != nil {
		tb.Fatalf("open manifest: %v", err)
	}
	defer mf.Close()
	sc := bufio.NewScanner(mf)
	for sc.Scan() {
		rel := strings.TrimSpace(sc.Text())
		if rel == "" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(libdir, rel))
		if err != nil {
			// A different Ruby version may not ship this exact file; skip the
			// whole stdlib benchmark rather than time a partial corpus.
			return nil, 0, false
		}
		srcs = append(srcs, string(b))
		bytes += len(b)
	}
	return srcs, bytes, len(srcs) > 0
}

func benchParseCorpus(b *testing.B, srcs []string, total int) {
	b.SetBytes(int64(total))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, s := range srcs {
			if _, err := parser.Parse(s); err != nil {
				b.Fatalf("parse error: %v", err)
			}
		}
	}
	b.StopTimer()
	reportThroughput(b, srcs, total)
}

func benchLexCorpus(b *testing.B, srcs []string, total int) {
	b.SetBytes(int64(total))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, s := range srcs {
			_ = lexer.New(s).Tokenize()
		}
	}
	b.StopTimer()
	reportThroughput(b, srcs, total)
}

// reportThroughput attaches KB/s and ns/file metrics (KB = 1024 bytes).
func reportThroughput(b *testing.B, srcs []string, total int) {
	perPass := b.Elapsed().Seconds() / float64(b.N)
	files := float64(b.N) * float64(len(srcs))
	b.ReportMetric(float64(total)/1024.0/perPass, "KB/s")
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/files, "ns/file")
}

func BenchmarkParseSnippets(b *testing.B) {
	srcs, total := loadSnippets(b)
	benchParseCorpus(b, srcs, total)
}

func BenchmarkLexSnippets(b *testing.B) {
	srcs, total := loadSnippets(b)
	benchLexCorpus(b, srcs, total)
}

func BenchmarkParseStdlib(b *testing.B) {
	srcs, total, ok := loadStdlib(b)
	if !ok {
		b.Skip("Ruby stdlib corpus unavailable (no ruby on PATH or version mismatch)")
	}
	benchParseCorpus(b, srcs, total)
}

func BenchmarkLexStdlib(b *testing.B) {
	srcs, total, ok := loadStdlib(b)
	if !ok {
		b.Skip("Ruby stdlib corpus unavailable (no ruby on PATH or version mismatch)")
	}
	benchLexCorpus(b, srcs, total)
}
