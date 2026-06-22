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

On the both-accept corpus (the 615 harvested snippets + 110 real CRuby-4.0.5
stdlib files **both** engines parse), go-ruby-parser **beats MRI's reference C
parser**: it parses **1.5× faster than `RubyVM::AbstractSyntaxTree.parse` on
small snippets and 2.1× faster on real stdlib files**, ~6× faster than
`Ripper.sexp`, and the lexer is ~24–30× faster than `Ripper.lex` — all while
building a full Go AST. Methodology, full parity tables, and the allocation
hotspots / action items are in [`BENCHMARKS.md`](BENCHMARKS.md); reproduce with
[`benchmarks/run.sh`](benchmarks) (an isolated module, outside the coverage
gate).

## Known limitations

The following are not yet parsed (they remain on go-embedded-ruby's roadmap;
contributions welcome):

- paren-less command calls with keyword/splat/block args (`foo a: 1`)
- default **block** parameters (`{ |a = 1| }`) — splat block params (`{ |*a| }`) are supported
- the positional `Class(a)` find-pattern (`Class[a]` is supported)

## License

BSD-3-Clause © the go-ruby-parser/parser authors.
