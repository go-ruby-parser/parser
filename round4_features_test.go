package parser_test

import (
	"testing"

	"github.com/go-ruby-parser/parser"
	"github.com/go-ruby-parser/parser/ast"
)

// parsesOK asserts every source in srcs parses without error. Used for the
// many Round-4 features whose acceptance (matching MRI `ruby -c`) is what
// matters; AST-shape assertions follow in dedicated tests where the shape is
// load-bearing.
func parsesOK(t *testing.T, srcs ...string) {
	t.Helper()
	for _, src := range srcs {
		if _, err := parser.Parse(src); err != nil {
			t.Errorf("Parse(%q): %v", src, err)
		}
	}
}

// parseErrs asserts every source fails to parse (a malformed-input guard).
func parseErrs(t *testing.T, srcs ...string) {
	t.Helper()
	for _, src := range srcs {
		if _, err := parser.Parse(src); err == nil {
			t.Errorf("Parse(%q): expected error, got none", src)
		}
	}
}

// --- Feature 1: parenthesized masgn LHS ---

func TestParenMasgn(t *testing.T) {
	parsesOK(t,
		"(a, b) = x, y\n",
		"((a, b), c) = z\n",
		"(a, *b) = list\n",
		"(*a, b) = list\n",
		"(a, *) = list\n",
		"a, (b, c) = 1, [2, 3]\n",
		"(last_wait, wait) = wait, last_wait + wait\n",
		"(a, b,) = x\n", // trailing comma inside the group
		"[1].each { |(a, b)| a }\n",
		"[1].each { |(a, b), c| a }\n",
	)
}

func TestParenMasgnShape(t *testing.T) {
	ma, ok := mustParseSingle(t, "(a, b) = x, y\n").(*ast.MultiAssign)
	if !ok {
		t.Fatalf("top node is %T, want *ast.MultiAssign", mustParseSingle(t, "(a, b) = x, y\n"))
	}
	// The single paren group is one target whose node is a nested MultiAssign.
	if len(ma.Targets) != 1 {
		t.Fatalf("want 1 outer target, got %d", len(ma.Targets))
	}
	inner, ok := ma.Targets[0].(*ast.MultiAssign)
	if !ok {
		t.Fatalf("nested target is %T, want *ast.MultiAssign", ma.Targets[0])
	}
	if len(inner.Names) != 2 || inner.Names[0] != "a" || inner.Names[1] != "b" {
		t.Fatalf("inner names = %v, want [a b]", inner.Names)
	}
	if inner.Values != nil {
		t.Fatalf("nested group must have nil Values, got %v", inner.Values)
	}
}

// --- Feature 2: scope-resolution method call ---

func TestScopeResolutionCall(t *testing.T) {
	parsesOK(t,
		"Syslog::LOG_UPTO(Syslog::LOG_INFO)\n",
		"Mod::meth arg\n",
		"Math::PI\n",
		"Foo::bar(1, 2)\n",
		"A::B::C\n",
		"x = Math::PI + 1\n",
		"Mod::run do\n x\nend\n",
		"obj::Down x, y\n",
	)
}

func TestScopeResolutionCallShape(t *testing.T) {
	call, ok := mustParseSingle(t, "Syslog::LOG_UPTO(Syslog::LOG_INFO)\n").(*ast.Call)
	if !ok {
		t.Fatalf("want *ast.Call for scope-resolution call")
	}
	if call.Name != "LOG_UPTO" || call.Recv == nil {
		t.Fatalf("call = %+v, want Name=LOG_UPTO with receiver", call)
	}
}

// --- Feature 3: trailing operator line continuation ---

func TestTrailingOperatorContinuation(t *testing.T) {
	parsesOK(t,
		"batch <<\n  x\n",
		"new_args <<\n  x\n",
		"a = b &\n  c\n",
		"x = foo |\n  bar\n",
		"y = a ^\n  b\n",
		"batch << \"\\n\" <<\n  x\n",
	)
	// Operand-position |/&/<</^ must NOT continue: block params, block-pass,
	// heredocs and pattern pins keep working.
	parsesOK(t,
		"[1].each { |x| x }\n",
		"foo(&blk)\n",
		"z = a & b\n",
		"x = <<~HEREDOC\n  text\nHEREDOC\n",
	)
}

// --- Feature 4: anonymous-argument forwarding at call sites ---

func TestAnonForwarding(t *testing.T) {
	parsesOK(t,
		"def m(*); g(*); end\n",
		"def k(**); h(**); end\n",
		"def b(&); c(&); end\n",
		"def f(...); g(**); end\n",
		"view_context.render(inline: x, **)\n",
		"buffer = capture { value = yield(*, **) }\n",
		"foo(&)\n",
		"bar(*)\n",
		"to_str[*]\n",
		"h(a, *, **, &)\n",
	)
}

func TestAnonForwardingShape(t *testing.T) {
	call := mustParseSingle(t, "foo(&)\n").(*ast.Call)
	bp, ok := call.Args[0].(*ast.BlockPass)
	if !ok || bp.Value != nil {
		t.Fatalf("foo(&) arg = %#v, want anonymous BlockPass (nil Value)", call.Args[0])
	}
	call = mustParseSingle(t, "bar(*)\n").(*ast.Call)
	sp, ok := call.Args[0].(*ast.SplatArg)
	if !ok || sp.Value != nil {
		t.Fatalf("bar(*) arg = %#v, want anonymous SplatArg (nil Value)", call.Args[0])
	}
}

// --- Feature 5: assignment inside a ternary branch ---

func TestTernaryAssign(t *testing.T) {
	parsesOK(t,
		"cond ? a = b : c\n",
		"x ? ENV[\"k\"] = v : super\n",
		"@i.nil? ? @i = true : @i\n",
		"old_tz ? ENV[\"TZ\"] = old_tz : ENV.delete(\"TZ\")\n",
		"root_session ? root_session.assertions = assertions : super\n",
		"c ? a : b ? d = 1 : e\n",
	)
}

// --- Feature 6: class/module definition as a value ---

func TestClassModuleAsValue(t *testing.T) {
	parsesOK(t,
		"sc = class << stat; self; end\n",
		"c = class Foo < Bar; 1; end\n",
		"m = module M; 1; end\n",
		"x = (class A; end)\n",
	)
	if _, ok := mustParseSingle(t, "c = class Foo; 1; end\n").(*ast.Assign); !ok {
		t.Fatalf("class-as-value should be the RHS of an Assign")
	}
}

// --- Feature 7: unicode identifiers / method names / symbols ---

func TestUnicodeIdentifiers(t *testing.T) {
	parsesOK(t,
		"def なまえ; end\n",
		"Weird.create(なまえ: \"たこ焼き仮面\")\n",
		"{ 🎃: \"x\" }\n",
		":🇺🇸\n",
		"enum :language, [:🇺🇸, :🇪🇸, :🇫🇷]\n",
		":røcket\n",
		"なまえ = 1\n",
		"add_reference :a, :røcket, foreign_key: { to_table: :r }\n",
	)
	// A non-ASCII char literal still works (?é) — the multi-byte path.
	parsesOK(t, "p ?é\n")
}

// TestOperatorMethodName asserts that an explicitly-called operator method
// (`recv.OP(arg)`) names the method by the operator's own spelling — not the
// backtick “ ` “ that only the empty-XSTRING backtick-method case yields.
func TestOperatorMethodName(t *testing.T) {
	cases := []struct{ src, name string }{
		{"1.+(2)", "+"},
		{"1.-(2)", "-"},
		{"1.*(2)", "*"},
		{"1.**(2)", "**"},
		{"a.<=>(b)", "<=>"},
		{"a.<(b)", "<"},
		{"a.>(b)", ">"},
		{"a.<=(b)", "<="},
		{"a.>=(b)", ">="},
		{"a.==(b)", "=="},
		{"a.===(b)", "==="},
		{"a.!=(b)", "!="},
		{"a.<<(b)", "<<"},
		{"a.>>(b)", ">>"},
		{"a.&(b)", "&"},
		{"a.|(b)", "|"},
		{"a.^(b)", "^"},
		{"a.=~(b)", "=~"},
		{"a.!~(b)", "!~"},
		{"a.~", "~"},
		{"a.!", "!"},
		{"a.``(c)", "`"}, // the backtick method (empty XSTRING literal), still "`"
	}
	for _, c := range cases {
		call, ok := parseOne(t, c.src).(*ast.Call)
		if !ok {
			t.Fatalf("Parse(%q): top node = %T, want *ast.Call", c.src, parseOne(t, c.src))
		}
		if call.Name != c.name {
			t.Errorf("Parse(%q): method name = %q, want %q", c.src, call.Name, c.name)
		}
	}
}

// --- Feature 8: safe-navigation with an operator method ---

func TestSafeNavOperatorMethod(t *testing.T) {
	parsesOK(t,
		"x&.[](i)\n",
		"query&.[] :ensure\n",
		"obj&.+(y)\n",
		"@request&.env&.[](\"k\")\n",
		"arr.[]=(i, v)\n",
		"x.&(y)\n",
		"x.|(y)\n",
		"x.^(y)\n",
		"x.~\n",
		"x.!\n",
		"x.>>(y)\n",
		"x.===(y)\n",
		"x.=~(y)\n",
	)
}

// --- Feature 9: operator / backtick method symbols + !~ ---

func TestOperatorSymbolsAndNMatch(t *testing.T) {
	parsesOK(t,
		"x = :`\n",
		"receive(:`)\n",
		"x = :!~\n",
		"ignores = [:to_s, :=~, :!~, :===]\n",
		"assert Mime[:js] !~ \"text/html\"\n",
		"assert zone !~ /Nonexistent_Place/\n",
		"a !~\n  b\n", // !~ continuation
		"def `(cmd); end\n",
	)
	sym, ok := mustParseSingle(t, ":`\n").(*ast.SymbolLit)
	if !ok || sym.Name != "`" {
		t.Fatalf(":` should be a SymbolLit with Name=`")
	}
}

// --- Feature 10: ranges and patterns ---

func TestBeginlessRangeAsArg(t *testing.T) {
	parsesOK(t,
		"x = [...11, 11]\n",
		"assert_operator(...0, :overlap?, -1..0)\n",
		"foo(..5)\n",
	)
	// Bare `...` forwarding must still be recognised.
	parsesOK(t, "def f(...); g(...); end\n", "foo(...)\n")
}

func TestScopedConstPattern(t *testing.T) {
	parsesOK(t,
		"case x\nin Prism::CaseNode[a]\n 1\nend\n",
		"case x\nin [Prism::SymbolNode[unescaped:]]\n 1\nend\n",
		"case x\nin Point[a, b]\n 1\nend\n",
		"case x\nin ::Foo\n 1\nend\n",
		"case x\nin Foo(a:, b:)\n 1\nend\n",
		"n in Prism::CaseNode[a]\n",
	)
}

// --- Statement-level keyword postfix / modifiers ---

func TestKeywordStatementPostfix(t *testing.T) {
	parsesOK(t,
		"if a\n 1\nelse\n 2\nend.html_safe\n",
		"while a\n b\nend.foo\n",
		"if a then 1 end if b\n",
		"begin\n x\nend if Process.respond_to?(:fork)\n",
		"def f; 1; end if cond\n",
		"class A; end if cond\n",
		"module M; end if cond\n",
		"until a\n b\nend.to_s\n",
		"unless a\n b\nend.to_s\n",
	)
}

// --- super with a block ---

func TestSuperBlock(t *testing.T) {
	parsesOK(t,
		"super do |record|\n x\nend\n",
		"super { |t| yield t }\n",
		"super() { |h, k| x }\n",
		"super(operation, payload) do\n x\nend\n",
		"number_to_currency super\n",
		"Request.new super, url_helpers, @block\n",
	)
	s, ok := mustParseSingle(t, "super { 1 }\n").(*ast.Super)
	if !ok || s.Block == nil {
		t.Fatalf("super { 1 } should be a Super with a Block")
	}
}

// --- alias / undef fitem list does not over-continue ---

func TestAliasFitemList(t *testing.T) {
	parsesOK(t,
		"def ==(o); 1; end\nalias eql? ==\ndef hash; 1; end\n",
		"alias eql? ==\n",
		"alias foo bar\nbaz\n",
		"undef ==\nx\n",
		"alias x y; z\n",
		"a ==\n  b\n", // ordinary == continuation must still work
	)
}

// --- attribute / index array-RHS assignment ---

func TestAttributeArrayRHS(t *testing.T) {
	parsesOK(t,
		"self.cache_store = :file_store, X\n",
		"@controller.cache_store = :file_store, @cache_path\n",
		"Capybara.server = :puma, { Silent: true }\n",
		"h[k] = 1, 2\n",
	)
}

// --- jump values: break/next take a comma list ---

func TestJumpCommaValues(t *testing.T) {
	parsesOK(t,
		"next table, true\n",
		"break a, b\n",
		"[1].each { next 1, 2 }\n",
		"next\n",
		"break\n",
	)
	nx, ok := mustParseSingle(t, "next 1, 2\n").(*ast.Next)
	if !ok {
		t.Fatalf("want *ast.Next")
	}
	if _, ok := nx.Value.(*ast.ArrayLit); !ok {
		t.Fatalf("next 1, 2 value = %T, want *ast.ArrayLit", nx.Value)
	}
}

// --- scoped masgn target (::Const) ---

func TestScopedMasgnTarget(t *testing.T) {
	parsesOK(t,
		"old_zone, ::Time.zone = ::Time.zone, new_zone\n",
		"::Foo, x = 1, 2\n",
	)
}

// --- command-arg value forms (super / const conversion) ---

func TestConstConversionCommand(t *testing.T) {
	parsesOK(t,
		"bd = BigDecimal \"0.01\"\n",
		"Integer \"42\"\n",
		"Float 1\n",
		"Sym :x\n",
	)
	// A constant next to a binary operator or a bare reference stays a const.
	if _, ok := mustParseSingle(t, "Foo + 1\n").(*ast.BinaryExpr); !ok {
		t.Fatalf("Foo + 1 should stay a BinaryExpr")
	}
	if _, ok := mustParseSingle(t, "Foo\n").(*ast.ConstRef); !ok {
		t.Fatalf("Foo should be a ConstRef")
	}
}

// --- rescue into a non-local target ---

func TestRescueIntoTarget(t *testing.T) {
	parsesOK(t,
		"begin\n x\nrescue => @error\n y\nend\n",
		"begin\nrescue => @setup_exception; end\n",
		"begin\nrescue => e\n y\nend\n",
		"begin\nrescue Foo => $g\nend\n",
		"begin\nrescue => @@c\nend\n",
		"begin\nrescue => obj.err\nend\n",
	)
}

// --- predicate / bang labels ---

func TestPredicateLabels(t *testing.T) {
	parsesOK(t,
		"{ frozen?: frozen? }\n",
		"h = { has_key?: true, include?: true }\n",
		"deprecate auto_populated?: :x, deprecator: Y\n",
		"foo valid!: 1\n",
	)
	// Ternary with a predicate condition must still parse (no false label).
	parsesOK(t, "x = empty? ? 1 : 2\n", "x = a ? b : c\n")
}

// --- rational / imaginary literals ---

func TestRationalImaginary(t *testing.T) {
	parsesOK(t,
		"x = 2r\n", "y = 0.5r\n", "z = 3i\n", "w = 2.5ri\n",
		"Time.new(2002, 10, 31, 2, 2, 2.123456789r)\n",
		"n = 1.upto(3)\n",
	)
	if _, ok := mustParseSingle(t, "x = 2r\n").(*ast.Assign).Value.(*ast.RationalLit); !ok {
		t.Fatalf("2r should be a RationalLit")
	}
	if _, ok := mustParseSingle(t, "x = 3i\n").(*ast.Assign).Value.(*ast.ImaginaryLit); !ok {
		t.Fatalf("3i should be an ImaginaryLit")
	}
	// `ri` nests imaginary over rational.
	im := mustParseSingle(t, "x = 2ri\n").(*ast.Assign).Value.(*ast.ImaginaryLit)
	if _, ok := im.Value.(*ast.RationalLit); !ok {
		t.Fatalf("2ri should be ImaginaryLit{RationalLit{...}}")
	}
	// A bignum literal with an `r` suffix exercises the bignum branch.
	parsesOK(t, "x = 99999999999999999999999999999r\n")
}

// --- nested compound assignment ---

func TestNestedCompoundAssign(t *testing.T) {
	parsesOK(t,
		"distribution[record] > 0 && distribution[record] -= 1\n",
		"record.car.save && count += 1\n",
		"[1].each { |x| x.save && count += 1 }\n",
		"@x.nil? && @x ||= 1\n",
		"a += 1\n",
		"cond && $g += 1\n",
		"cond && @@c += 1\n",
		"flag && obj.attr += 1\n",
	)
}

// --- def parameter defaults (masgn suppression + self-name) ---

func TestDefParamDefaults(t *testing.T) {
	parsesOK(t,
		"def make_codec(secret = secret(\"secret\"), v = nil, **options); end\n",
		"def attach_to(ns, sub = new, notifier = AS::N.instance, inherit_all: false); end\n",
		"def create_migration(p = default, c = {}, g = self, &block); end\n",
		"def f(s = c(1), v = nil); end\n",
		"def f(a = 1, b: a); end\n",
		"def f(secret = secret(\"x\")); end\n",
	)
}

// --- class name with an expression receiver path ---

func TestClassPathExpression(t *testing.T) {
	parsesOK(t,
		"class self.class::TestRailtie < Rails::Railtie; end\n",
		"class obj::Bar; end\n",
		"class Foo::Bar; end\n",
		"class ::Top; end\n",
		"module A::B; end\n",
	)
	parseErrs(t, "class 1.foo; end\n") // a non-path receiver is rejected
}

// --- nested do-block inside a brace block that is a command argument ---

func TestNestedBlockInCommandArg(t *testing.T) {
	parsesOK(t,
		"include Module.new {\n  define_method(:x) do\n    1\n  end\n}\n",
		"while foo do\n bar\nend\n", // loop do must still bind to the loop
	)
}

// --- option global variables ($-I) ---

func TestOptionGlobals(t *testing.T) {
	parsesOK(t,
		"$-I.each { |p| p }\n",
		"$stdout.puts 1\n",
		"x = $$\n",
	)
	parseErrs(t, "x = $\n") // a bare $ is still illegal
}
