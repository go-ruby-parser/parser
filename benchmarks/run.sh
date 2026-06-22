#!/usr/bin/env bash
# Reproduce the BENCHMARKS.md numbers end-to-end.
#
#   ./run.sh            # default: Go count=6, MRI iters=12, warmups=3
#   ./run.sh 8 16 4     # Go count, MRI iters, MRI warmups
#
# The Go benchmarks are a SEPARATE module (own go.mod) so they never enter the
# parser's 100%-coverage CI gate. GOWORK=off keeps them self-contained even when
# a parent go.work is present.
set -euo pipefail
cd "$(dirname "$0")"

GO_COUNT="${1:-6}"
MRI_ITERS="${2:-12}"
MRI_WARMUPS="${3:-3}"

echo "== go-ruby-parser (Go) =="
GOWORK=off go test -run x -bench='Parse|Lex' -benchmem -count="$GO_COUNT" .

echo
echo "== MRI reference (Ripper / RubyVM::AST) =="
ruby mri_bench.rb "$MRI_ITERS" "$MRI_WARMUPS"
