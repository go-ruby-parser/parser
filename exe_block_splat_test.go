package parser_test

import (
	"reflect"
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// TestEndBlockRecordsNoSplat.
//
// `END { }` desugars to `at_exit { }`, and the Block it built left SplatIndex at
// Go's zero value. Zero is a perfectly good index, and the field's "no splat"
// value is -1 — so the block announced a *splat parameter at position 0 with no
// parameters to hold it. go-embedded-ruby's compiler believed it and emitted a
// rest-binding that read past an empty environment:
//
//	$ echo 'END { puts "E" }' > p.rb && rbgo p.rb
//	panic: runtime error: slice bounds out of range [:1] with capacity 0
//
// An uncontained Go panic on ordinary Ruby, which for an embedder takes the host
// process down (go-embedded-ruby/ruby#805). Nothing here crashed, because a
// parser does not run what it builds: the cost landed entirely in a consumer.
func TestEndBlockRecordsNoSplat(t *testing.T) {
	for _, src := range []string{
		`END { 1 }`,
		`puts "x"` + "\n" + `END { 1 }`,
		`END { 1 }` + "\n" + `END { 2 }`,
	} {
		prog, err := parser.Parse(src)
		if err != nil {
			t.Fatalf("Parse(%q): %v", src, err)
		}
		found := false
		for _, n := range prog.Body {
			call, ok := n.(*ast.Call)
			if !ok || call.Name != "at_exit" || call.Block == nil {
				continue
			}
			found = true
			if call.Block.SplatIndex != -1 {
				t.Errorf("%q: END block SplatIndex = %d, want -1 (no splat)",
					src, call.Block.SplatIndex)
			}
			if len(call.Block.Params) != 0 {
				t.Errorf("%q: END block has %d params, want 0", src, len(call.Block.Params))
			}
		}
		if !found {
			t.Errorf("%q: no at_exit call produced", src)
		}
	}
}

// TestEveryBlockHasAConsistentSplatIndex is the guard, and it is the point of
// this file rather than the case above.
//
// SplatIndex is an int whose absent value is -1, so a Block built by a literal
// that does not mention the field claims a splat at 0. Two of the three
// &ast.Block literals in parser.go set it — the two that build their value from
// parsed parameters. The one that forgot was the one with no parameters to
// parse, which is exactly the shape a future desugaring will have.
//
// So this does not check one construct. It parses a corpus and asserts the
// invariant over EVERY Block in every tree: SplatIndex is -1, or a valid index
// into Params. A new desugaring that forgets the field fails here rather than in
// a consumer's runtime.
func TestEveryBlockHasAConsistentSplatIndex(t *testing.T) {
	sources := []string{
		`END { 1 }`,
		`BEGIN { 1 }` + "\n" + `END { 2 }`,
		`at_exit { 1 }`,
		`[1].each { |x| x }`,
		`[1].each { |*a| a }`,
		`[1].each { |a, *b, c| b }`,
		`[1].each { }`,
		`[1].each do |x| x end`,
		`f = lambda { |a, *b| a }`,
		`f = ->(a) { a }`,
		`define_method(:x) { |*a| a }`,
		`[1].map { |a, (b, c)| b }`,
		`[1].each { |a, b: 1| a }`,
		`[1].each { |a, &blk| a }`,
		`proc { |a = 1| a }`,
		`loop { break }`,
		`[[1,2]].each { |(a, b)| a }`,
	}
	for _, src := range sources {
		prog, err := parser.Parse(src)
		if err != nil {
			t.Fatalf("Parse(%q): %v", src, err)
		}
		n := 0
		walkBlocks(reflect.ValueOf(prog), func(b *ast.Block) {
			n++
			switch {
			case b.SplatIndex == -1:
			case b.SplatIndex >= 0 && b.SplatIndex < len(b.Params):
			default:
				t.Errorf("%q: Block with %d param(s) has SplatIndex %d; want -1 or an index into Params",
					src, len(b.Params), b.SplatIndex)
			}
		})
		if n == 0 {
			// Without this the guard would pass on a walk that found nothing,
			// which is indistinguishable from a tree with no defects.
			t.Errorf("%q: the walk found no Block at all", src)
		}
	}
}

// walkBlocks visits every *ast.Block reachable from v. It walks by reflection
// rather than by a type switch over the AST so that a node added later is
// covered without anyone remembering to extend it -- the failure this guard
// exists to catch is precisely somebody not remembering.
func walkBlocks(v reflect.Value, fn func(*ast.Block)) {
	seen := map[uintptr]bool{}
	var walk func(reflect.Value)
	walk = func(v reflect.Value) {
		switch v.Kind() {
		case reflect.Ptr, reflect.Interface:
			if v.IsNil() {
				return
			}
			if v.Kind() == reflect.Ptr {
				if seen[v.Pointer()] {
					return
				}
				seen[v.Pointer()] = true
				if b, ok := v.Interface().(*ast.Block); ok {
					fn(b)
				}
			}
			walk(v.Elem())
		case reflect.Slice, reflect.Array:
			for i := 0; i < v.Len(); i++ {
				walk(v.Index(i))
			}
		case reflect.Map:
			for _, k := range v.MapKeys() {
				walk(v.MapIndex(k))
			}
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				if v.Type().Field(i).IsExported() {
					walk(v.Field(i))
				}
			}
		}
	}
	walk(v)
}
