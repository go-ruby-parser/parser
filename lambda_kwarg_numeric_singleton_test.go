package parser_test

import (
	"math/big"
	"strings"
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// This file covers eight refusals fixed together, each traced to one production
// in MRI's grammar. Every `accepted` entry below is a shape both ruby 4.0.5
// parsers (prism and `--parser=parse.y`) accept, and every `refused` entry is one
// both of them refuse — checked shape by shape, not by exit status alone.

// lambdaBlock digs the *ast.Block out of the `lambda { … }` call a stabby lambda
// desugars to.
func lambdaBlock(t *testing.T, src string) *ast.Block {
	t.Helper()
	call, ok := mustParseSingle(t, src).(*ast.Call)
	if !ok {
		t.Fatalf("Parse(%q): top is %T, want *ast.Call", src, mustParseSingle(t, src))
	}
	if call.Name != "lambda" {
		t.Fatalf("Parse(%q): call name = %q, want %q", src, call.Name, "lambda")
	}
	if call.Block == nil {
		t.Fatalf("Parse(%q): call has no block", src)
	}
	return call.Block
}

// --- 1. a keyword parameter first in a PAREN-LESS lambda parameter list ---

// TestParenlessLambdaKeywordParams is the target defect: `-> x: 1 { x }` was
// refused with "expected {, got x (LABEL)".
//
// MRI's paren-less form is the plain `f_larglist: f_args` alternative (parse.y
// v3_4_0 5216) — the SAME `f_args` the parenthesised form takes — and
// `f_args: args_tail` (6473) makes a bare `args_tail` a complete parameter list.
// `args_tail` begins with `f_kwarg(f_kw)` (6371), `f_kw: f_label arg_value |
// f_label` (6611), `f_label: tLABEL` (6591). A tLABEL is therefore a legal FIRST
// token, exactly as `**kw` and `&b` already were.
//
// The parameter names carry the trailing-colon sentinel parseBlockParams uses for
// a keyword parameter, and Defaults is nil for a required one.
func TestParenlessLambdaKeywordParams(t *testing.T) {
	cases := []struct {
		src        string
		wantParams []string
		wantDflt   []bool // true where a default expression is present
	}{
		{"-> x: { x }", []string{"x:"}, []bool{false}},
		{"-> x: 1 { x }", []string{"x:"}, []bool{true}},
		{"-> x: 1 do x end", []string{"x:"}, []bool{true}},
		{"-> a:, b: { }", []string{"a:", "b:"}, []bool{false, false}},
		{"-> x: 1, y: 2 { }", []string{"x:", "y:"}, []bool{true, true}},
		{"-> k: 1, **kw { }", []string{"k:", "**kw"}, []bool{true, false}},
		// A positional parameter first already worked; it must keep working, and it
		// pins that the two entry paths agree on the recorded shape.
		{"-> x, y: 2 { }", []string{"x", "y:"}, []bool{false, true}},
		// The parenthesised spelling is the other f_larglist alternative.
		{"->(x: 1) { x }", []string{"x:"}, []bool{true}},
	}
	for _, c := range cases {
		blk := lambdaBlock(t, c.src)
		if strings.Join(blk.Params, ",") != strings.Join(c.wantParams, ",") {
			t.Errorf("Parse(%q): params = %v, want %v", c.src, blk.Params, c.wantParams)
			continue
		}
		if len(blk.Defaults) != len(c.wantDflt) {
			t.Errorf("Parse(%q): %d defaults, want %d", c.src, len(blk.Defaults), len(c.wantDflt))
			continue
		}
		for i, want := range c.wantDflt {
			if got := blk.Defaults[i] != nil; got != want {
				t.Errorf("Parse(%q): default %d present = %v, want %v", c.src, i, got, want)
			}
		}
	}
}

// TestParenlessLambdaKeywordParamBlockParam covers the `&b` tail after a keyword
// parameter (`args_tail: f_kwarg(f_kw) opt_f_block_arg`, 6376), which lands in
// BlockParam rather than Params.
func TestParenlessLambdaKeywordParamBlockParam(t *testing.T) {
	blk := lambdaBlock(t, "-> k: 1, &b { }")
	if strings.Join(blk.Params, ",") != "k:" {
		t.Errorf("params = %v, want [k:]", blk.Params)
	}
	if blk.BlockParam != "b" {
		t.Errorf("BlockParam = %q, want %q", blk.BlockParam, "b")
	}
}

// --- 2. negating an `r`/`i`-suffixed numeric literal ---

// TestNegateRationalAndImaginaryLiteral covers the unchecked type assertion that
// ended negateLiteral: applyNumSuffix wraps the literal in a RationalLit and/or
// an ImaginaryLit, and the assertion was `n.(*ast.FloatLit)`. Every `-Nr`, `-Ni`
// and `-Nri` therefore panicked, surfacing as "internal parser error".
//
// The wrapper is preserved and the innermost numeric is negated. MRI sets a
// `minus` flag on the OUTERMOST node (`negate_lit`, parse.y v3_4_0: NODE_RATIONAL
// and NODE_IMAGINARY each carry their own), which for these shapes is the same
// number — (-3)/1 is (-3/1), and 0 + (-3/1)i is (0-(3/1)*i), the value ruby 4.0.5
// prints for `p(-3ri)`.
func TestNegateRationalAndImaginaryLiteral(t *testing.T) {
	t.Run("-3r is RationalLit(IntLit -3)", func(t *testing.T) {
		r, ok := mustParseSingle(t, "-3r").(*ast.RationalLit)
		if !ok {
			t.Fatalf("top is %T, want *ast.RationalLit", mustParseSingle(t, "-3r"))
		}
		i, ok := r.Value.(*ast.IntLit)
		if !ok || i.Value != -3 {
			t.Fatalf("RationalLit.Value = %#v, want IntLit(-3)", r.Value)
		}
	})
	t.Run("-3i is ImaginaryLit(IntLit -3)", func(t *testing.T) {
		im, ok := mustParseSingle(t, "-3i").(*ast.ImaginaryLit)
		if !ok {
			t.Fatalf("top is %T, want *ast.ImaginaryLit", mustParseSingle(t, "-3i"))
		}
		i, ok := im.Value.(*ast.IntLit)
		if !ok || i.Value != -3 {
			t.Fatalf("ImaginaryLit.Value = %#v, want IntLit(-3)", im.Value)
		}
	})
	t.Run("-3ri nests imaginary over rational", func(t *testing.T) {
		im, ok := mustParseSingle(t, "-3ri").(*ast.ImaginaryLit)
		if !ok {
			t.Fatalf("top is %T, want *ast.ImaginaryLit", mustParseSingle(t, "-3ri"))
		}
		r, ok := im.Value.(*ast.RationalLit)
		if !ok {
			t.Fatalf("ImaginaryLit.Value = %T, want *ast.RationalLit", im.Value)
		}
		i, ok := r.Value.(*ast.IntLit)
		if !ok || i.Value != -3 {
			t.Fatalf("innermost = %#v, want IntLit(-3)", r.Value)
		}
	})
	t.Run("-0.5r negates the float inside", func(t *testing.T) {
		r, ok := mustParseSingle(t, "-0.5r").(*ast.RationalLit)
		if !ok {
			t.Fatalf("top is %T, want *ast.RationalLit", mustParseSingle(t, "-0.5r"))
		}
		f, ok := r.Value.(*ast.FloatLit)
		if !ok || f.Value != -0.5 {
			t.Fatalf("RationalLit.Value = %#v, want FloatLit(-0.5)", r.Value)
		}
	})
	t.Run("a negated bignum with a suffix stays a bignum", func(t *testing.T) {
		const src = "-99999999999999999999999r"
		r, ok := mustParseSingle(t, src).(*ast.RationalLit)
		if !ok {
			t.Fatalf("top is %T, want *ast.RationalLit", mustParseSingle(t, src))
		}
		bn, ok := r.Value.(*ast.BignumLit)
		if !ok {
			t.Fatalf("RationalLit.Value = %T, want *ast.BignumLit", r.Value)
		}
		want, _ := new(big.Int).SetString("-99999999999999999999999", 10)
		if bn.Val.Cmp(want) != 0 {
			t.Errorf("value = %s, want %s", bn.Val, want)
		}
	})
}

// --- 3. the `r`/`i` suffix on a radix-prefixed integer, and MRI's two suffix rules ---

// TestNumericSuffixOnRadixLiteral covers `0x10r` and friends, which lexRadixInt
// simply did not read: parse.y v3_4_0 calls number_literal_suffix(NUM_SUFFIX_ALL)
// on the hex (9839), binary (9863), explicit-decimal (9892) and octal (9920)
// paths exactly as on the decimal one.
func TestNumericSuffixOnRadixLiteral(t *testing.T) {
	cases := []struct {
		src  string
		kind string // "r", "i", or "ri"
		val  int64
	}{
		{"0x10r", "r", 16},
		{"0b10i", "i", 2},
		{"0o7r", "r", 7},
		{"0d9i", "i", 9},
		{"0x10ri", "ri", 16},
		{"0xar", "r", 10}, // 'a' is a hex digit; 'r' is the suffix
		{"0xai", "i", 10},
	}
	for _, c := range cases {
		n := mustParseSingle(t, c.src)
		// Unwrap in the order applyNumSuffix wrapped: "r" innermost, "i" outermost.
		if strings.Contains(c.kind, "i") {
			im, ok := n.(*ast.ImaginaryLit)
			if !ok {
				t.Errorf("Parse(%q): top is %T, want *ast.ImaginaryLit", c.src, n)
				continue
			}
			n = im.Value
		}
		if strings.Contains(c.kind, "r") {
			r, ok := n.(*ast.RationalLit)
			if !ok {
				t.Errorf("Parse(%q): inner is %T, want *ast.RationalLit", c.src, n)
				continue
			}
			n = r.Value
		}
		i, ok := n.(*ast.IntLit)
		if !ok || i.Value != c.val {
			t.Errorf("Parse(%q): innermost = %#v, want IntLit(%d)", c.src, n, c.val)
		}
	}
}

// TestNumericSuffixRejectedByTrailingLetter pins the rule that makes
// `2rescue nil` a valid program: number_literal_suffix (parse.y v3_4_0 9044)
// rewinds and returns NO suffix when the character ending the scan is a letter,
// `_`, or non-ASCII. `2r` followed by `escue` would be a syntax error; `2` with a
// modifier `rescue` is what MRI parses, and `ruby -e 'p 2rescue nil'` prints 2.
func TestNumericSuffixRejectedByTrailingLetter(t *testing.T) {
	// `p 2rescue nil` is `(p 2) rescue nil`: a Begin with one rescue clause.
	b, ok := mustParseSingle(t, "p 2rescue nil").(*ast.Begin)
	if !ok {
		t.Fatalf("top is %T, want *ast.Begin (a modifier rescue)", mustParseSingle(t, "p 2rescue nil"))
	}
	if len(b.Rescues) != 1 {
		t.Fatalf("%d rescue clauses, want 1", len(b.Rescues))
	}
	call, ok := b.Body[0].(*ast.Call)
	if !ok || call.Name != "p" || len(call.Args) != 1 {
		t.Fatalf("Begin.Body[0] = %#v, want Call p with 1 arg", b.Body[0])
	}
	// The argument must be the PLAIN integer 2 — not a rational.
	if i, ok := call.Args[0].(*ast.IntLit); !ok || i.Value != 2 {
		t.Fatalf("argument = %#v, want IntLit(2) with no suffix", call.Args[0])
	}
}

// TestNumericSuffixIAfterRIsNotR pins the other half of that function: after an
// `i`, the `r` bit is masked off ("rational of complex is disallowed", 9053), so
// `2ri` carries both suffixes and `2ir` carries neither — and with `r` no longer
// acceptable, the trailing-letter rule then rejects the whole suffix, which is
// why ruby 4.0.5 refuses `p 2ir` under both its parsers.
func TestNumericSuffixIAfterRIsNotR(t *testing.T) {
	// 2ri: rational inside imaginary.
	im, ok := mustParseSingle(t, "2ri").(*ast.ImaginaryLit)
	if !ok {
		t.Fatalf("2ri: top is %T, want *ast.ImaginaryLit", mustParseSingle(t, "2ri"))
	}
	if _, ok := im.Value.(*ast.RationalLit); !ok {
		t.Fatalf("2ri: ImaginaryLit.Value = %T, want *ast.RationalLit", im.Value)
	}
	// 2ir: refused, because the suffix scan rewinds and leaves the identifier `ir`
	// sitting after the integer.
	if _, err := parser.Parse("p 2ir"); err == nil {
		t.Error("Parse(`p 2ir`): accepted, want a refusal (MRI refuses it too)")
	}
}

// --- 4. a singleton `def` whose receiver is separated by `::` ---

// TestSingletonDefViaScopeOperator covers `def M::name(v)` and `def self::m`,
// which reach rbconfig.rb and mkmf.rb. MRI has one production for both
// separators: `defs_head: k_def singleton dot_or_colon def_name` (parse.y v3_4_0
// 3405) with `dot_or_colon: '.' | tCOLON2` (6839). The `::` spelling must produce
// exactly the same MethodDef the `.` spelling does.
func TestSingletonDefViaScopeOperator(t *testing.T) {
	t.Run("def self::m sets Singleton", func(t *testing.T) {
		d, ok := mustParseSingle(t, "def self::m; end").(*ast.MethodDef)
		if !ok {
			t.Fatalf("top is %T, want *ast.MethodDef", mustParseSingle(t, "def self::m; end"))
		}
		if !d.Singleton || d.Recv != nil || d.Name != "m" {
			t.Errorf("got Singleton=%v Recv=%#v Name=%q, want true/nil/\"m\"", d.Singleton, d.Recv, d.Name)
		}
	})
	t.Run("def M::name(v) sets a ConstRef receiver", func(t *testing.T) {
		const src = "def M::name(v); end"
		d, ok := mustParseSingle(t, src).(*ast.MethodDef)
		if !ok {
			t.Fatalf("top is %T, want *ast.MethodDef", mustParseSingle(t, src))
		}
		c, ok := d.Recv.(*ast.ConstRef)
		if !ok || c.Name != "M" {
			t.Fatalf("Recv = %#v, want ConstRef{M}", d.Recv)
		}
		if d.Singleton {
			t.Error("Singleton = true, want false (an explicit receiver, not self)")
		}
		if d.Name != "name" || len(d.Params) != 1 || d.Params[0] != "v" {
			t.Errorf("got Name=%q Params=%v, want \"name\" [v]", d.Name, d.Params)
		}
	})
	t.Run(":: and . yield the same node", func(t *testing.T) {
		dot, ok := mustParseSingle(t, "def M.name(v); end").(*ast.MethodDef)
		if !ok {
			t.Fatal("def M.name: not a MethodDef")
		}
		scope, ok := mustParseSingle(t, "def M::name(v); end").(*ast.MethodDef)
		if !ok {
			t.Fatal("def M::name: not a MethodDef")
		}
		if dot.Name != scope.Name || dot.Singleton != scope.Singleton ||
			len(dot.Params) != len(scope.Params) {
			t.Errorf("`.` gave %#v, `::` gave %#v — the same production must agree", dot, scope)
		}
	})
	t.Run("keyword_variable receivers", func(t *testing.T) {
		// `singleton: var_ref` and `var_ref: user_variable | keyword_variable`
		// (parse.y v3_4_0 6711), so nil/true/false are receivers MRI parses.
		for _, src := range []string{"def nil.m; end", "def true.m; end", "def false.m; end"} {
			d, ok := mustParseSingle(t, src).(*ast.MethodDef)
			if !ok {
				t.Fatalf("Parse(%q): top is %T, want *ast.MethodDef", src, mustParseSingle(t, src))
			}
			if d.Recv == nil {
				t.Errorf("Parse(%q): Recv = nil, want a literal receiver node", src)
			}
		}
	})
}

// --- 5. `when`/`in`/`case` with the value on the next line ---

// TestWhenValueOnNextLine covers rubygems/request.rb:125. `case_body: k_when
// case_args then` (parse.y v3_4_0 6191) has no `terms` between `k_when` and
// `case_args`, so a newline there can only be legal because MRI's lexer discards
// it: `when` sets a value-expecting state and `parser_yylex`'s '\n' case does not
// emit a terminator in one.
func TestWhenValueOnNextLine(t *testing.T) {
	const src = "case x\nwhen\n  1 then 2\nend"
	c, ok := mustParseSingle(t, src).(*ast.Case)
	if !ok {
		t.Fatalf("top is %T, want *ast.Case", mustParseSingle(t, src))
	}
	if len(c.Whens) != 1 {
		t.Fatalf("%d when clauses, want 1", len(c.Whens))
	}
	if len(c.Whens[0].Conds) != 1 {
		t.Fatalf("%d conditions, want 1", len(c.Whens[0].Conds))
	}
	if i, ok := c.Whens[0].Conds[0].(*ast.IntLit); !ok || i.Value != 1 {
		t.Errorf("condition = %#v, want IntLit(1)", c.Whens[0].Conds[0])
	}
	if len(c.Whens[0].Body) != 1 {
		t.Errorf("%d body nodes, want 1 — the newline must join, not split", len(c.Whens[0].Body))
	}
}

// TestCaseWhenWithoutTerminator covers `k_case terms?` (6136): the terminator
// after `case` is OPTIONAL, so a subject-less `case` may be followed by its first
// clause keyword with nothing in between.
func TestCaseWhenWithoutTerminator(t *testing.T) {
	const src = "case when false then 1 end"
	c, ok := mustParseSingle(t, src).(*ast.Case)
	if !ok {
		t.Fatalf("top is %T, want *ast.Case", mustParseSingle(t, src))
	}
	if c.Subject != nil {
		t.Errorf("Subject = %#v, want nil (the subject-less case)", c.Subject)
	}
	if len(c.Whens) != 1 {
		t.Errorf("%d when clauses, want 1", len(c.Whens))
	}
}

// TestClauseKeywordAsMethodNameDoesNotContinue is the regression this batch
// produced and then had to fix: once `in` continued a line, `def in` ⏎ `body`
// joined into `def in body` and prism/node.rb — 728 of 728 stdlib files
// otherwise — stopped parsing. A reserved word in a method-NAME position
// (MRI's EXPR_FNAME, after `def`, and EXPR_DOT, after a dot) is a name, not the
// operator it usually spells, so the line must not continue.
func TestClauseKeywordAsMethodNameDoesNotContinue(t *testing.T) {
	for _, name := range []string{"in", "then", "case", "when", "if", "unless", "while", "until", "and", "or"} {
		src := "class C\n  def " + name + "\n    other_loc.slice\n  end\nend"
		prog, err := parser.Parse(src)
		if err != nil {
			t.Errorf("Parse(def %s with a body on the next line): %v", name, err)
			continue
		}
		cd, ok := prog.Body[0].(*ast.ClassDef)
		if !ok {
			t.Errorf("def %s: top is %T, want *ast.ClassDef", name, prog.Body[0])
			continue
		}
		d, ok := cd.Body[0].(*ast.MethodDef)
		if !ok {
			t.Errorf("def %s: class body[0] is %T, want *ast.MethodDef", name, cd.Body[0])
			continue
		}
		if d.Name != name {
			t.Errorf("method name = %q, want %q", d.Name, name)
		}
		if len(d.Params) != 0 {
			t.Errorf("def %s: %d params, want 0 — the next line is the BODY", name, len(d.Params))
		}
		if len(d.Body) != 1 {
			t.Errorf("def %s: %d body nodes, want 1", name, len(d.Body))
		}
	}
}

// --- 6. `**nil` in a block / lambda parameter list ---

// TestBlockKeywordNoRest covers `-> (**nil) { }` and `{ |**nil| }`. MRI reaches
// it through `block_args_tail: f_any_kwrest opt_f_block_arg` (parse.y v3_4_0
// 5006), `f_any_kwrest: f_kwrest | f_no_kwarg` and `f_no_kwarg: p_kwnorest:
// kwrest_mark keyword_nil` — the same marker a method list already accepted.
func TestBlockKeywordNoRest(t *testing.T) {
	t.Run("stabby lambda", func(t *testing.T) {
		blk := lambdaBlock(t, "-> (**nil) { }")
		if strings.Join(blk.Params, ",") != "**nil" {
			t.Errorf("params = %v, want [**nil]", blk.Params)
		}
	})
	t.Run("brace block with a positional first", func(t *testing.T) {
		call, ok := mustParseSingle(t, "proc { |a, **nil| }").(*ast.Call)
		if !ok || call.Block == nil {
			t.Fatalf("top is %#v, want a call with a block", mustParseSingle(t, "proc { |a, **nil| }"))
		}
		if strings.Join(call.Block.Params, ",") != "a,**nil" {
			t.Errorf("params = %v, want [a **nil]", call.Block.Params)
		}
	})
	t.Run("followed by a block param", func(t *testing.T) {
		blk := lambdaBlock(t, "-> (a, **nil, &b) { }")
		if strings.Join(blk.Params, ",") != "a,**nil" {
			t.Errorf("params = %v, want [a **nil]", blk.Params)
		}
		if blk.BlockParam != "b" {
			t.Errorf("BlockParam = %q, want %q", blk.BlockParam, "b")
		}
	})
	t.Run("an anonymous ** is still distinct from **nil", func(t *testing.T) {
		blk := lambdaBlock(t, "-> (**) { }")
		if strings.Join(blk.Params, ",") != "**" {
			t.Errorf("params = %v, want [**]", blk.Params)
		}
	})
}

// --- 7. BEGIN { } and END { } ---

// TestBeginBlockHoistsToProgramFront covers `BEGIN { }`: MRI accumulates the
// block into `p->eval_tree_begin` and runs it before the program body, leaving a
// void `NEW_BEGIN(0)` where it stood (parse.y v3_4_0 3020, 3029). Both halves are
// reproduced, so the statement that textually FOLLOWS a BEGIN runs after it.
func TestBeginBlockHoistsToProgramFront(t *testing.T) {
	const src = "p 2\nBEGIN { p 1 }"
	prog := mustParse(t, src)
	if len(prog.Body) != 3 {
		t.Fatalf("%d top-level nodes, want 3 (hoisted body, `p 2`, the void Begin)", len(prog.Body))
	}
	first, ok := prog.Body[0].(*ast.Call)
	if !ok || first.Name != "p" {
		t.Fatalf("Body[0] = %#v, want the hoisted `p 1`", prog.Body[0])
	}
	if i, ok := first.Args[0].(*ast.IntLit); !ok || i.Value != 1 {
		t.Errorf("Body[0] argument = %#v, want IntLit(1): the BEGIN body must come FIRST", first.Args[0])
	}
	second, ok := prog.Body[1].(*ast.Call)
	if !ok {
		t.Fatalf("Body[1] = %#v, want `p 2`", prog.Body[1])
	}
	if i, ok := second.Args[0].(*ast.IntLit); !ok || i.Value != 2 {
		t.Errorf("Body[1] argument = %#v, want IntLit(2)", second.Args[0])
	}
	// The placeholder left behind evaluates to nil and must carry no body.
	last, ok := prog.Body[2].(*ast.Begin)
	if !ok {
		t.Fatalf("Body[2] = %#v, want an empty *ast.Begin placeholder", prog.Body[2])
	}
	if len(last.Body) != 0 || len(last.Rescues) != 0 {
		t.Errorf("placeholder = %#v, want an empty Begin (MRI's NEW_BEGIN(0))", last)
	}
}

// TestEndBlockIsAtExit covers `END { }`, desugared to the replacement MRI's own
// warning names (`rb_warn0("END in method; use at_exit")`, parse.y v3_4_0 3196).
func TestEndBlockIsAtExit(t *testing.T) {
	call, ok := mustParseSingle(t, "END { p 1 }").(*ast.Call)
	if !ok {
		t.Fatalf("top is %T, want *ast.Call", mustParseSingle(t, "END { p 1 }"))
	}
	if call.Name != "at_exit" || call.Recv != nil {
		t.Fatalf("got %q with Recv %#v, want a receiver-less at_exit", call.Name, call.Recv)
	}
	if call.Block == nil || len(call.Block.Body) != 1 {
		t.Fatalf("block = %#v, want one body statement", call.Block)
	}
	if len(call.Args) != 0 {
		t.Errorf("%d arguments, want 0", len(call.Args))
	}
}

// TestBeginBlockNestingRule covers where BEGIN is and is not allowed. Every
// nested body reaches `stmt_or_begin` (parse.y v3_4_0 3093), whose keyword_BEGIN
// alternative exists only to emit "BEGIN is permitted only at toplevel" — but
// `begin_block` itself reads a `top_compstmt` (3029), so a BEGIN inside a BEGIN
// is at top level again and parses.
func TestBeginBlockNestingRule(t *testing.T) {
	if _, err := parser.Parse("BEGIN { BEGIN { } }"); err != nil {
		t.Errorf("Parse(`BEGIN { BEGIN { } }`): %v — begin_block reads a top_compstmt", err)
	}
	for _, src := range []string{
		"def f; BEGIN { }; end",
		"if true; BEGIN { }; end",
		"1.times { BEGIN { } }",
		"class C; BEGIN{}; end",
		"(BEGIN { })",
	} {
		_, err := parser.Parse(src)
		if err == nil {
			t.Errorf("Parse(%q): accepted, want refused (MRI: BEGIN is permitted only at toplevel)", src)
			continue
		}
		if !strings.Contains(err.Error(), "toplevel") {
			t.Errorf("Parse(%q): %v — want the message to name the toplevel rule", src, err)
		}
	}
	// END is an ordinary `stmt` (3193), legal in a method body; MRI only warns.
	if _, err := parser.Parse("def f; END { }; end"); err != nil {
		t.Errorf("Parse(`def f; END { }; end`): %v — END is a plain stmt", err)
	}
}

// --- 8. an operator method name after a dot ---

// TestOperatorMethodNameAfterDot covers `6543.21.%(137)` from
// library/bigdecimal/shared/modulo.rb and `2./(3)`. The four %-literal openers
// and the regexp opener were each guarded by `prevType != token.DEF`, which named
// `def` but not a dot, so a `%` after `.` opened a %Q string and a `/` opened a
// regexp. MRI's states for a method-name position are EXPR_FNAME and EXPR_DOT,
// and neither belongs to the EXPR_BEG_ANY set `parser_yylex` tests before calling
// parse_percent or opening a regexp.
func TestOperatorMethodNameAfterDot(t *testing.T) {
	cases := []struct{ src, name string }{
		{"6543.21.%(137)", "%"},
		{"2.%(3)", "%"},
		{"2./(3)", "/"},
		{"x.%(3)", "%"},
		{"x&.%(3)", "%"},
		{"x./(3)", "/"},
	}
	for _, c := range cases {
		call, ok := mustParseSingle(t, c.src).(*ast.Call)
		if !ok {
			t.Errorf("Parse(%q): top is %T, want *ast.Call", c.src, mustParseSingle(t, c.src))
			continue
		}
		if call.Name != c.name {
			t.Errorf("Parse(%q): method = %q, want %q", c.src, call.Name, c.name)
		}
		if call.Recv == nil {
			t.Errorf("Parse(%q): Recv = nil, want the literal/variable before the dot", c.src)
		}
		if len(call.Args) != 1 {
			t.Errorf("Parse(%q): %d args, want 1", c.src, len(call.Args))
		}
	}
}

// TestPercentAndRegexpStillOpenInValuePosition is the other side of that
// predicate: where a VALUE is expected, `%(` and `/` must still open literals.
// Without this the fix above could be satisfied by never opening them at all.
func TestPercentAndRegexpStillOpenInValuePosition(t *testing.T) {
	if s, ok := mustParseSingle(t, "%(hi)").(*ast.StringLit); !ok || s.Value != "hi" {
		t.Errorf("Parse(`%%(hi)`) = %#v, want StringLit(\"hi\")", mustParseSingle(t, "%(hi)"))
	}
	if _, ok := mustParseSingle(t, "/a/").(*ast.RegexpLit); !ok {
		t.Errorf("Parse(`/a/`) = %T, want *ast.RegexpLit", mustParseSingle(t, "/a/"))
	}
	if _, ok := mustParseSingle(t, "%w[a b]").(*ast.ArrayLit); !ok {
		t.Errorf("Parse(`%%w[a b]`) = %T, want *ast.ArrayLit", mustParseSingle(t, "%w[a b]"))
	}
	// `def %(o)` and `def /(o)` name the operator methods, unchanged.
	for _, src := range []string{"def %(o); end", "def /(o); end"} {
		d, ok := mustParseSingle(t, src).(*ast.MethodDef)
		if !ok {
			t.Errorf("Parse(%q): top is %T, want *ast.MethodDef", src, mustParseSingle(t, src))
			continue
		}
		if len(d.Params) != 1 {
			t.Errorf("Parse(%q): %d params, want 1", src, len(d.Params))
		}
	}
}

// --- 9. "can't define singleton method for literals" ---

// TestSingletonDefLiteralReceiverRefused covers MRI's own check on the `singleton`
// production, over a fixed node list (parse.y v3_4_0 6724-6744: NODE_STR, DSTR,
// XSTR, DXSTR, REGX, DREGX, SYM, DSYM, LINE, FILE, ENCODING, INTEGER, FLOAT,
// RATIONAL, IMAGINARY, LIST, ZLIST).
//
// Ruby 4.0 MOVED this check: v3_4_0 applies it only to the `'(' expr rparen`
// alternative, while 4.0.5 restructured `singleton` into
// `value_expr(singleton_expr)` and applies it to the bare `var_ref` spelling too.
// `def __FILE__.m` is the shape that distinguishes them — it parses under the
// v3_4_0 grammar and is refused by ruby 4.0.5 under BOTH its parsers, which is
// the oracle this repo measures against.
func TestSingletonDefLiteralReceiverRefused(t *testing.T) {
	for _, src := range []string{
		"def __FILE__.m; end", "def __LINE__.m; end", "def __ENCODING__.m; end",
		"def (1).m; end", "def (1.5).m; end", "def (2r).m; end", "def (2i).m; end",
		`def ("s").m; end`, `def ("a#{1}").m; end`, "def (:sym).m; end",
		"def ([]).m; end", "def ([1]).m; end", "def (/re/).m; end", "def (`x`).m; end",
	} {
		if _, err := parser.Parse(src); err == nil {
			t.Errorf("Parse(%q): accepted, want refused (MRI: can't define singleton method for literals)", src)
		}
	}
	// Not in MRI's list: nil/true/false/self and a hash, plus any real expression.
	for _, src := range []string{
		"def (nil).m; end", "def (self).m; end", "def ({}).m; end",
		"def (obj).m; end", "def (foo.bar).m; end",
	} {
		if _, err := parser.Parse(src); err != nil {
			t.Errorf("Parse(%q): %v — MRI accepts this receiver", src, err)
		}
	}
}

// TestSingletonDefReceiverIsNotAPath pins an over-acceptance this batch removed:
// MRI's receiver is a single `var_ref`, never a constant path, so
// `def M::Bar::baz` is a SyntaxError ("unexpected ::, expecting '\n' or ';'").
// It used to be accepted here by mis-reading it as `def M` with a `::Bar::baz`
// body — a definition of the wrong method, silently.
func TestSingletonDefReceiverIsNotAPath(t *testing.T) {
	for _, src := range []string{"def M::Bar::baz; end", "def obj&.m; end"} {
		if _, err := parser.Parse(src); err == nil {
			t.Errorf("Parse(%q): accepted, want refused (MRI refuses it under both parsers)", src)
		}
	}
}

// --- the standing accept/refuse tables, extended ---

// TestWt47AcceptTable and TestWt47RefuseTable extend the repo's existing
// accept/refuse tables with this batch's shapes plus the controls that must not
// move. Each entry was judged by BOTH ruby 4.0.5 front ends (prism and
// `--parser=parse.y`) and they agree on every one; shapes where they disagree
// (`a[foo bar do end]`, `f(foo bar do end)` and the `not`-in-`expr` family) are
// deliberately absent, because there is nothing there to assert.
func TestWt47AcceptTable(t *testing.T) {
	accepted := []string{
		// 1. paren-less lambda keyword parameters
		"-> x: { x }", "-> x: 1 { x }", "-> x: 1 do x end", "-> a:, b: { }",
		"-> x: 1, y: 2 { }", "-> k: 1, **kw { }", "-> k: 1, &b { }",
		"-> x, y: 2 { }", "-> x: 1 { x }.call(x: 2)", "->(x: 1) { x }",
		// 2. negated suffixed numerics
		"p(-3r)", "p(-3i)", "p(-3ri)", "p(-0.5r)", "p(-2.5i)", "p(-3r ** 2)", "p(-3r.to_s)",
		// 3. radix literals with a suffix
		"p 0x10r", "p 0b10i", "p 0o7r", "p 0d9i", "p 0x10ri", "p 0xar",
		"p(-0x10r)", "p 1_000r", "p 2rescue nil",
		// 4. `::` singleton def
		"def M::name(v); end", "def M::Name(v); end", "def self::m; end",
		"def obj::m; end", "def @iv::m; end", "def $g::m; end", "def @@cv::m; end",
		"def (foo)::m; end", "def M::m=(v); end", "def M::[](i); end",
		"def M::+(o); end", "def M::class; end", "def M::%(o); end",
		"def nil.m; end", "def true.m; end", "def false.m; end",
		// 5. a clause value on the next line
		"case x\nwhen\n  1\n  2\nend",
		"case x\nwhen\n  1 then 2\nend",
		"case x\nwhen\n  1, 2\n  3\nend",
		"case\n  x\nwhen 1\n  2\nend",
		"case h\nin\n  {}\n  1\nend",
		"for i in\n  [1]\n  p i\nend",
		"case when false then 1 end",
		"case\nwhen false then 1\nend",
		"if x\nthen\n  1\nend",
		"case x\nwhen 1\nthen 2\nend",
		"class C\n  def in\n    in_loc.slice\n  end\nend",
		"class C\n  def then\n    then_loc.slice\n  end\nend",
		// 6. **nil
		"-> (**nil) { }", "lambda { |**nil| }", "proc { |a, **nil| }",
		"-> (a, **nil, &b) { }", "def f(**nil); end",
		// 7. BEGIN / END
		"BEGIN { }", "END { }", "BEGIN { p 1 }\nEND { p 2 }", "BEGIN { BEGIN { } }",
		"p 2\nBEGIN { p 1 }", "def f; END { }; end",
		// 8. an operator method name after a dot, and the value-position literals
		"p 6543.21.%(137)", "p 2.%(3)", "p 2./(3)", "p x.%(3)", "p x&.%(3)",
		"p %(hi)", `p(/a/ =~ "a")`, "p %w[a b]",
		"class C; def %(o); end; end", "class C; def /(o); end; end",
		// controls from the standing table that must stay accepted
		"a[foo bar]", "f(foo bar, 1)", "x = (y do end)", "[defined? foo]",
		"{k: defined? foo}", "p((x rescue y))",
		// shapes that fail for SEMANTIC reasons unless written inside the context
		// that makes them legal — the distinction `ruby -c` alone cannot make
		"def f; yield 1; end", "while true; break; end", "[1].each { next }",
		"def f; return 1; end", "class C; def f; super; end; end",
	}
	for _, src := range accepted {
		if _, err := parser.Parse(src); err != nil {
			t.Errorf("Parse(%q): %v — MRI accepts this under both parsers", src, err)
		}
	}
}

func TestWt47RefuseTable(t *testing.T) {
	refused := []string{
		// over-acceptances removed by this batch
		"def M::Bar::baz; end", "def obj&.m; end",
		// dot_or_colon is the ONLY thing that may follow a parenthesised receiver
		"def (foo)m; end", "def (foo) m; end",
		"def __FILE__.m; end", "def __LINE__.m; end", "def __ENCODING__.m; end",
		"def (1).m; end", "def (1.5).m; end", `def ("s").m; end`, `def ("a#{1}").m; end`,
		"def (:sym).m; end", "def ([]).m; end", "def ([1]).m; end", "def (/re/).m; end",
		"def (2r).m; end", "def (2i).m; end", "def (`x`).m; end",
		// BEGIN outside a top_stmts list, and the `do` spelling of either block
		"def f; BEGIN { }; end", "if true; BEGIN { }; end", "1.times { BEGIN { } }",
		"class C; BEGIN{}; end", "(BEGIN { })", "BEGIN do end", "END do end",
		// `r` after `i` is not a suffix pair, and the whole suffix is then rejected
		"p 2ir",
		// a command call in an `arg` position, even where `yield` itself is legal:
		// `[yield 1]` is the `[foo bar]` rule, not a semantic refusal
		"def f; [yield 1]; end",
		// `{k: not x}` — both MRI parsers refuse this one, unlike the rest of the
		// `not`-in-`expr` family, where they disagree and nothing is asserted
		"{k: not x}",
		// controls from the standing table that must stay refused
		"while begin; y do end; end; end", "while y do end; end", "[foo bar]",
		"if x end", "while x end", "case x; when 1 end", "p(x rescue y)",
	}
	for _, src := range refused {
		if _, err := parser.Parse(src); err == nil {
			t.Errorf("Parse(%q): accepted, want refused (MRI refuses it under both parsers)", src)
		}
	}
}
