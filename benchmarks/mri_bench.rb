# MRI reference benchmark — the C parser this project is measured against.
#
# Times three reference operations over the SAME on-disk corpus the Go
# benchmark uses (corpus/snippets, corpus/stdlib):
#
#   * RubyVM::AbstractSyntaxTree.parse — the production C parser that builds the
#     node tree CRuby itself compiles from (the truest "parse" comparison).
#   * Ripper.sexp                      — the event-driven C parser exposed to
#     Ruby, building an s-expression tree.
#   * Ripper.lex                       — tokenisation only, comparable to our
#     lexer.Tokenize().
#
# Methodology mirrors the Go harness: load all sources into memory once, warm
# up, then take the best of N timed passes (single core). Reports KB/s and
# ns/file for each op/corpus.
#
# Usage: ruby mri_bench.rb [iterations] [warmups]

require 'ripper'
require 'benchmark'

ITERS   = (ARGV[0] || 12).to_i
WARMUPS = (ARGV[1] || 3).to_i
HERE    = File.expand_path(File.dirname(__FILE__))

# load_corpus returns [sources, byte_total] for a named corpus.
#   * "snippets" — the embedded project corpus under corpus/snippets.
#   * "stdlib"   — the both-accept CRuby files listed in
#     corpus/stdlib_manifest.txt, read from THIS Ruby's own rubylibdir (the same
#     set the Go harness times). Returns nil if any listed file is missing.
def load_corpus(name)
  if name == 'stdlib'
    libdir = RbConfig::CONFIG['rubylibdir']
    manifest = File.join(HERE, 'corpus', 'stdlib_manifest.txt')
    rels = File.readlines(manifest).map(&:strip).reject(&:empty?)
    paths = rels.map { |r| File.join(libdir, r) }
    return nil unless paths.all? { |p| File.exist?(p) }
    srcs = paths.map { |p| File.read(p) }
  else
    dir = File.join(HERE, 'corpus', name)
    srcs = Dir.glob(File.join(dir, '*.rb')).sort.map { |f| File.read(f) }
  end
  [srcs, srcs.sum(&:bytesize)]
end

# best_of runs `block` over the whole corpus `iters` times (after `warmups`
# untimed passes) and returns the fastest wall-clock pass, in seconds.
def best_of(srcs, iters, warmups)
  WARMUPS.times { srcs.each { |s| yield s } }
  best = Float::INFINITY
  iters.times do
    t0 = Process.clock_gettime(Process::CLOCK_MONOTONIC)
    srcs.each { |s| yield s }
    dt = Process.clock_gettime(Process::CLOCK_MONOTONIC) - t0
    best = dt if dt < best
  end
  best
end

def report(op, corpus, srcs, bytes, secs)
  kbs    = (bytes / 1024.0) / secs
  nsfile = (secs * 1e9) / srcs.size
  printf("%-22s %-9s %10.1f KB/s %12.0f ns/file\n", op, corpus, kbs, nsfile)
  { op: op, corpus: corpus, kb_s: kbs, ns_file: nsfile }
end

results = []
%w[snippets stdlib].each do |corpus|
  loaded = load_corpus(corpus)
  if loaded.nil?
    warn "corpus #{corpus}: skipped (stdlib files not found in this Ruby)"
    next
  end
  srcs, bytes = loaded
  warn "corpus #{corpus}: #{srcs.size} files, #{bytes} bytes"

  secs = best_of(srcs, ITERS, WARMUPS) { |s| RubyVM::AbstractSyntaxTree.parse(s) }
  results << report('RubyVM::AST.parse', corpus, srcs, bytes, secs)

  secs = best_of(srcs, ITERS, WARMUPS) { |s| Ripper.sexp(s) }
  results << report('Ripper.sexp', corpus, srcs, bytes, secs)

  secs = best_of(srcs, ITERS, WARMUPS) { |s| Ripper.lex(s) }
  results << report('Ripper.lex', corpus, srcs, bytes, secs)
end

# Emit a machine-readable block the BENCHMARKS.md generator / a human can paste.
puts '--- JSON ---'
require 'json'
puts JSON.generate(results)
