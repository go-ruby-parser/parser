<p align="center"><img src="https://raw.githubusercontent.com/go-ruby-parser/brand/main/social/go-ruby-parser.png" alt="go-ruby-parser/parser" width="720"></p>

# parser — go-ruby-parser

[![ci](https://github.com/go-ruby-parser/parser/actions/workflows/ci.yml/badge.svg)](https://github.com/go-ruby-parser/parser/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26.4%2B-00ADD8)](https://go.dev/dl/)

**A pure-Go (CGO=0) Ruby front-end** — a lexer, a recursive-descent + Pratt
parser, and an AST for Ruby source. No cgo, no Prism, no shelling out to Ruby:
the single thing the Go ecosystem lacked for building Ruby tooling (linters,
formatters, analysers, doc generators, LSP servers, transpilers) in pure Go.

It was extracted from the [go-embedded-ruby](https://github.com/go-embedded-ruby/ruby)
interpreter — which now consumes it — and is developed test-first against MRI
Ruby 4.0.5.

Current release: **v0.4.0**. (The interpreter currently pins v0.3.0.) There are no
GitHub release notes; the tags are the record.

## Install

```sh
go get github.com/go-ruby-parser/parser
```

## Usage

```go
import "github.com/go-ruby-parser/parser"

prog, err := parser.Parse(`
def fib(n)
  n < 2 ? n : fib(n - 1) + fib(n - 2)
end
puts fib(20)
`)
if err != nil {
    // err is a parse error with a line number
}
// prog is an *ast.Program; walk prog.Body ([]ast.Node).
```

The AST node types live in [`github.com/go-ruby-parser/parser/ast`](ast); the
token kinds in [`.../token`](token); the stateful lexer in [`.../lexer`](lexer).

## What it parses

A broad, practical subset of Ruby 4.0, all differential-tested against MRI:

- **Literals:** integers (with `Bignum`/arbitrary precision, radix `0x`/`0o`/
  `0b`/`0d`, underscores), floats (incl. **scientific `1.5e3`**), strings
  (double- and **single-quoted**, interpolation, heredocs `<<`/`<<-`/`<<~`,
  `%q`/`%Q` literals, the `\a`/`\b`/`\v`/`\f`/`\s`/`\n`/`\t`/`\r`/`\e`/`\0`
  escapes), symbols (incl. quoted/operator), `%w`/`%i`/`%W`/`%I` arrays, arrays,
  hashes (incl. the `{x:}` value-shorthand), ranges (incl. beginless/endless),
  regexps (`/re/imx`), `true`/`false`/`nil`.
- **Operators:** arithmetic, comparison/`<=>`, `==`/`===`, bitwise/shift,
  `&&`/`||`/`and`/`or`/`not`, ternary, `::` scope, safe navigation `&.`,
  compound assignment (`+=`, `-=`, `*=`, `/=`, `%=`, `<<=`, `||=`, `&&=`).
- **Control flow:** `if`/`unless`/`while`/`until` (block and modifier),
  `case`/`when`, `case`/`in` **pattern matching** (array/find/hash/pin/
  alternative/range patterns, guards, one-line `=>`/`in`), `begin`/`rescue`/
  `else`/`ensure`/`retry`, `break`/`next`/`return`, `loop`.
- **Methods/blocks:** required/optional/`*splat`/keyword/`**rest`/`&block`
  params, endless methods (`def f = expr`), setters, operator/`[]`/`[]=` method
  names, **operator-method calls** (`1.+(2)`), `{ }` / `do…end` blocks,
  `(a, b)` destructuring group params, stabby lambdas `->(){}`, numbered params
  (`_1`) and `it`, `yield`, `super`, **multiple-value `return a, b`**.
- **Classes/modules/metaprogramming:** `class`/`module`, inheritance, `@ivars`,
  **`@@class variables`**, constants, singleton method defs
  (`def self.foo`/`def obj.foo`/`def Const.foo`), **global-variable assignment**
  (`$g = …`), multiple assignment / destructuring, **adjacent string-literal
  concatenation** (`"a" "b"`).

## Performance

Fast enough for tooling, and **not** faster than MRI's own C parser. Measured on
2026-09-26, darwin/arm64, against MRI 4.0.5: 46 real CRuby 4.0.5 stdlib files
(768 KB) that **both** engines accept, each parsed 20 times in-process.

| | per file | vs go-ruby-parser |
| --- | ---: | --- |
| **go-ruby-parser `Parse`** (full Go AST) | **0.45 ms** | — |
| `RubyVM::AbstractSyntaxTree.parse` (MRI's C parser) | 0.29 ms | **1.6× faster than us** |
| `Ripper.sexp` | 0.86 ms | 1.9× slower than us |
| | | |
| **go-ruby-parser `lexer.Tokenize`** | **0.36 ms** | — |
| `Ripper.lex` | 2.16 ms | 6.0× slower than us |

So: comfortably quicker than `Ripper`, the thing most Ruby tooling actually uses,
and roughly **1.6× slower** than the C parser built into CRuby. The token counts are
not comparable between the two lexers (82 k vs 125 k over the same corpus — `Ripper`
emits whitespace and comment tokens), so only the per-file times are.

> **Note.** Earlier revisions of this file claimed go-ruby-parser *beat* MRI's C
> parser (2.1× on stdlib files, ~6× on `Ripper.sexp`, 24–30× on `Ripper.lex`).
> **None of those reproduced** on the measurement above. The corpus and host differ
> from the original run, so treat the table above as this host's numbers rather than
> a refutation of the method — but do not quote the old figures.

Methodology, the full parity tables and the allocation hotspots are in
[`BENCHMARKS.md`](BENCHMARKS.md); reproduce with
[`benchmarks/run.sh`](benchmarks) (an isolated module, outside the coverage gate).

## Known limitations

As of **v0.4.0**, the three limitations this section used to list all parse. Verified
against v0.4.0:

```go
parser.Parse("foo a: 1")                        // paren-less command call with kwargs — OK
parser.Parse("[1].each { |a = 1| a }")          // default block parameter — OK
parser.Parse("case x\nin Point(a, b)\n  a\nend") // positional find-pattern — OK
```

What still does not parse:

- **`BEGIN { }` and `END { }` blocks** — `parse error: unexpected "{" after statement`.

A broader note on completeness: of 47 top-level CRuby 4.0.5 stdlib files that MRI
accepts, go-ruby-parser accepts **46**. The one refusal is `mkmf.rb`
(`expected CONST, got "log_open"`). The parser is deliberately
**under**-permissive rather than over-permissive: it aims never to accept Ruby that
MRI rejects.

## License

BSD-3-Clause © the go-ruby-parser/parser authors.

## WebAssembly

Being pure Go (CGO=0), this library also compiles to **WebAssembly** — both
`GOOS=js GOARCH=wasm` (browser / Node.js) and `GOOS=wasip1 GOARCH=wasm` (WASI).
CI builds both targets on every push, alongside the six 64-bit native/qemu arches.

```sh
GOOS=js     GOARCH=wasm go build ./...   # browser / Node
GOOS=wasip1 GOARCH=wasm go build ./...   # WASI (wasmtime, wasmer, wasmedge, …)
```
