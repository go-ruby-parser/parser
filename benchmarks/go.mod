// The benchmarks live in their own module so the parser's 100%-coverage CI
// gate (which runs `go test ./...` from the repo root over `go list ./...`)
// never sees this package: a nested module is excluded from the parent's
// package list. Run them explicitly with `go test -bench=. ./benchmarks/...`
// from inside this directory, or via `benchmarks/run.sh`.
module github.com/go-ruby-parser/parser/benchmarks

go 1.26.4

require github.com/go-ruby-parser/parser v0.0.0

replace github.com/go-ruby-parser/parser => ../
