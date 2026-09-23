#!/usr/bin/env bash
# A 100% coverage gate that means 100%.
#
# The gate this replaces read the total `go tool cover -func` prints. That total is
# rounded to one decimal, so it says "100.0%" for anything from 99.95% upward -- and
# `%.1f` crosses that line at exactly 2000 statements:
#
#     1999 statements, 1 uncovered -> 99.949975% -> prints 99.9%  -> refused
#     2000 statements, 1 uncovered -> 99.950000% -> prints 100.0% -> PASSED
#
# This module instruments 2770 statements, so it is over that line: one uncovered
# statement would have printed as 100.0% and passed a gate named for 100%. It is
# exact today (2770 of 2770, measured with this workflow's own command), which is
# what makes this the right moment to change the instrument rather than the code.
#
# What the rounding hides is not a random statement. It hides the ones hardest to
# reach from a test, and that population is mostly GUARDS -- in go-crdt, where this
# was first found, the two hidden statements were both allocation bounds against
# hostile input.
#
# So this counts statements in the profile, where the numbers are exact. Each line is
#     file.go:fromLine.col,toLine.col numberOfStatements timesExecuted
# and a block executed zero times is the thing a 100% gate exists to refuse.
#
# Usage: coverage-gate.sh [--report] <profile> [awk-regexp over the file:line field]
#
# The regexp narrows the gate to part of a profile; matching NOTHING is a failure
# rather than an empty pass, so a rename cannot quietly empty it. --report prints the
# count without failing on uncovered statements.
set -euo pipefail

report=0
if [ "${1:-}" = "--report" ]; then
  report=1
  shift
fi
profile=${1:?usage: coverage-gate.sh [--report] <profile> [pattern]}
pattern=${2:-}

# Two passes over the profile, because it may carry the SAME block more than
# once. `go test -coverpkg=X ./...` writes one section per test binary, each
# listing every block of X with that binary's counts -- so a block the root
# package's tests cover and another package's tests do not appears twice, once
# with a count and once with a zero. `go tool cover` merges those by block; a
# script that reads each line on its own does not, and reports thousands of
# uncovered statements that are covered. Measured: 4646 of them, on a profile the
# rounded gate called 100.0%.
#
# So the first pass sums the counts per block and the second prints the ones that
# are still zero, in file order.
awk -v pat="$pattern" -v report="$report" '
  FNR == 1 { next }                    # the "mode:" header, in each pass
  pat != "" && $1 !~ pat { next }
  NR == FNR {
    if (!($1 in stmts)) { stmts[$1] = $2; order[++n] = $1 }
    hits[$1] += $3
    next
  }
  # Second pass: nothing to do per line, the work is in END.
  { }
  END {
    for (i = 1; i <= n; i++) {
      key = order[i]
      total += stmts[key]
      if (hits[key] + 0 == 0 && stmts[key] + 0 > 0) {
        uncovered += stmts[key]
        if (!report) print "uncovered (" stmts[key] " statements): " key
      }
    }
    if (total == 0) {
      print "::error::the coverage gate matched no statements" \
            (pat == "" ? "" : " for /" pat "/") ", which is not a pass"
      exit 1
    }
    printf "%d of %d statements covered\n", total - uncovered, total
    if (report) exit 0
    if (uncovered > 0) {
      print "::error::" uncovered " statement(s) uncovered; the gate is 100%"
      exit 1
    }
  }
' "$profile" "$profile"
