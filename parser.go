// Package parser builds an AST from tokens: recursive descent for statements,
// Pratt (precedence-climbing) for expressions, and a scope stack to resolve the
// classic local-variable-vs-method-call ambiguity (plan-rbgo.md §10).
//
// The scope stack is what lets `foo` mean a variable read when `foo` was
// assigned earlier in the same def, and a (possibly command-style) method call
// otherwise — exactly MRI's rule.
package parser

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/go-ruby-parser/parser/ast"
	"github.com/go-ruby-parser/parser/lexer"
	"github.com/go-ruby-parser/parser/token"
)

type parseError struct{ msg string }

func (e parseError) Error() string { return e.msg }

// scope tracks declared locals. A hard scope is a method/class/module/top-level
// boundary that local lookup does not cross; a soft scope (a block) chains to
// its enclosing scope, so a block sees and can assign the enclosing locals.
type scope struct {
	locals map[string]bool
	hard   bool
	// Implicit block-parameter tracking (numbered params _1.._9 and `it`),
	// only meaningful for a block scope that declared no explicit |params|.
	explicitParams bool
	maxNum         int  // highest _N referenced in the block body (0 = none)
	usedIt         bool // bare `it` referenced in the block body
}

func newScope(hard bool) *scope { return &scope{locals: map[string]bool{}, hard: hard} }

// numberedParam returns N for a numbered block parameter name "_1".."_9", or 0.
func numberedParam(name string) int {
	if len(name) == 2 && name[0] == '_' && name[1] >= '1' && name[1] <= '9' {
		return int(name[1] - '0')
	}
	return 0
}

// implicitParamScope returns the innermost scope if it is a block that may host
// implicit numbered/`it` parameters (a soft scope with no explicit |params|),
// or nil. Implicit parameters bind to the innermost block and never cross a
// method/class boundary.
func (p *Parser) implicitParamScope() *scope {
	s := p.scope()
	if s.hard || s.explicitParams {
		return nil
	}
	return s
}

// finishImplicitParams resolves the parameter list of a freshly-parsed block.
// With explicit params it returns them unchanged; otherwise it synthesises the
// numbered (_1.._maxNum) or `it` parameters its body referenced. A body may not
// mix the two forms.
func (p *Parser) finishImplicitParams(s *scope, explicit []string) []string {
	if len(explicit) > 0 {
		return explicit
	}
	if s.maxNum > 0 && s.usedIt {
		p.fail("`it` is not allowed together with numbered parameters")
	}
	if s.maxNum > 0 {
		// Numbered parameters may not nest: an enclosing block (up to the nearest
		// method/class boundary) that also uses them is a Ruby SyntaxError.
		for i := len(p.scopes) - 2; i >= 0; i-- {
			if p.scopes[i].maxNum > 0 {
				p.fail("numbered parameter is already used in outer block")
			}
			if p.scopes[i].hard {
				break
			}
		}
		names := make([]string, s.maxNum)
		for i := range names {
			names[i] = "_" + string(rune('1'+i))
		}
		return names
	}
	if s.usedIt {
		return []string{"it"}
	}
	return explicit
}

// Parser holds parsing state.
type Parser struct {
	toks   []token.Token
	pos    int
	scopes []*scope
	// noDo suppresses `do…end` block attachment while parsing a while/until
	// condition, so the `do` there belongs to the loop, not to a call in the
	// condition.
	noDo bool
	// patternDepth > 0 while parsing a pattern atom, where a top-level `|` is the
	// alternation separator rather than the bitwise-or operator.
	patternDepth int
	// noPipe suppresses treating `|` as the bitwise-or operator while parsing the
	// default expression of an optional block parameter inside a `|...|` list, so
	// the `|` there closes the parameter list rather than continuing the default.
	noPipe bool
	// noMasgn suppresses multiple-assignment detection while parsing a parameter
	// default (`def f(a = c(1), b = nil)`), so the comma separating parameters is
	// not mistaken for a masgn target separator (`c(1), b = nil`).
	noMasgn bool
}

// parseHook, when non-nil, runs at the start of Parse. It exists only so a
// white-box test can inject a non-parseError panic and exercise the recover's
// internal-error path; it is nil in normal operation.
var parseHook func()

// Parse lexes and parses src into a Program. It never panics: a malformed input
// yields a parse error, and any unexpected internal panic is also surfaced as a
// parse error rather than propagating to the caller.
func Parse(src string) (prog *ast.Program, err error) {
	toks := lexer.New(src).Tokenize()
	p := &Parser{toks: toks, scopes: []*scope{newScope(true)}}
	defer func() {
		if r := recover(); r != nil {
			if pe, ok := r.(parseError); ok {
				prog, err = nil, pe
				return
			}
			// An internal bug (e.g. an unexpected type assertion or index) must not
			// crash the caller: report it as a parse error so the parser's contract
			// of never panicking holds for every input.
			prog, err = nil, parseError{msg: fmt.Sprintf("internal parser error: %v", r)}
		}
	}()
	if parseHook != nil {
		parseHook()
	}
	body := p.parseStatements(map[token.Type]bool{})
	p.expect(token.EOF)
	return &ast.Program{Body: body}, nil
}

// --- token cursor ---

// The cursor clamps at the trailing EOF: once exhausted, cur/peekTok keep
// returning EOF and advance is a no-op, so a parser that over-reads on malformed
// input (e.g. an unterminated string interpolation) fails cleanly via expect
// instead of indexing past the slice and panicking.
func (p *Parser) cur() token.Token {
	if p.pos >= len(p.toks) {
		return p.toks[len(p.toks)-1] // the EOF token
	}
	return p.toks[p.pos]
}

// peekTok returns the token after the cursor. Every caller first checks the
// cursor is a specific non-EOF token, and the trailing EOF is always present, so
// the cursor is at most the second-to-last token and pos+1 stays in range.
func (p *Parser) peekTok() token.Token { return p.toks[p.pos+1] }

func (p *Parser) advance() token.Token {
	t := p.cur()
	if p.pos < len(p.toks) {
		p.pos++
	}
	return t
}

func (p *Parser) is(tt token.Type) bool { return p.cur().Type == tt }

func (p *Parser) accept(tt token.Type) bool {
	if p.is(tt) {
		p.advance()
		return true
	}
	return false
}

func (p *Parser) expect(tt token.Type) token.Token {
	if !p.is(tt) {
		p.fail("expected %s, got %q (%s)", tt, p.cur().Lit, p.cur().Type)
	}
	return p.advance()
}

// fail never returns; the ast.Node result lets primary parsers write
// `return p.fail(...)` without an unreachable trailing return.
func (p *Parser) fail(format string, args ...any) ast.Node {
	t := p.cur()
	panic(parseError{msg: fmt.Sprintf("parse error at line %d: %s", t.Line, fmt.Sprintf(format, args...))})
}

func (p *Parser) skipNewlines() {
	for p.is(token.NEWLINE) {
		p.advance()
	}
}

// firstSignificantIs reports whether the first non-NEWLINE token at or after the
// cursor is of type tt, without consuming anything. The token stream always ends
// in a (non-NEWLINE) EOF, so the scan always finds a significant token.
func (p *Parser) firstSignificantIs(tt token.Type) bool {
	i := p.pos
	for i < len(p.toks) && p.toks[i].Type == token.NEWLINE {
		i++
	}
	return i < len(p.toks) && p.toks[i].Type == tt
}

// --- scope ---

func (p *Parser) scope() *scope         { return p.scopes[len(p.scopes)-1] }
func (p *Parser) pushScope()            { p.scopes = append(p.scopes, newScope(true)) }
func (p *Parser) pushBlockScope()       { p.scopes = append(p.scopes, newScope(false)) }
func (p *Parser) popScope()             { p.scopes = p.scopes[:len(p.scopes)-1] }
func (p *Parser) declareLocal(n string) { p.scope().locals[n] = true }

// isLocal reports whether n is a visible local: it searches the scope chain but
// does not cross a hard (method/class/module/top-level) boundary, while block
// scopes (soft) chain to their enclosing scope.
func (p *Parser) isLocal(n string) bool {
	for i := len(p.scopes) - 1; i >= 0; i-- {
		if p.scopes[i].locals[n] {
			return true
		}
		if p.scopes[i].hard {
			break
		}
	}
	return false
}

// barewordValue turns a bare name into the node it denotes: a local-variable
// reference when the name is a visible local, otherwise a no-arg method call on
// self. Used for `{x:}` hash shorthand.
func (p *Parser) barewordValue(name string) ast.Node {
	if p.isLocal(name) {
		return &ast.VarRef{Name: name}
	}
	return &ast.Call{Name: name}
}

// --- statements ---

var (
	bodyEnd       = map[token.Type]bool{token.END: true}
	braceBlockEnd = map[token.Type]bool{token.RBRACE: true}
	beginBodyEnd  = map[token.Type]bool{token.RESCUE: true, token.ELSE: true, token.ENSURE: true, token.END: true}
	caseBodyEnd   = map[token.Type]bool{token.WHEN: true, token.ELSE: true, token.END: true}
	inBodyEnd     = map[token.Type]bool{token.IN: true, token.ELSE: true, token.END: true}
	ifBodyEnd     = map[token.Type]bool{token.END: true, token.ELSE: true, token.ELSIF: true}
	// rangeHiEnds marks tokens that cannot begin a range's high endpoint, making
	// the range endless (`1..`, `arr[2..]`).
	rangeHiEnds = map[token.Type]bool{token.RBRACKET: true, token.RPAREN: true, token.RBRACE: true, token.NEWLINE: true, token.EOF: true, token.COMMA: true, token.END: true, token.THEN: true, token.DO: true}
)

func (p *Parser) parseStatements(stop map[token.Type]bool) []ast.Node {
	var body []ast.Node
	for {
		p.skipNewlines()
		if p.is(token.EOF) || stop[p.cur().Type] {
			break
		}
		body = append(body, p.parseStatement())
		// Statements are separated by newlines/semicolons; the lexer emits both
		// as NEWLINE. A terminator or EOF may follow directly.
		if !p.is(token.NEWLINE) && !p.is(token.EOF) && !stop[p.cur().Type] {
			p.fail("unexpected %q after statement", p.cur().Lit)
		}
	}
	return body
}

func (p *Parser) parseStatement() ast.Node {
	switch p.cur().Type {
	case token.DEF:
		// A method definition may carry a trailing modifier (`def f; …; end if c`)
		// and is occasionally chained (`def f; end.tap { … }`), so feed it through
		// the postfix/modifier machinery rather than returning it raw.
		return p.applyModifiers(p.parsePostfixTail(p.parseDef()))
	case token.CLASS:
		return p.applyModifiers(p.parsePostfixTail(p.parseClass()))
	case token.MODULE:
		return p.applyModifiers(p.parsePostfixTail(p.parseModule()))
	case token.RETURN:
		return p.applyModifiers(p.parseReturn())
	case token.BREAK:
		return p.applyModifiers(p.parseBreak())
	case token.NEXT:
		return p.applyModifiers(p.parseNext())
	case token.RETRY:
		p.advance()
		return p.applyModifiers(&ast.Retry{})
	case token.ALIAS:
		return p.applyModifiers(p.parseAlias())
	case token.UNDEF:
		return p.applyModifiers(p.parseUndef())
	default:
		return p.applyModifiers(p.parseOneLineMatch(p.parseKeywordLogical()))
	}
}

// parseKeywordLogical parses the low-precedence keyword operators `and`, `or`,
// and the prefix `not`. They bind looser than `=` (and everything else except
// the trailing if/unless/while/until modifiers), so they sit between an
// assignment and a statement. `and`/`or` are left-associative and desugar to the
// `&&`/`||` BinaryExpr nodes; `not` desugars to a `!` UnaryExpr. (`p(1 and 2)`
// is itself invalid Ruby, so this layer only appears in statement/assignment
// positions, never inside a paren-less argument.)
func (p *Parser) parseKeywordLogical() ast.Node {
	left := p.parseExprOrAssign()
	for p.is(token.AND) || p.is(token.OR) {
		op := "&&"
		if p.is(token.OR) {
			op = "||"
		}
		p.advance()
		left = &ast.BinaryExpr{Op: op, Left: left, Right: p.parseKeywordOperand()}
	}
	return left
}

// parseKeywordOperand parses an operand of `and`/`or`. Besides an ordinary
// expression (a leading `not` is handled by parseExprOrAssign) it accepts the
// jump keywords `return`/`break`/`next` (`a or return`, `x = 1 and break`),
// which are valid in this position in MRI.
func (p *Parser) parseKeywordOperand() ast.Node {
	switch p.cur().Type {
	case token.RETURN:
		return p.parseReturn()
	case token.BREAK:
		return p.parseBreak()
	case token.NEXT:
		return p.parseNext()
	}
	return p.parseExprOrAssign()
}

// parseOneLineMatch wraps a statement-level expression in a one-line pattern
// match when followed by `=> pattern` (rightward assignment) or `in pattern`
// (boolean test). `=` binds tighter than these, so `x = v in P` is `(x=v) in P`.
func (p *Parser) parseOneLineMatch(subject ast.Node) ast.Node {
	switch {
	case p.accept(token.HASHROCKET):
		return &ast.MatchPattern{Subject: subject, Pattern: p.parsePattern()}
	case p.accept(token.IN):
		return &ast.MatchPattern{Subject: subject, Pattern: p.parsePattern(), Bool: true}
	}
	return subject
}

// applyModifiers wraps a statement in trailing `if/unless/while/until` modifiers
// (`puts x if cond`, `return unless ok`).
func (p *Parser) applyModifiers(node ast.Node) ast.Node {
	for {
		switch p.cur().Type {
		case token.IF:
			p.advance()
			node = &ast.If{Cond: p.parseCond(), Then: []ast.Node{node}}
		case token.UNLESS:
			p.advance()
			node = &ast.If{Cond: not(p.parseCond()), Then: []ast.Node{node}}
		case token.WHILE:
			p.advance()
			node = &ast.While{Cond: p.parseCond(), Body: []ast.Node{node}}
		case token.UNTIL:
			p.advance()
			node = &ast.While{Cond: not(p.parseCond()), Body: []ast.Node{node}}
		default:
			return node
		}
	}
}

func not(n ast.Node) ast.Node { return &ast.UnaryExpr{Op: "!", Operand: n} }

// parseConstPath parses a constant path in a name/superclass position: a bare
// constant (`Foo`), a scope-resolution path (`Foo::Bar::Baz`), or a leading-`::`
// path (`::Foo`, `::Foo::Bar`). It returns the trailing segment name and, when
// the path is more than a bare constant, the full *ScopedConst node (else nil).
func (p *Parser) parseConstPath() (name string, path ast.Node) {
	var node ast.Node
	if p.accept(token.SCOPE) { // leading `::Name`
		name = p.expect(token.CONST).Lit
		node = &ast.ScopedConst{Name: name, Global: true}
	} else if p.is(token.CONST) {
		name = p.advance().Lit
		node = &ast.ConstRef{Name: name}
	} else {
		// An expression receiver before `::CONST` names a dynamically-scoped class
		// or module: `class self.class::Foo`, `class obj::Bar`. parsePostfix reads
		// the whole `recv::CONST` chain and yields a ScopedConst whose Name is the
		// trailing segment, exactly the path node we want.
		expr := p.parsePostfix()
		sc, ok := expr.(*ast.ScopedConst)
		if !ok {
			p.fail("expected a class/module path")
		}
		return sc.Name, sc
	}
	scoped := node // becomes the *ScopedConst once a `::CONST` segment is seen
	for p.is(token.SCOPE) && p.peekTok().Type == token.CONST {
		p.advance() // ::
		name = p.advance().Lit
		scoped = &ast.ScopedConst{Recv: scoped, Name: name}
	}
	if _, ok := scoped.(*ast.ConstRef); ok {
		return name, nil // a bare constant: no path node
	}
	return name, scoped
}

func (p *Parser) parseClass() ast.Node {
	p.expect(token.CLASS)
	// `class << target` opens target's singleton (metaclass). A SHOVEL here is
	// the singleton-class form, not a constant path.
	if p.accept(token.SHOVEL) {
		target := p.parseTernary()
		p.pushScope() // the singleton-class body has its own local scope
		body := p.parseBodyWithRescue()
		p.popScope()
		p.expect(token.END)
		return &ast.SingletonClassDef{Target: target, Body: body}
	}
	name, path := p.parseConstPath()
	super := ""
	var superExpr ast.Node
	if p.accept(token.LT) {
		// The superclass is any expression (`class A < Base`, `class A < self`,
		// `class A < Struct.new(:x)`, `class A < ns::Base`). A bare constant keeps
		// the plain-name Super form; anything else is recorded as SuperExpr.
		sup := p.parseTernary()
		if c, ok := sup.(*ast.ConstRef); ok {
			super = c.Name
		} else {
			superExpr = sup
		}
	}
	p.pushScope() // a class body has its own local scope
	body := p.parseBodyWithRescue()
	p.popScope()
	p.expect(token.END)
	return &ast.ClassDef{Name: name, NamePath: path, Super: super, SuperExpr: superExpr, Body: body}
}

func (p *Parser) parseModule() ast.Node {
	p.expect(token.MODULE)
	name, path := p.parseConstPath()
	p.pushScope() // a module body has its own local scope
	body := p.parseBodyWithRescue()
	p.popScope()
	p.expect(token.END)
	return &ast.ModuleDef{Name: name, NamePath: path, Body: body}
}

// isDefRecvStart reports whether tt can begin an explicit `def` receiver
// (`def self.x`, `def obj.x`, `def Const.x`, `def @ivar.x`, `def @@c.x`,
// `def $g.x`).
func isDefRecvStart(tt token.Type) bool {
	switch tt {
	case token.SELF, token.IDENT, token.CONST, token.IVAR, token.CVAR, token.GVAR:
		return true
	}
	return false
}

func (p *Parser) parseDef() ast.Node {
	p.expect(token.DEF)
	singleton := false
	var recv ast.Node
	// A receiver before the method name: def self.foo / def obj.foo / def Const.foo
	// / def @ivar.foo / def $g.foo. The kind guard keeps peekTok in range (the
	// receiver is always a single name token followed by a dot).
	if isDefRecvStart(p.cur().Type) && p.peekTok().Type == token.DOT {
		switch p.cur().Type {
		case token.SELF:
			p.advance() // self
			singleton = true
		case token.IDENT: // def obj.foo — singleton method on a local's object
			recv = &ast.VarRef{Name: p.advance().Lit}
		case token.CONST: // def Const.foo — class/module method
			recv = &ast.ConstRef{Name: p.advance().Lit}
		case token.IVAR: // def @ivar.foo — singleton method on an ivar's object
			recv = &ast.IvarRef{Name: p.advance().Lit}
		case token.CVAR:
			recv = &ast.CVarRef{Name: p.advance().Lit}
		case token.GVAR:
			recv = &ast.GVarRef{Name: p.advance().Lit}
		}
		if recv != nil || singleton {
			p.advance() // .
		}
	}
	name, ok := p.parseDefName()
	if !ok {
		p.fail("expected method name after def")
	}
	p.pushScope() // params (and their defaults) live in the method scope
	var params []string
	var defaults []ast.Node
	var kwParams []ast.KwParam
	var kwRest, blockParam string
	forward := false
	splat := -1
	if p.accept(token.LPAREN) {
		params, defaults, splat, kwParams, kwRest, blockParam, forward = p.parseDefParams(token.RPAREN)
		p.expect(token.RPAREN)
	} else if (p.is(token.IDENT) || p.is(token.LABEL) || p.is(token.AMPER) || p.is(token.DOTDOTDOT)) && !p.is(token.NEWLINE) {
		// paren-less params: def foo a, b  /  def foo a:, b: 2  /  def foo &blk
		params, defaults, splat, kwParams, kwRest, blockParam, forward = p.parseDefParams(token.NEWLINE)
	}
	// Endless method definition: def name(params) = expr (no body/end).
	if p.accept(token.ASSIGN) {
		body := []ast.Node{p.parseExprOrAssign()}
		p.popScope()
		return &ast.MethodDef{Name: name, Params: params, Defaults: defaults, SplatIndex: splat, KwParams: kwParams, KwRest: kwRest, BlockParam: blockParam, Singleton: singleton, Recv: recv, Forward: forward, Body: body}
	}
	// A method body may carry rescue/else/ensure clauses without an explicit begin.
	body := p.parseBodyWithRescue()
	p.popScope()
	p.expect(token.END)
	return &ast.MethodDef{Name: name, Params: params, Defaults: defaults, SplatIndex: splat, KwParams: kwParams, KwRest: kwRest, BlockParam: blockParam, Singleton: singleton, Recv: recv, Forward: forward, Body: body}
}

// parseDefName reads the name in a `def`: an identifier/constant, an operator
// method (`<=>`, `<`, `==`, `+`, `<<`, …), or the index methods `[]` / `[]=`.
func (p *Parser) parseDefName() (string, bool) {
	switch p.cur().Type {
	case token.IDENT:
		name := p.advance().Lit
		// Setter method: def name=(value) — the '=' hugs the name (no space).
		if p.is(token.ASSIGN) && !p.cur().SpaceBefore {
			p.advance()
			name += "="
		}
		return name, true
	case token.PLUS, token.MINUS, token.TILDE, token.BANG:
		// Binary `+`/`-` and the unary methods `~`/`!`, plus the unary-operator
		// methods `+@`/`-@`/`~@`/`!@` whose `@` hugs the operator.
		name := p.advance().Lit
		// `+@`/`-@`/`~@`/`!@`: a bare `@` (no following name) hugs the operator. The
		// lexer yields it as an ILLEGAL "@" token (bare `@` is not a valid ivar).
		if p.cur().Lit == "@" && !p.cur().SpaceBefore {
			p.advance()
			name += "@"
		}
		return name, true
	case token.CONST,
		token.SPACESHIP, token.LT, token.GT, token.LE, token.GE,
		token.EQ, token.EQQ, token.NEQ, token.MATCH, token.SHOVEL, token.RSHIFT,
		token.STAR, token.POW, token.SLASH, token.PERCENT,
		token.AMPER, token.PIPE, token.CARET:
		return p.advance().Lit, true
	case token.LBRACKET:
		p.advance()
		p.expect(token.RBRACKET)
		if p.accept(token.ASSIGN) {
			return "[]=", true
		}
		return "[]", true
	case token.XSTRING:
		// The backtick method name `def \`(cmd); end` (lexed as an empty XSTRING).
		p.advance()
		return "`", true
	}
	// A keyword used as a method name: def do / def then / def in / def class …
	// Ruby permits any reserved word in def-name position.
	if _, isKeyword := token.Keywords[p.cur().Lit]; isKeyword {
		return p.advance().Lit, true
	}
	return "", false
}

// parseDefParams parses a method's parameter list, each optionally `name =
// default`. Each parameter is declared before its (and later) defaults are
// parsed, so a default may reference earlier parameters. defaults is parallel to
// params, nil for a required parameter.
func (p *Parser) parseDefParams(until token.Type) (params []string, defaults []ast.Node, splat int, kwParams []ast.KwParam, kwRest, blockParam string, forward bool) {
	splat = -1
	// A parenthesised parameter list may span several lines, with newlines after
	// the open paren and around the separating commas; a paren-less list (until ==
	// NEWLINE) must not skip newlines, as one terminates it.
	paren := until == token.RPAREN
	if paren {
		p.skipNewlines()
	}
	if p.is(until) || p.is(token.NEWLINE) {
		return params, defaults, splat, kwParams, kwRest, blockParam, forward
	}
	for {
		if paren {
			p.skipNewlines()
			if p.is(until) { // a trailing comma before the close paren
				break
			}
		}
		if p.accept(token.DOTDOTDOT) { // `...` argument-forwarding param (always last)
			forward = true
			break
		}
		if p.accept(token.AMPER) { // &block param, or anonymous & (always last)
			// `def f(&)` — an anonymous block param (Ruby 3.1+), forwardable as `&`.
			// It is recorded with the sentinel name "&" (no Ruby local can be named
			// that). A named &block declares the local.
			if p.is(token.IDENT) {
				blockParam = p.advance().Lit
				p.declareLocal(blockParam)
			} else {
				blockParam = "&"
			}
			break
		}
		if p.accept(token.POW) { // **rest keyword-splat, anonymous **, or **nil
			// `def f(**)` — anonymous double-splat (Ruby 3.2+), sentinel name "**".
			// `def f(**nil)` — explicitly no keyword args, recorded as "nil".
			switch {
			case p.is(token.IDENT):
				kwRest = p.advance().Lit
				p.declareLocal(kwRest)
			case p.is(token.NIL):
				p.advance()
				kwRest = "nil"
			default:
				kwRest = "**"
			}
			// A double-splat may be followed by a &block param, so do not break;
			// consume a separating comma and continue, otherwise stop.
			if !p.accept(token.COMMA) {
				break
			}
			continue
		}
		if p.is(token.LABEL) { // keyword param: `a:` (required) or `a: default`
			name := p.advance().Lit
			p.declareLocal(name)
			var def ast.Node
			if !p.is(token.COMMA) && !p.is(until) && !p.is(token.NEWLINE) {
				def = p.parseParamDefault()
			}
			kwParams = append(kwParams, ast.KwParam{Name: name, Default: def})
			if !p.accept(token.COMMA) {
				break
			}
			continue
		}
		if p.accept(token.STAR) { // *rest splat param, or anonymous *
			splat = len(params)
			// `def f(*)` / `def f(a, *)` — anonymous splat (sentinel name "*").
			if p.is(token.IDENT) {
				params = append(params, p.advance().Lit)
				p.declareLocal(params[splat])
			} else {
				params = append(params, "*")
			}
			defaults = append(defaults, nil)
			if !p.accept(token.COMMA) {
				break
			}
			continue
		}
		name := p.expect(token.IDENT).Lit
		params = append(params, name)
		// A parameter is not yet in scope while its own default expression is
		// parsed (MRI's rule), so a default may call a same-named method —
		// `def f(secret = secret("x"))` is the secret() *method*, not a self
		// reference. Declare the local only after the default is consumed; earlier
		// parameters are already declared and so visible to this default.
		if p.accept(token.ASSIGN) {
			defaults = append(defaults, p.parseParamDefault())
		} else {
			defaults = append(defaults, nil)
		}
		p.declareLocal(name)
		if !p.accept(token.COMMA) {
			break
		}
	}
	if paren {
		p.skipNewlines() // tolerate a newline before the close paren
	}
	return params, defaults, splat, kwParams, kwRest, blockParam, forward
}

// parseParamDefault parses a parameter's default-value expression with masgn
// detection suppressed, so the comma that separates parameters is not mistaken
// for a multiple-assignment target separator (`def f(a = c(1), b = nil)`).
func (p *Parser) parseParamDefault() ast.Node {
	saved := p.noMasgn
	p.noMasgn = true
	def := p.parseExprOrAssign()
	p.noMasgn = saved
	return def
}

// parseCond parses an if/unless/while/until condition, where the low-precedence
// keyword operators `and`/`or`/`not` are permitted (`if a and b`, `while x or
// y`, `unless not done`).
func (p *Parser) parseCond() ast.Node { return p.parseKeywordLogical() }

func (p *Parser) parseIf() ast.Node {
	p.expect(token.IF)
	cond := p.parseCond()
	p.accept(token.THEN)
	then := p.parseStatements(ifBodyEnd)
	node := &ast.If{Cond: cond, Then: then}
	for p.is(token.ELSIF) {
		p.advance()
		c := p.parseCond()
		p.accept(token.THEN)
		b := p.parseStatements(ifBodyEnd)
		node.Elsifs = append(node.Elsifs, ast.Elsif{Cond: c, Body: b})
	}
	if p.accept(token.ELSE) {
		node.Else = p.parseStatements(bodyEnd)
	}
	p.expect(token.END)
	return node
}

// parseUnless desugars `unless c ... else ... end` to `if !c ... else ... end`.
func (p *Parser) parseUnless() ast.Node {
	p.expect(token.UNLESS)
	cond := p.parseCond()
	p.accept(token.THEN)
	then := p.parseStatements(ifBodyEnd)
	node := &ast.If{Cond: not(cond), Then: then}
	if p.accept(token.ELSE) {
		node.Else = p.parseStatements(bodyEnd)
	}
	p.expect(token.END)
	return node
}

func (p *Parser) parseWhile() ast.Node {
	p.expect(token.WHILE)
	cond := p.parseLoopCond()
	p.accept(token.DO)
	body := p.parseStatements(bodyEnd)
	p.expect(token.END)
	return &ast.While{Cond: cond, Body: body}
}

// parseLoopCond parses a while/until condition with `do…end` attachment
// suppressed, so a trailing `do` is the loop's, not a call block's.
func (p *Parser) parseLoopCond() ast.Node {
	saved := p.noDo
	p.noDo = true
	cond := p.parseCond()
	p.noDo = saved
	return cond
}

// parseUntil desugars `until c ... end` to `while !c ... end`.
func (p *Parser) parseUntil() ast.Node {
	p.expect(token.UNTIL)
	cond := p.parseLoopCond()
	p.accept(token.DO)
	body := p.parseStatements(bodyEnd)
	p.expect(token.END)
	return &ast.While{Cond: not(cond), Body: body}
}

func (p *Parser) parseReturn() ast.Node {
	p.expect(token.RETURN)
	// A value-less `return` ends at a terminator, a body/block close, or a
	// trailing modifier (`return if c`, `return unless c`) — same rule as
	// break/next. Without this, `return unless x` would parse `unless x … end`
	// as the return value and swallow the matching `end`.
	if p.atStatementEnd() {
		return &ast.Return{}
	}
	first := p.parseExprOrAssign()
	if !p.is(token.COMMA) {
		return &ast.Return{Value: first}
	}
	// `return a, b, …` returns an array of the values.
	elems := []ast.Node{first}
	for p.accept(token.COMMA) {
		elems = append(elems, p.parseExprOrAssign())
	}
	return &ast.Return{Value: &ast.ArrayLit{Elems: elems}}
}

// parseAlias parses `alias NewName OldName`. Each name is a method name (a bare
// identifier/constant/keyword/operator) or a symbol, or — for global aliasing —
// a global variable. The two names are separated by whitespace, not a comma.
func (p *Parser) parseAlias() ast.Node {
	p.expect(token.ALIAS)
	return &ast.Alias{NewName: p.parseFitem(), OldName: p.parseFitem()}
}

// parseUndef parses `undef name [, name…]`, removing the named methods.
func (p *Parser) parseUndef() ast.Node {
	p.expect(token.UNDEF)
	names := []string{p.parseFitem()}
	for p.accept(token.COMMA) {
		names = append(names, p.parseFitem())
	}
	return &ast.Undef{Names: names}
}

// parseFitem reads one method-name item for alias/undef: a symbol (`:foo`,
// `:==`), a global variable (`$x`, alias only), or a bare method name — an
// identifier, a constant, a reserved word, or an operator (`==`, `<=>`, `[]`).
func (p *Parser) parseFitem() string {
	switch p.cur().Type {
	case token.SYMBOL, token.GVAR:
		return p.advance().Lit
	}
	if name, ok := p.parseDefName(); ok {
		return name
	}
	p.fail("expected a method name")
	return ""
}

func (p *Parser) parseBreak() ast.Node {
	p.expect(token.BREAK)
	if p.atStatementEnd() {
		return &ast.Break{}
	}
	return &ast.Break{Value: p.parseJumpValue()}
}

func (p *Parser) parseNext() ast.Node {
	p.expect(token.NEXT)
	if p.atStatementEnd() {
		return &ast.Next{}
	}
	return &ast.Next{Value: p.parseJumpValue()}
}

// parseJumpValue parses the value of a `break`/`next` jump: a single expression,
// or a comma-separated list gathered into an array (`next table, true`,
// `break a, b`), matching `return a, b`.
func (p *Parser) parseJumpValue() ast.Node {
	first := p.parseExprOrAssign()
	if !p.is(token.COMMA) {
		return first
	}
	elems := []ast.Node{first}
	for p.accept(token.COMMA) {
		elems = append(elems, p.parseExprOrAssign())
	}
	return &ast.ArrayLit{Elems: elems}
}

// atStatementEnd reports whether the cursor is at a point where a value-less
// return/break/next ends: a terminator, a block/body close (end, else, elsif,
// `}`, `)`, `]`), or a trailing modifier (if/unless/while/until). The closing
// `)`/`]` let a bare jump end a parenthesised/bracketed group — `( x or next )`,
// `[a or break]` — rather than swallowing the delimiter as its value.
func (p *Parser) atStatementEnd() bool {
	switch p.cur().Type {
	case token.NEWLINE, token.EOF, token.END, token.ELSE, token.ELSIF, token.RBRACE,
		token.RPAREN, token.RBRACKET,
		token.IF, token.UNLESS, token.WHILE, token.UNTIL:
		return true
	}
	return false
}

// parseBegin parses `begin BODY (rescue [Classes] [=> var] BODY)* [else BODY]
// [ensure BODY] end`.
// rescueVarTarget parses the non-local capture target of a `rescue => TARGET`
// clause (an ivar/cvar/gvar/const or an attribute/index target), reusing the
// masgn-target machinery so the resulting node is in the same assignable shape.
func (p *Parser) rescueVarTarget() ast.Node {
	_, tgt, _ := p.parseMlhsTarget()
	return tgt
}

func (p *Parser) parseBegin() ast.Node {
	p.expect(token.BEGIN)
	node := p.parseRescueTail(p.parseStatements(beginBodyEnd))
	p.expect(token.END)
	return node
}

// parseRescueTail parses the `(rescue …)* [else …] [ensure …]` clauses that
// follow an already-parsed body, returning a Begin. The caller consumes `end`.
// It is shared by `begin … end` and method-level rescue (`def … rescue … end`).
func (p *Parser) parseRescueTail(body []ast.Node) *ast.Begin {
	node := &ast.Begin{Body: body}
	for p.is(token.RESCUE) {
		p.advance()
		clause := ast.RescueClause{}
		if !p.is(token.NEWLINE) && !p.is(token.HASHROCKET) && !p.is(token.THEN) {
			// A rescue class list may be a `*splat` of an exception array
			// (`rescue *EXCEPTIONS => e`, `rescue *FOO, Bar`).
			clause.Classes = append(clause.Classes, p.parseSplatOrExpr())
			for p.accept(token.COMMA) {
				clause.Classes = append(clause.Classes, p.parseSplatOrExpr())
			}
		}
		if p.accept(token.HASHROCKET) {
			// The capture target is usually a plain local (`rescue => e`), but may be
			// any assignable: an ivar/cvar/gvar/const or an attribute (`rescue =>
			// @error`, `rescue => $g`, `rescue => obj.err`).
			if p.is(token.IDENT) && !isPostfixStart(p.peekTok().Type) {
				clause.Var = p.advance().Lit
				p.declareLocal(clause.Var)
			} else {
				clause.VarTarget = p.rescueVarTarget()
			}
		}
		p.accept(token.THEN)
		clause.Body = p.parseStatements(beginBodyEnd)
		node.Rescues = append(node.Rescues, clause)
	}
	if p.accept(token.ELSE) {
		node.ElseBody = p.parseStatements(beginBodyEnd)
	}
	if p.accept(token.ENSURE) {
		node.EnsureBody = p.parseStatements(bodyEnd)
	}
	return node
}

// parseBodyWithRescue parses a body that may carry rescue/else/ensure clauses
// directly, without an explicit `begin` — as Ruby allows for method, class,
// module, singleton-class, and do…end-block bodies. With no rescue/else/ensure
// it returns the plain body; otherwise it wraps the body in a Begin. The caller
// consumes the terminating `end`.
func (p *Parser) parseBodyWithRescue() []ast.Node {
	body := p.parseStatements(beginBodyEnd)
	if p.is(token.RESCUE) || p.is(token.ELSE) || p.is(token.ENSURE) {
		return []ast.Node{p.parseRescueTail(body)}
	}
	return body
}

// parseStringConcat parses one or more adjacent string literals and folds them
// into a single node, as MRI concatenates them at parse time: `"a" "b"` == `"ab"`,
// and a `\`-continued line joins too (the lexer already swallows the backslash +
// newline as whitespace). Each piece is a plain STRING or an interpolated
// STRBEG…STREND run. With a single plain piece it returns a StringLit; otherwise
// the pieces' parts are concatenated into one StrInterp (a run that turns out to
// be all-plain is collapsed back to a StringLit).
func (p *Parser) parseStringConcat() ast.Node {
	var pieces []ast.Node
	for p.is(token.STRING) || p.is(token.STRBEG) {
		if p.is(token.STRING) {
			pieces = append(pieces, &ast.StringLit{Value: p.advance().Lit})
		} else {
			pieces = append(pieces, p.parseInterpString())
		}
	}
	if len(pieces) == 1 {
		return pieces[0]
	}
	// Concatenate: flatten every piece's parts into one interpolation, then
	// collapse to a plain StringLit when no interpolation survives.
	var parts []ast.Node
	for _, pc := range pieces {
		switch n := pc.(type) {
		case *ast.StringLit:
			parts = append(parts, n)
		case *ast.StrInterp:
			parts = append(parts, n.Parts...)
		}
	}
	if lit, ok := mergeStringParts(parts); ok {
		return lit
	}
	return &ast.StrInterp{Parts: parts}
}

// mergeStringParts returns a single StringLit when every part is a plain string
// literal (so an all-plain concatenation like `"a" "b"` yields `"ab"`), else
// (nil, false).
func mergeStringParts(parts []ast.Node) (*ast.StringLit, bool) {
	var b strings.Builder
	for _, pt := range parts {
		s, ok := pt.(*ast.StringLit)
		if !ok {
			return nil, false
		}
		b.WriteString(s.Value)
	}
	return &ast.StringLit{Value: b.String()}, true
}

// parseInterpString assembles an interpolated string from the lexer's
// STRBEG (literal …#{) / expr / STRMID (}…#{) / … / STREND (}…") sequence.
func (p *Parser) parseInterpString() ast.Node {
	parts := []ast.Node{&ast.StringLit{Value: p.advance().Lit}} // STRBEG
	for {
		parts = append(parts, p.parseInterpBody())
		t := p.advance()
		parts = append(parts, &ast.StringLit{Value: t.Lit})
		if t.Type == token.STREND {
			break
		}
		if t.Type != token.STRMID {
			p.fail("malformed string interpolation")
		}
	}
	return &ast.StrInterp{Parts: parts}
}

// interpEnd marks the tokens that close a `#{…}` interpolation body.
var interpEnd = map[token.Type]bool{token.STRMID: true, token.STREND: true}

// parseInterpBody parses the contents of one `#{…}` interpolation, which is a
// full statement sequence (so it admits trailing modifiers, `#{'s' if n > 1}`,
// and several `;`-separated statements). Its value is the last statement; an
// empty body (`#{}`) is nil.
func (p *Parser) parseInterpBody() ast.Node {
	stmts := p.parseStatements(interpEnd)
	switch len(stmts) {
	case 0:
		return &ast.NilLit{}
	case 1:
		return stmts[0]
	default:
		return &ast.Begin{Body: stmts}
	}
}

// parseCase parses either `case [subj] (when …)* [else] end` or the pattern
// form `case subj (in PATTERN [guard])* [else] end`. The first clause keyword
// (when vs in) selects the form.
func (p *Parser) parseCase() ast.Node {
	p.expect(token.CASE)
	var subject ast.Node
	if !p.is(token.NEWLINE) {
		subject = p.parseExprOrAssign()
	}
	p.skipNewlines()
	if p.is(token.IN) {
		return p.parseCaseIn(subject)
	}
	node := &ast.Case{Subject: subject}
	for p.is(token.WHEN) {
		p.advance()
		// A `when` list may contain `*splat` of an array of candidate values
		// (`when *LIST`, `when 1, *rest`).
		clause := ast.WhenClause{Conds: []ast.Node{p.parseSplatOrExpr()}}
		for p.accept(token.COMMA) {
			clause.Conds = append(clause.Conds, p.parseSplatOrExpr())
		}
		p.accept(token.THEN)
		clause.Body = p.parseStatements(caseBodyEnd)
		node.Whens = append(node.Whens, clause)
	}
	if p.accept(token.ELSE) {
		node.Else = p.parseStatements(bodyEnd)
	}
	p.expect(token.END)
	return node
}

// parseCaseIn parses the `(in PATTERN [guard])* [else body] end` tail of a
// pattern-matching case, with the subject already consumed.
func (p *Parser) parseCaseIn(subject ast.Node) ast.Node {
	node := &ast.CaseIn{Subject: subject}
	for p.is(token.IN) {
		p.advance()
		clause := ast.InClause{Pattern: p.parsePattern()}
		switch {
		case p.accept(token.IF):
			clause.Guard = p.parseExprOrAssign()
		case p.accept(token.UNLESS):
			clause.Guard = p.parseExprOrAssign()
			clause.GuardNeg = true
		}
		p.accept(token.THEN)
		clause.Body = p.parseStatements(inBodyEnd)
		node.Clauses = append(node.Clauses, clause)
	}
	if p.accept(token.ELSE) {
		node.Else = p.parseStatements(bodyEnd)
	}
	p.expect(token.END)
	return node
}

// parsePattern parses a top-level pattern. The top level also accepts a bare
// comma-separated array pattern (`in a, b`, `in *a, b`), the implicit array
// form.
func (p *Parser) parsePattern() ast.Pattern {
	// A leading label is an implicit (brace-less) hash pattern: `in a:, b:`.
	if p.is(token.LABEL) || p.is(token.POW) {
		return p.parseHashPatternBody(nil, token.NEWLINE)
	}
	first := p.parseArrayPatternElem()
	if !first.splat && !p.is(token.COMMA) {
		return first.pat
	}
	// Implicit (un-bracketed) array pattern: `in a, b` ≡ `in [a, b]`.
	return p.parseArrayPatternRest(nil, first)
}

// parsePatternPrimary parses a single pattern element, applying a trailing
// `=> name` binding suffix.
func (p *Parser) parsePatternPrimary() ast.Pattern {
	first := p.parsePatternSuffixed()
	if !p.is(token.PIPE) {
		return first
	}
	// `p1 | p2 | …` — alternative pattern (matches if any alternative does).
	alts := []ast.Pattern{first}
	for p.accept(token.PIPE) {
		alts = append(alts, p.parsePatternSuffixed())
	}
	return &ast.AltPattern{Alts: alts}
}

// parsePatternSuffixed parses one pattern atom with an optional trailing
// `=> name` binding suffix.
func (p *Parser) parsePatternSuffixed() ast.Pattern {
	p.patternDepth++
	pat := p.parsePatternAtom()
	p.patternDepth--
	if p.accept(token.HASHROCKET) {
		name := p.expect(token.IDENT).Lit
		p.declareLocal(name)
		pat = &ast.BindingPattern{Sub: pat, Name: name}
	}
	return pat
}

// parsePatternAtom parses one pattern without the `=> name` suffix.
func (p *Parser) parsePatternAtom() ast.Pattern {
	switch p.cur().Type {
	case token.CARET:
		// Pin: `^name` / `^(expr)` matches the subject against an existing value
		// (with ===), instead of binding — i.e. a value pattern over that expr.
		p.advance()
		if p.accept(token.LPAREN) {
			e := p.parseExprOrAssign()
			p.expect(token.RPAREN)
			return &ast.ValuePattern{Value: e}
		}
		return &ast.ValuePattern{Value: &ast.VarRef{Name: p.expect(token.IDENT).Lit}}
	case token.LBRACKET:
		return p.parseArrayPattern(nil)
	case token.LBRACE:
		return p.parseHashPattern(nil)
	case token.IDENT:
		// A bare lowercase identifier binds the subject (the wildcard `_`
		// included; it binds in MRI too).
		name := p.advance().Lit
		p.declareLocal(name)
		return &ast.BindPattern{Name: name}
	case token.CONST, token.SCOPE:
		// A (possibly scope-resolved) constant pattern: `Point`, `Prism::CaseNode`,
		// `::Foo`. Point[...] is a const array pattern (deconstruct); Point(x:, y:)
		// is a const hash pattern (deconstruct_keys); otherwise the constant is a
		// class match.
		c := p.parsePatternConst()
		if p.is(token.LBRACKET) {
			return p.parseArrayPattern(c)
		}
		if p.accept(token.LPAREN) {
			hp := p.parseHashPatternBody(c, token.RPAREN)
			p.expect(token.RPAREN)
			return hp
		}
		return &ast.ConstPattern{Const: c}
	default:
		// Everything else is a value pattern matched with `===`: literals and
		// ranges (true/false/nil/Integer/Float/String/Symbol/range).
		return &ast.ValuePattern{Value: p.parsePatternValue()}
	}
}

// parsePatternConst parses the constant reference at the head of a constant
// pattern, including a scope-resolution path (`Prism::CaseNode`) and a leading
// top-level `::Foo`.
func (p *Parser) parsePatternConst() ast.Node {
	var node ast.Node
	if p.accept(token.SCOPE) {
		node = &ast.ScopedConst{Name: p.expect(token.CONST).Lit, Global: true}
	} else {
		node = &ast.ConstRef{Name: p.expect(token.CONST).Lit}
	}
	for p.is(token.SCOPE) && p.peekTok().Type == token.CONST {
		p.advance() // ::
		node = &ast.ScopedConst{Recv: node, Name: p.advance().Lit}
	}
	return node
}

// parsePatternValue parses the expression behind a value pattern: a literal or
// a range of literals (`1..5`, `..10`, `1..`).
func (p *Parser) parsePatternValue() ast.Node {
	return p.parseRange()
}

// parseArrayPattern parses `[pat, …]`, with an optional leading constant. When
// the bracket body begins with a label or `**`, it is a hash pattern written
// with bracket delimiters (`Const[key:]`, deconstruct_keys), which MRI accepts.
func (p *Parser) parseArrayPattern(constName ast.Node) ast.Pattern {
	p.expect(token.LBRACKET)
	if p.is(token.LABEL) || p.is(token.POW) {
		hp := p.parseHashPatternBody(constName, token.RBRACKET)
		p.expect(token.RBRACKET)
		return hp
	}
	var elems []arrayElem
	if !p.accept(token.RBRACKET) {
		elems = append(elems, p.parseArrayPatternElem())
		for p.accept(token.COMMA) {
			if p.is(token.RBRACKET) { // trailing comma
				break
			}
			elems = append(elems, p.parseArrayPatternElem())
		}
		p.expect(token.RBRACKET)
	}
	return p.buildArrayOrFind(constName, elems)
}

// parseHashPattern parses a braced hash pattern `{ key: [pat], …, **rest }`,
// with an optional leading constant.
func (p *Parser) parseHashPattern(constName ast.Node) ast.Pattern {
	p.expect(token.LBRACE)
	hp := p.parseHashPatternBody(constName, token.RBRACE)
	p.expect(token.RBRACE)
	return hp
}

// parseHashPatternBody parses the comma-separated entries of a hash pattern up
// to (but not consuming) end. Entries are `label: [value-pattern]` or the
// double-splat `**name` / `**nil`.
func (p *Parser) parseHashPatternBody(constName ast.Node, end token.Type) *ast.HashPattern {
	hp := &ast.HashPattern{Const: constName}
	for !p.is(end) && !p.is(token.NEWLINE) && !p.is(token.THEN) && !p.is(token.IF) && !p.is(token.UNLESS) {
		if p.accept(token.POW) { // **rest / **nil
			if hp.HasRest || hp.RestNil {
				p.fail("unexpected ** in hash pattern")
			}
			if p.accept(token.NIL) {
				hp.RestNil = true
			} else {
				hp.HasRest = true
				if p.is(token.IDENT) {
					hp.RestName = p.advance().Lit
					p.declareLocal(hp.RestName)
				}
			}
		} else {
			key := p.expect(token.LABEL).Lit
			hp.Keys = append(hp.Keys, key)
			// `name:` with no following pattern binds local `name`; otherwise the
			// value is a sub-pattern. A clause/entry boundary means no value.
			if p.is(token.COMMA) || p.is(end) || p.is(token.NEWLINE) || p.is(token.THEN) || p.is(token.IF) || p.is(token.UNLESS) {
				p.declareLocal(key)
				hp.Values = append(hp.Values, nil)
			} else {
				hp.Values = append(hp.Values, p.parsePatternPrimary())
			}
		}
		if !p.accept(token.COMMA) {
			break
		}
	}
	return hp
}

// parseArrayPatternRest builds an implicit (top-level, un-bracketed) array
// pattern from an already-parsed first element and the comma-separated tail,
// terminating at a clause boundary rather than a bracket.
func (p *Parser) parseArrayPatternRest(constName ast.Node, first arrayElem) ast.Pattern {
	elems := []arrayElem{first}
	for p.accept(token.COMMA) {
		elems = append(elems, p.parseArrayPatternElem())
	}
	return p.buildArrayOrFind(constName, elems)
}

// arrayElem is one parsed array-pattern element: an ordinary sub-pattern, or a
// `*[name]` splat marker (splat true, with the optional capture name).
type arrayElem struct {
	pat   ast.Pattern
	splat bool
	name  string
}

// parseArrayPatternElem parses one element of an array pattern, which may be a
// `*[name]` splat.
func (p *Parser) parseArrayPatternElem() arrayElem {
	if p.accept(token.STAR) {
		name := ""
		if p.is(token.IDENT) {
			name = p.advance().Lit
			p.declareLocal(name)
		}
		return arrayElem{splat: true, name: name}
	}
	return arrayElem{pat: p.parsePatternPrimary()}
}

// buildArrayOrFind turns the parsed elements into an array pattern, or a find
// pattern when exactly two splats bracket the elements (`[*pre, mid…, *post]`).
func (p *Parser) buildArrayOrFind(constName ast.Node, elems []arrayElem) ast.Pattern {
	splats := 0
	for _, e := range elems {
		if e.splat {
			splats++
		}
	}
	if splats == 2 && len(elems) > 2 && elems[0].splat && elems[len(elems)-1].splat {
		fp := &ast.FindPattern{Const: constName, PreName: elems[0].name, PostName: elems[len(elems)-1].name}
		for _, e := range elems[1 : len(elems)-1] {
			fp.Mid = append(fp.Mid, e.pat)
		}
		return fp
	}
	if splats > 1 {
		p.fail("unexpected multiple * in array pattern")
	}
	ap := &ast.ArrayPattern{Const: constName}
	for _, e := range elems {
		switch {
		case e.splat:
			ap.HasSplat = true
			ap.SplatName = e.name
		case ap.HasSplat:
			ap.Post = append(ap.Post, e.pat)
		default:
			ap.Pre = append(ap.Pre, e.pat)
		}
	}
	return ap
}

// --- expressions ---

// looksLikeMlhs scans ahead (without consuming) for a multiple-assignment left
// side: `[*]TARGET (, [*]TARGET)* =` with at least one top-level comma. A target
// may be a local (IDENT), a constant (CONST), an instance/class/global variable
// (IVAR/CVAR/GVAR), or an attribute / index target (`obj.x`, `arr[i]`, with
// nested calls, scope-resolution, and brackets). The scan tracks bracket and
// paren depth so commas and the trailing `=` are only recognized at the top
// level; it succeeds only when a top-level `=` is reached after a top-level
// comma. A lone `*TARGET =` (one splat target, no comma) is also a masgn.
func (p *Parser) looksLikeMlhs() bool {
	i := p.pos
	sawComma := false
	sawSplat := false
	sawGroup := false
	for {
		// Optional leading splat on this target.
		if p.toks[i].Type == token.STAR {
			i++
			sawSplat = true
			// A nameless splat target (`*`, as in `a, * = x` or `* = x`) is followed
			// directly by `,` or `=`.
			switch p.toks[i].Type {
			case token.COMMA:
				sawComma = true
				i++
				continue
			case token.ASSIGN:
				return sawComma || sawSplat
			}
		}
		// A trailing comma before `=` ends the target list: `a, = x`.
		if (sawComma || sawGroup) && p.toks[i].Type == token.ASSIGN {
			return true
		}
		// A parenthesized nested target group: `(a, b) = …`, `((a, b), c) = …`,
		// `(a, b), c = …`. A `(` here begins a target, not a postfix call (which
		// only follows a name). Scan the balanced group, then a `,`/`=` must follow.
		if p.toks[i].Type == token.LPAREN {
			j := p.scanBalanced(i, token.LPAREN, token.RPAREN)
			if j < 0 {
				return false
			}
			sawGroup = true
			i = j
			switch p.toks[i].Type {
			case token.COMMA:
				sawComma = true
				i++
				continue
			case token.ASSIGN:
				return true
			default:
				return false
			}
		}
		// A target must start with one of these kinds. A leading `::` begins a
		// top-level scoped target (`::Time.zone = …`).
		switch p.toks[i].Type {
		case token.IDENT, token.CONST, token.IVAR, token.CVAR, token.GVAR, token.SELF:
		case token.SCOPE:
			i++
			if p.toks[i].Type != token.CONST {
				return false
			}
		default:
			return false
		}
		i++
		// Consume any postfix chain of this target at the top level: `.name`,
		// `::Name`, `(...)`, `[...]`. Brackets/parens are scanned with depth so a
		// comma or `=` inside them is not mistaken for a top-level one.
		i = p.scanMlhsTargetTail(i)
		if i < 0 {
			return false
		}
		switch p.toks[i].Type {
		case token.COMMA:
			sawComma = true
			i++
		case token.ASSIGN:
			return sawComma || sawSplat
		default:
			return false
		}
	}
}

// scanMlhsTargetTail advances past the postfix part of a single masgn target
// starting at token index i (`.name`, `::Name`, balanced `(...)`/`[...]`),
// returning the index of the first token that is not part of the target, or -1
// on a malformed run (e.g. unbalanced brackets running off the end).
func (p *Parser) scanMlhsTargetTail(i int) int {
	for {
		switch p.toks[i].Type {
		case token.DOT, token.SAFEDOT, token.SCOPE:
			i++
			switch p.toks[i].Type {
			case token.IDENT, token.CONST:
				i++
			default:
				return -1
			}
		case token.LBRACKET:
			j := p.scanBalanced(i, token.LBRACKET, token.RBRACKET)
			if j < 0 {
				return -1
			}
			i = j
		case token.LPAREN:
			j := p.scanBalanced(i, token.LPAREN, token.RPAREN)
			if j < 0 {
				return -1
			}
			i = j
		default:
			return i
		}
	}
}

// scanBalanced returns the index just past a balanced open/close pair beginning
// at index i (which must hold the open token), or -1 if the input ends first. It
// counts only the matching open/close kinds so other delimiters inside are
// skipped over.
func (p *Parser) scanBalanced(i int, open, close token.Type) int {
	depth := 0
	for {
		switch p.toks[i].Type {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return i + 1
			}
		case token.EOF:
			return -1
		}
		i++
	}
}

// parseMlhs parses a multiple assignment whose targets may be locals, constants,
// instance/class/global variables, or attribute / index targets, with at most
// one *splat target. When every target is a plain local the result uses only
// Names (and Targets stays nil); otherwise it additionally fills Targets
// (parallel to Names) with the per-position LHS node so a consumer can store into
// each. For a non-local target Names[i] is "" (a splat-captured local keeps its
// name).
func (p *Parser) parseMlhs() ast.Node {
	var names []string
	var targets []ast.Node
	onlyLocals := true
	splat := -1
	for {
		isSplat := p.accept(token.STAR)
		if isSplat {
			splat = len(names)
		}
		// A bare `*` (nameless splat target) is valid: `a, * = list`. Only when a
		// target token follows do we parse one.
		if isSplat && (p.is(token.COMMA) || p.is(token.ASSIGN)) {
			names = append(names, "")
			targets = append(targets, nil)
			onlyLocals = false
		} else {
			name, tgt, local := p.parseMlhsTarget()
			names = append(names, name)
			targets = append(targets, tgt)
			if !local {
				onlyLocals = false
			}
		}
		if !p.accept(token.COMMA) {
			break
		}
		// A trailing comma before `=` (`a, = x`) ends the target list.
		if p.is(token.ASSIGN) {
			break
		}
	}
	if onlyLocals {
		targets = nil // all-locals fast path: Names alone suffices
	}
	p.expect(token.ASSIGN)
	values := []ast.Node{p.parseMasgnValue()}
	for p.accept(token.COMMA) {
		values = append(values, p.parseMasgnValue())
	}
	return &ast.MultiAssign{Names: names, Targets: targets, SplatIndex: splat, Values: values}
}

// parseMlhsGroup parses a parenthesized nested masgn target `(t1, t2, …)` and
// returns it as a *MultiAssign with Values left nil (it captures one destructured
// value rather than driving its own RHS). It reuses the same per-target grammar,
// so nesting (`((a, b), c)`) and an inner splat (`(a, *b)`) work.
func (p *Parser) parseMlhsGroup() ast.Node {
	p.expect(token.LPAREN)
	var names []string
	var targets []ast.Node
	splat := -1
	for {
		isSplat := p.accept(token.STAR)
		if isSplat {
			splat = len(names)
		}
		if isSplat && (p.is(token.COMMA) || p.is(token.RPAREN)) {
			names = append(names, "")
			targets = append(targets, nil)
		} else {
			name, tgt, _ := p.parseMlhsTarget()
			names = append(names, name)
			targets = append(targets, tgt)
		}
		if !p.accept(token.COMMA) {
			break
		}
		if p.is(token.RPAREN) { // trailing comma `(a, b,)`
			break
		}
	}
	p.expect(token.RPAREN)
	return &ast.MultiAssign{Names: names, Targets: targets, SplatIndex: splat}
}

// parseSplatOrExpr parses an expression that may be prefixed by a `*splat`,
// spreading an array in a position that accepts several values (a `when`
// candidate list, a `rescue` class list).
func (p *Parser) parseSplatOrExpr() ast.Node {
	if p.accept(token.STAR) {
		return &ast.SplatArg{Value: p.parseExprOrAssign()}
	}
	return p.parseExprOrAssign()
}

// parseMasgnValue parses one right-hand value of a multiple assignment, which
// may be a `*splat` whose array is spread across the targets (`a, b = *args`,
// `x, y = 1, *rest`) or a further (chained) assignment whose value is then
// destructured (`a, b = c = [1, 2]`, `_, h, _ = resp = call(x)`).
func (p *Parser) parseMasgnValue() ast.Node {
	if p.accept(token.STAR) {
		return &ast.SplatArg{Value: p.parseTernary()}
	}
	return p.parseExprOrAssign()
}

// parseMlhsTarget parses one masgn target and returns (localName, targetNode,
// isPlainLocal). For a plain local, localName is its name, targetNode is a
// *VarRef, and isPlainLocal is true. For any other target (constant, ivar,
// cvar, gvar, attribute, or index) localName is "" and targetNode is the LHS
// node a consumer stores into. Attribute targets are emitted as the existing
// setter-call shape (Call with Name "x=" / "[]=") so consumers reuse their
// single-assignment store logic.
func (p *Parser) parseMlhsTarget() (string, ast.Node, bool) {
	// Parenthesized nested target group: `(a, b) = …`, `((x, y), z) = …`. The
	// group destructures one value into its own sub-targets; it is represented as
	// a nested *MultiAssign with no Values (Values stays nil), stored as a target.
	if p.is(token.LPAREN) {
		return "", p.parseMlhsGroup(), false
	}
	// Simple local: declare it and use a *VarRef (fast path).
	if p.is(token.IDENT) && !isPostfixStart(p.peekTok().Type) {
		name := p.advance().Lit
		p.declareLocal(name)
		return name, &ast.VarRef{Name: name}, true
	}
	node := p.parsePostfix()
	switch n := node.(type) {
	case *ast.ConstRef, *ast.ScopedConst, *ast.IvarRef, *ast.CVarRef, *ast.GVarRef:
		return "", node, false
	case *ast.Call:
		// Attribute / index target with an explicit receiver: rewrite to its
		// setter-call shape, leaving the value argument to be appended by the
		// store. recv[i] → recv.[]=(i), recv.attr → recv.attr=(). A receiver-less
		// call (a bare funcall) is not an assignable target.
		if n.Recv != nil {
			if n.Name == "[]" {
				n.Name = "[]="
			} else {
				n.Name += "="
			}
			return "", n, false
		}
	}
	p.fail("unexpected masgn target")
	return "", nil, false
}

// isPostfixStart reports whether tt can begin a postfix operator chain (so a
// bare IDENT followed by it is an attribute/index target, not a plain local).
func isPostfixStart(tt token.Type) bool {
	switch tt {
	case token.DOT, token.SAFEDOT, token.SCOPE, token.LBRACKET:
		return true
	}
	return false
}

func (p *Parser) parseExprOrAssign() ast.Node {
	// A `not EXPR` prefix in value position (assignment RHS, call argument, array
	// element): `x = not true`, `foo(not flag)`. `not` binds looser than the
	// binary operators, so it wraps the whole following expression.
	if p.accept(token.NOT) {
		return not(p.parseExprOrAssign())
	}
	// Multiple assignment to local targets: a, b = … / a, *b = … . Disabled while
	// parsing a parameter default, where a comma separates parameters.
	if !p.noMasgn && p.looksLikeMlhs() {
		return p.parseMlhs()
	}
	// Simple local assignment: IDENT '=' expr (right-associative, chainable).
	if p.is(token.IDENT) && p.peekTok().Type == token.ASSIGN {
		name := p.advance().Lit
		p.expect(token.ASSIGN)
		val := p.parseAssignRhs()
		p.declareLocal(name)
		return &ast.Assign{Name: name, Value: val}
	}
	// Constant assignment: NAME '=' expr.
	if p.is(token.CONST) && p.peekTok().Type == token.ASSIGN {
		name := p.advance().Lit
		p.expect(token.ASSIGN)
		return &ast.ConstAssign{Name: name, Value: p.parseAssignRhs()}
	}
	// Instance-variable assignment: @name '=' expr.
	if p.is(token.IVAR) && p.peekTok().Type == token.ASSIGN {
		name := p.advance().Lit
		p.expect(token.ASSIGN)
		return &ast.IvarAssign{Name: name, Value: p.parseAssignRhs()}
	}
	// Class-variable assignment: @@name '=' expr.
	if p.is(token.CVAR) && p.peekTok().Type == token.ASSIGN {
		name := p.advance().Lit
		p.expect(token.ASSIGN)
		return &ast.CVarAssign{Name: name, Value: p.parseAssignRhs()}
	}
	// Global-variable assignment: $name '=' expr.
	if p.is(token.GVAR) && p.peekTok().Type == token.ASSIGN {
		name := p.advance().Lit
		p.expect(token.ASSIGN)
		return &ast.GVarAssign{Name: name, Value: p.parseAssignRhs()}
	}
	// Compound assignment to a local / ivar: LHS OP= expr → LHS = LHS OP expr.
	if p.is(token.IDENT) && p.peekTok().Type == token.OPASSIGN {
		name := p.advance().Lit
		op := p.advance().Lit
		rhs := p.parseExprOrAssign()
		p.declareLocal(name)
		return &ast.OpAssign{Name: name, Op: op, Value: rhs}
	}
	if p.is(token.IVAR) && p.peekTok().Type == token.OPASSIGN {
		name := p.advance().Lit
		op := p.advance().Lit
		rhs := p.parseExprOrAssign()
		return &ast.IvarAssign{Name: name, Value: &ast.BinaryExpr{Op: op, Left: &ast.IvarRef{Name: name}, Right: rhs}}
	}
	// Compound assignment to a global: $name OP= expr → $name = $name OP expr.
	if p.is(token.GVAR) && p.peekTok().Type == token.OPASSIGN {
		name := p.advance().Lit
		op := p.advance().Lit
		rhs := p.parseExprOrAssign()
		return &ast.GVarAssign{Name: name, Value: &ast.BinaryExpr{Op: op, Left: &ast.GVarRef{Name: name}, Right: rhs}}
	}
	// Compound assignment to a class variable: @@name OP= expr.
	if p.is(token.CVAR) && p.peekTok().Type == token.OPASSIGN {
		name := p.advance().Lit
		op := p.advance().Lit
		rhs := p.parseExprOrAssign()
		return &ast.CVarAssign{Name: name, Value: &ast.BinaryExpr{Op: op, Left: &ast.CVarRef{Name: name}, Right: rhs}}
	}
	left := p.parseTernary()
	if p.is(token.OPASSIGN) {
		if call, ok := left.(*ast.Call); ok && call.Recv != nil {
			// Compound index assignment: recv[i] OP= v → recv[i] = recv[i] OP v.
			if call.Name == "[]" {
				op := p.advance().Lit
				rhs := p.parseExprOrAssign()
				read := &ast.Call{Recv: call.Recv, Name: "[]", Args: call.Args}
				newVal := &ast.BinaryExpr{Op: op, Left: read, Right: rhs}
				args := append(append([]ast.Node{}, call.Args...), newVal)
				return &ast.Call{Recv: call.Recv, Name: "[]=", Args: args}
			}
			// Compound attribute assignment: recv.a OP= v → recv.a = recv.a OP v.
			if len(call.Args) == 0 && call.Block == nil {
				op := p.advance().Lit
				rhs := p.parseExprOrAssign()
				read := &ast.Call{Recv: call.Recv, Name: call.Name}
				newVal := &ast.BinaryExpr{Op: op, Left: read, Right: rhs}
				return &ast.Call{Recv: call.Recv, Name: call.Name + "=", Args: []ast.Node{newVal}}
			}
		}
	}
	if p.is(token.ASSIGN) {
		if call, ok := left.(*ast.Call); ok && call.Recv != nil {
			// Index assignment: recv[i] = v  →  recv.[]=(i, v). A comma-separated
			// right-hand side becomes an implicit array (`h[k] = 1, 2`).
			if call.Name == "[]" {
				p.advance()
				call.Name = "[]="
				call.Args = append(call.Args, p.parseAssignRhs())
				return call
			}
			// Attribute assignment: recv.attr = v  →  recv.attr=(v). A comma list on
			// the right is gathered into an array (`self.cache_store = :s, path`).
			if len(call.Args) == 0 && call.Block == nil {
				p.advance()
				call.Name += "="
				call.Args = []ast.Node{p.parseAssignRhs()}
				return call
			}
		}
		// Scope-resolved constant assignment: `A::B = v`, `::Top = v`.
		if sc, ok := left.(*ast.ScopedConst); ok {
			p.advance()
			return &ast.ScopedConstAssign{Target: sc, Value: p.parseAssignRhs()}
		}
	}
	return p.withRescueModifier(left)
}

// parseAssignRhs parses the right-hand side of a single-target assignment. It is
// usually one expression (chainable: `a = b = 1`), but a top-level comma — or a
// leading `*splat` — makes the RHS an implicit array: `x = 1, 2, 3` ≡ `x = [1,
// 2, 3]` and `a = *list, y` ≡ `a = [*list, y]`, matching MRI.
func (p *Parser) parseAssignRhs() ast.Node {
	first := p.parseRhsElem()
	if !p.is(token.COMMA) {
		// A bare leading `*splat` with no comma still yields a one-element array
		// (`a = *list` ≡ `a = [*list]`), as MRI does.
		if sp, ok := first.(*ast.SplatArg); ok {
			return &ast.ArrayLit{Elems: []ast.Node{sp}}
		}
		return first
	}
	elems := []ast.Node{first}
	for p.accept(token.COMMA) {
		// `x = 1, 2,` — a trailing comma before the terminator is allowed.
		if p.atStatementEnd() {
			break
		}
		elems = append(elems, p.parseRhsElem())
	}
	return &ast.ArrayLit{Elems: elems}
}

// parseRhsElem parses one element of an assignment right-hand side, which may be
// a `*splat`.
func (p *Parser) parseRhsElem() ast.Node {
	if p.accept(token.STAR) {
		return &ast.SplatArg{Value: p.parseTernary()}
	}
	return p.parseExprOrAssign()
}

// withRescueModifier consumes a trailing `rescue FALLBACK` on the same line (the
// modifier form: `risky rescue default`), wrapping node so a StandardError it
// raises yields FALLBACK instead. It binds tighter than `=` because it is
// applied here, to an assignment's right-hand side, before the Assign is built.
// A clause `rescue` (in a begin/def body) always starts a new line, so the
// preceding NEWLINE keeps it from being read as a modifier. Left-associative.
func (p *Parser) withRescueModifier(node ast.Node) ast.Node {
	for p.is(token.RESCUE) {
		p.advance()
		fallback := p.parseTernary()
		node = &ast.Begin{Body: []ast.Node{node}, Rescues: []ast.RescueClause{{Body: []ast.Node{fallback}}}}
	}
	return node
}

// parseTernary handles `cond ? then : else`, binding looser than ranges/binary
// operators but tighter than assignment, and right-associative. It desugars to
// an If expression.
func (p *Parser) parseTernary() ast.Node {
	cond := p.parseRange()
	if !p.accept(token.QUESTION) {
		return cond
	}
	// Each branch may itself be an assignment (`c ? a = b : d`,
	// `x ? ENV["k"] = v : super`), which MRI permits in this position even though
	// `=` otherwise binds looser than `?:`.
	then := p.maybeInlineAssign(p.parseTernary())
	p.expect(token.COLON)
	els := p.maybeInlineAssign(p.parseTernary())
	return &ast.If{Cond: cond, Then: []ast.Node{then}, Else: []ast.Node{els}}
}

// parseRange handles `lo..hi` / `lo...hi`, binding looser than the binary
// operators but tighter than assignment.
func (p *Parser) parseRange() ast.Node {
	if p.is(token.DOTDOT) || p.is(token.DOTDOTDOT) { // beginless: ..hi / ...hi
		excl := p.is(token.DOTDOTDOT)
		p.advance()
		return &ast.RangeLit{Hi: p.parseBinary(0), Exclusive: excl}
	}
	left := p.parseBinary(0)
	if p.is(token.DOTDOT) || p.is(token.DOTDOTDOT) {
		excl := p.is(token.DOTDOTDOT)
		p.advance()
		var hi ast.Node // endless when nothing can follow the dots
		if !rangeHiEnds[p.cur().Type] {
			hi = p.parseBinary(0)
		}
		return &ast.RangeLit{Lo: left, Hi: hi, Exclusive: excl}
	}
	return left
}

// Binding powers for infix operators (higher binds tighter).
func binBP(tt token.Type) int {
	switch tt {
	case token.OROR:
		return 4
	case token.ANDAND:
		return 6
	case token.EQ, token.EQQ, token.NEQ, token.SPACESHIP, token.MATCH, token.NMATCH:
		return 10
	case token.LT, token.GT, token.LE, token.GE:
		return 20
	case token.PIPE, token.CARET:
		return 22
	case token.AMPER:
		return 24
	case token.SHOVEL, token.RSHIFT:
		return 25
	case token.PLUS, token.MINUS:
		return 30
	case token.STAR, token.SLASH, token.PERCENT:
		return 40
	case token.POW:
		return 50
	}
	return 0
}

func (p *Parser) parseBinary(minBP int) ast.Node {
	left := p.parseUnary()
	for {
		tt := p.cur().Type
		bp := binBP(tt)
		// Inside a pattern, a top-level `|` separates alternatives rather than
		// being the bitwise-or operator.
		if (p.patternDepth > 0 || p.noPipe) && tt == token.PIPE {
			bp = 0
		}
		if bp == 0 || bp <= minBP {
			return left
		}
		op := p.advance().Lit
		rbp := bp
		if op == "**" { // exponentiation is right-associative
			rbp = bp - 1
		}
		// A value-less control-flow jump may be the right operand of `&&`/`||`:
		// `x || return`, `expr && next`, `y || break` (MRI accepts the jump only
		// without a value here — `x || return 5` is a syntax error).
		if jump, ok := p.valuelessJumpOperand(); ok {
			left = &ast.BinaryExpr{Op: op, Left: left, Right: jump}
			return left
		}
		right := p.maybeInlineAssign(p.parseBinary(rbp))
		left = &ast.BinaryExpr{Op: op, Left: left, Right: right}
	}
}

// valuelessJumpOperand parses a value-less control-flow jump (`return`, `break`,
// `next`) sitting in operand position, returning its node and true. MRI accepts a
// bare jump as the right operand of `&&`/`||` (`x || return`, `expr && next`) but
// not one carrying a value (`x || return 5` is a syntax error), nor a `retry`
// (`x || retry` is a syntax error), so only these argument-less forms qualify.
func (p *Parser) valuelessJumpOperand() (ast.Node, bool) {
	switch p.cur().Type {
	case token.RETURN:
		p.advance()
		return &ast.Return{}, true
	case token.BREAK:
		p.advance()
		return &ast.Break{}, true
	case token.NEXT:
		p.advance()
		return &ast.Next{}, true
	}
	return nil, false
}

// maybeInlineAssign turns a freshly-parsed operand into an assignment when an
// `=` follows it and it names an assignable target. This lets an assignment sit
// as a sub-expression inside a condition or larger expression — `if a && b = c`,
// `while x && line = gets` — which MRI permits even though `=` otherwise binds
// looser than the binary operators. (A top-level `lhs = rhs` is already handled
// in parseExprOrAssign; this covers the nested operand case.) `==`/`=>`/`=~` are
// their own tokens, so only a real assignment `=` is consumed here.
func (p *Parser) maybeInlineAssign(node ast.Node) ast.Node {
	// A compound assignment may also sit as a nested operand: `a && count += 1`,
	// `x > 0 && h[k] -= 1`, `cond && precision ||= 1`. Desugar it the same way the
	// statement-level OP= paths do.
	if p.is(token.OPASSIGN) {
		if assigned := p.inlineOpAssign(node); assigned != nil {
			return assigned
		}
		return node
	}
	if !p.is(token.ASSIGN) {
		return node
	}
	switch n := node.(type) {
	case *ast.VarRef:
		p.advance()
		p.declareLocal(n.Name)
		return &ast.Assign{Name: n.Name, Value: p.parseAssignRhs()}
	case *ast.Call:
		// A receiver-less, argument-less bare call is really a not-yet-seen local:
		// `b = c` where b was unknown. Receiver attribute/index targets become the
		// setter-call shape, matching parseExprOrAssign's handling.
		if n.Recv == nil && len(n.Args) == 0 && n.Block == nil {
			p.advance()
			p.declareLocal(n.Name)
			return &ast.Assign{Name: n.Name, Value: p.parseAssignRhs()}
		}
		if n.Recv != nil && n.Block == nil {
			if n.Name == "[]" {
				p.advance()
				n.Name = "[]="
				n.Args = append(n.Args, p.parseExprOrAssign())
				return n
			}
			if len(n.Args) == 0 {
				p.advance()
				n.Name += "="
				n.Args = []ast.Node{p.parseExprOrAssign()}
				return n
			}
		}
	case *ast.IvarRef:
		p.advance()
		return &ast.IvarAssign{Name: n.Name, Value: p.parseAssignRhs()}
	case *ast.CVarRef:
		p.advance()
		return &ast.CVarAssign{Name: n.Name, Value: p.parseAssignRhs()}
	case *ast.GVarRef:
		p.advance()
		return &ast.GVarAssign{Name: n.Name, Value: p.parseAssignRhs()}
	case *ast.ConstRef:
		p.advance()
		return &ast.ConstAssign{Name: n.Name, Value: p.parseAssignRhs()}
	}
	return node
}

// inlineOpAssign desugars a compound assignment `target OP= rhs` whose target is
// the already-parsed node, returning the assignment node, or nil if the node is
// not an assignable target (so the caller leaves it untouched). It mirrors the
// statement-level OP= handling for each target kind.
func (p *Parser) inlineOpAssign(node ast.Node) ast.Node {
	switch n := node.(type) {
	case *ast.VarRef:
		op := p.advance().Lit
		rhs := p.parseExprOrAssign()
		p.declareLocal(n.Name)
		return &ast.OpAssign{Name: n.Name, Op: op, Value: rhs}
	case *ast.Call:
		if n.Recv == nil && len(n.Args) == 0 && n.Block == nil { // a bare local
			op := p.advance().Lit
			rhs := p.parseExprOrAssign()
			p.declareLocal(n.Name)
			return &ast.OpAssign{Name: n.Name, Op: op, Value: rhs}
		}
		if n.Recv != nil && n.Block == nil {
			// recv[i] OP= v → recv[i] = recv[i] OP v; recv.a OP= v → recv.a = recv.a OP v.
			if n.Name == "[]" {
				op := p.advance().Lit
				rhs := p.parseExprOrAssign()
				read := &ast.Call{Recv: n.Recv, Name: "[]", Args: n.Args}
				newVal := &ast.BinaryExpr{Op: op, Left: read, Right: rhs}
				args := append(append([]ast.Node{}, n.Args...), newVal)
				return &ast.Call{Recv: n.Recv, Name: "[]=", Args: args}
			}
			if len(n.Args) == 0 {
				op := p.advance().Lit
				rhs := p.parseExprOrAssign()
				read := &ast.Call{Recv: n.Recv, Name: n.Name}
				newVal := &ast.BinaryExpr{Op: op, Left: read, Right: rhs}
				return &ast.Call{Recv: n.Recv, Name: n.Name + "=", Args: []ast.Node{newVal}}
			}
		}
	case *ast.IvarRef:
		op := p.advance().Lit
		rhs := p.parseExprOrAssign()
		return &ast.IvarAssign{Name: n.Name, Value: &ast.BinaryExpr{Op: op, Left: &ast.IvarRef{Name: n.Name}, Right: rhs}}
	case *ast.CVarRef:
		op := p.advance().Lit
		rhs := p.parseExprOrAssign()
		return &ast.CVarAssign{Name: n.Name, Value: &ast.BinaryExpr{Op: op, Left: &ast.CVarRef{Name: n.Name}, Right: rhs}}
	case *ast.GVarRef:
		op := p.advance().Lit
		rhs := p.parseExprOrAssign()
		return &ast.GVarAssign{Name: n.Name, Value: &ast.BinaryExpr{Op: op, Left: &ast.GVarRef{Name: n.Name}, Right: rhs}}
	}
	return nil
}

// negateLiteral returns the negation of a numeric literal node. The MINUS path
// in parseUnary reaches here only after parsePrimary consumed an INT or FLOAT
// token, which yields exactly one of these three kinds: a FLOAT is always a
// FloatLit, and an INT is an IntLit or — when it overflows int64, e.g.
// -9999999999999999999999999999999 — a BignumLit. (An INT with invalid digits
// fails inside parsePrimary and never returns here.)
func negateLiteral(n ast.Node) ast.Node {
	switch lit := n.(type) {
	case *ast.IntLit:
		return &ast.IntLit{Value: -lit.Value}
	case *ast.BignumLit:
		return &ast.BignumLit{Val: new(big.Int).Neg(lit.Val)}
	}
	fl := n.(*ast.FloatLit)
	return &ast.FloatLit{Value: -fl.Value}
}

func (p *Parser) parseUnary() ast.Node {
	switch p.cur().Type {
	case token.MINUS:
		p.advance()
		// A minus directly before a numeric literal forms a negative literal:
		// -2.abs is (-2).abs, but ** binds tighter so -2**x means -(2**x).
		if p.is(token.INT) || p.is(token.FLOAT) {
			lit := p.parsePrimary()
			if p.is(token.POW) {
				p.advance()
				right := p.parseBinary(binBP(token.POW) - 1)
				return &ast.UnaryExpr{Op: "-", Operand: &ast.BinaryExpr{Op: "**", Left: lit, Right: right}}
			}
			return p.parsePostfixTail(negateLiteral(lit))
		}
		return &ast.UnaryExpr{Op: "-", Operand: p.parseUnary()}
	case token.PLUS:
		p.advance()
		return p.parseUnary() // unary plus is a no-op
	case token.BANG:
		p.advance()
		return &ast.UnaryExpr{Op: "!", Operand: p.parseUnary()}
	case token.TILDE:
		p.advance()
		return &ast.UnaryExpr{Op: "~", Operand: p.parseUnary()}
	}
	return p.parsePostfix()
}

func (p *Parser) parsePostfix() ast.Node {
	return p.parsePostfixTail(p.parsePrimary())
}

// parsePostfixTail applies postfix operators (.method, [index], block) to an
// already-parsed primary. Split out so a negative numeric literal can carry its
// own postfix chain (e.g. -2.abs == (-2).abs).
func (p *Parser) parsePostfixTail(node ast.Node) ast.Node {
	for {
		switch {
		case p.is(token.DOT) || p.is(token.SAFEDOT):
			safe := p.advance().Type == token.SAFEDOT
			// `.()` shorthand: `recv.(args)` desugars to `recv.call(args)`. The
			// '(' hugs the dot (no method name token), so read it directly.
			if p.is(token.LPAREN) && !p.cur().SpaceBefore {
				p.advance()
				args := p.parseCallArgs(token.RPAREN)
				p.expect(token.RPAREN)
				node = &ast.Call{Recv: node, Name: "call", Args: args, Safe: safe}
				break
			}
			name := p.methodName()
			var args []ast.Node
			if p.is(token.LPAREN) && !p.cur().SpaceBefore {
				p.advance()
				args = p.parseCallArgs(token.RPAREN)
				p.expect(token.RPAREN)
			} else if p.canStartCommandArg() || p.atHuggingStringArg() {
				// Paren-less command call on a receiver: `obj.foo bar`,
				// `Fiber.yield 1`, or a string hugging the method name
				// (`obj.m"x"`). The space-separated argument list terminates
				// this postfix chain (it greedily consumes the rest). A trailing
				// `do…end` then binds to this command call (`obj.foo bar do … end`),
				// as MRI attaches it to the outermost command rather than to an
				// argument call.
				call := &ast.Call{Recv: node, Name: name, Args: p.parseCommandArgs(), Safe: safe}
				if p.is(token.DO) && !p.noDo {
					call.Block = p.parseDoBlock()
					// The block-bearing command call may itself be chained
					// (`obj.foo bar do … end.baz`), so continue the postfix loop.
					node = call
					break
				}
				return call
			}
			node = &ast.Call{Recv: node, Name: name, Args: args, Safe: safe}
		case p.is(token.SCOPE):
			p.advance()
			// `Const::Name(args)` — a capitalized scope-resolution method call:
			// the `(` hugging the name (no space) means a send, not a constant
			// (`Syslog::LOG_UPTO(Syslog::LOG_INFO)`). A bare `Const::Name` is a
			// scoped constant (`Math::PI`).
			if p.is(token.CONST) && p.peekTok().Type == token.LPAREN && !p.peekTok().SpaceBefore {
				name := p.advance().Lit
				p.advance() // consume '('
				args := p.parseCallArgs(token.RPAREN)
				p.expect(token.RPAREN)
				node = &ast.Call{Recv: node, Name: name, Args: args}
				break
			}
			if p.is(token.CONST) {
				name := p.advance().Lit
				// A capitalized scope-resolution name with a space-separated command
				// argument is a method call (`obj::Down x, y`); otherwise it is a
				// scoped constant (`Math::PI`).
				if p.canStartCommandArg() || p.atHuggingStringArg() {
					call := &ast.Call{Recv: node, Name: name, Args: p.parseCommandArgs()}
					if p.is(token.DO) && !p.noDo {
						call.Block = p.parseDoBlock()
						node = call
						break
					}
					return call
				}
				node = &ast.ScopedConst{Recv: node, Name: name}
				break
			}
			// Foo::bar(args) or `Mod::meth arg` — a method call, like the dot form.
			name := p.methodName()
			var args []ast.Node
			if p.is(token.LPAREN) && !p.cur().SpaceBefore {
				p.advance()
				args = p.parseCallArgs(token.RPAREN)
				p.expect(token.RPAREN)
			} else if p.canStartCommandArg() || p.atHuggingStringArg() {
				// Paren-less scope-resolution command call: `Mod::meth arg`.
				call := &ast.Call{Recv: node, Name: name, Args: p.parseCommandArgs()}
				if p.is(token.DO) && !p.noDo {
					call.Block = p.parseDoBlock()
					node = call
					break
				}
				return call
			}
			node = &ast.Call{Recv: node, Name: name, Args: args}
		case p.is(token.LBRACKET): // index: recv[args] → recv.[](args)
			p.advance()
			args := p.parseCallArgs(token.RBRACKET)
			p.expect(token.RBRACKET)
			node = &ast.Call{Recv: node, Name: "[]", Args: args}
		case p.is(token.LBRACE) || (p.is(token.DO) && !p.noDo):
			// A block binds to the immediately preceding method call; chaining
			// then continues (`recv.map { … }.join`). A `super` also takes a block
			// (`super { … }`, `super(x) do … end`).
			var blockSlot **ast.Block
			switch n := node.(type) {
			case *ast.Call:
				if n.Block == nil {
					blockSlot = &n.Block
				}
			case *ast.Super:
				if n.Block == nil {
					blockSlot = &n.Block
				}
			}
			if blockSlot == nil {
				return node
			}
			if p.is(token.LBRACE) {
				*blockSlot = p.parseBraceBlock()
			} else {
				*blockSlot = p.parseDoBlock()
			}
		default:
			return node
		}
	}
}

// parseArrayLiteral parses `[a, b, c]` (a trailing comma and newlines are
// allowed).
func (p *Parser) parseArrayLiteral() ast.Node {
	p.expect(token.LBRACKET)
	// An array literal shares the argument grammar so trailing `key: value` (and
	// the value-omitted `key:` shorthand) collapse into one implicit trailing Hash
	// element, exactly as in a call: `[a, k: v]` == `[a, {k: v}]`.
	elems := p.parseCallArgs(token.RBRACKET)
	p.expect(token.RBRACKET)
	return &ast.ArrayLit{Elems: elems}
}

// parseHashLiteral parses `{ k => v, … }` (hashrocket form). A `{` only reaches
// here at expression-start; a `{` after a call is a block (see parsePostfix).
func (p *Parser) parseHashLiteral() ast.Node {
	p.expect(token.LBRACE)
	h := &ast.HashLit{}
	p.skipNewlines()
	for !p.is(token.RBRACE) {
		var k, v ast.Node
		if p.accept(token.POW) {
			// `**expr` — a double-splat entry (nil key signals a merge).
			h.Keys = append(h.Keys, nil)
			h.Values = append(h.Values, p.parseExprOrAssign())
			p.skipNewlines()
			if !p.accept(token.COMMA) {
				break
			}
			p.skipNewlines()
			continue
		}
		if p.is(token.LABEL) {
			// `name: value` — the label is sugar for a symbol key.
			name := p.advance().Lit
			k = &ast.SymbolLit{Name: name}
			if p.is(token.COMMA) || p.is(token.RBRACE) || p.is(token.NEWLINE) {
				v = p.barewordValue(name) // `{x:}` shorthand == `{x: x}`
			} else {
				v = p.parseExprOrAssign()
			}
		} else {
			k = p.parseExprOrAssign()
			// A quoted string key followed by `:` is a symbol key (`{'a': 1}`,
			// `{"d-a-s-h": 2}`, `{"a#{x}": 3}`), the quoted form of a `name:` label.
			if sym, ok := p.stringKeyColon(k); ok {
				k = sym
				v = p.parseExprOrAssign()
			} else {
				p.expect(token.HASHROCKET)
				v = p.parseExprOrAssign()
			}
		}
		h.Keys = append(h.Keys, k)
		h.Values = append(h.Values, v)
		p.skipNewlines()
		if !p.accept(token.COMMA) {
			break
		}
		p.skipNewlines()
	}
	p.expect(token.RBRACE)
	return h
}

// stringKeyColon recognises the quoted-symbol-key shorthand: a string-literal
// key immediately followed by `:` denotes a symbol key, the quoted analogue of a
// `name:` label (`{'a': 1}`, `{"d-a-s-h": 2}`, `tag("@click": "f")`). It returns
// the symbol-key node and true when key is a string literal and the cursor is on
// the separating `:` (consuming it); otherwise it returns (nil, false), leaving
// the cursor untouched so the caller falls back to the `=>` form. A plain string
// becomes a SymbolLit; an interpolated string becomes a dynamic `"…".to_sym`.
func (p *Parser) stringKeyColon(key ast.Node) (ast.Node, bool) {
	if !p.is(token.COLON) {
		return nil, false
	}
	switch s := key.(type) {
	case *ast.StringLit:
		p.advance() // ':'
		return &ast.SymbolLit{Name: s.Value}, true
	case *ast.StrInterp:
		p.advance() // ':'
		return &ast.Call{Recv: s, Name: "to_sym"}, true
	}
	return nil, false
}

// parseLambda parses a stabby lambda `->(params) { body }` / `-> { body }` /
// `->(params) do body end`, desugaring it to `lambda { |params| body }`.
func (p *Parser) parseLambda() ast.Node {
	p.expect(token.ARROW)
	p.pushBlockScope()
	var params []string
	var defaults, prepends []ast.Node
	var blockParam string
	splat := -1
	if p.accept(token.LPAREN) {
		// The stabby-lambda `(...)` list shares the block parameter grammar, so it
		// supports the same optional, *splat, &block, and destructuring forms.
		params, defaults, prepends, splat, blockParam = p.parseBlockParams(token.RPAREN)
		p.expect(token.RPAREN)
		p.scope().explicitParams = true
	} else if p.is(token.IDENT) || p.is(token.STAR) || p.is(token.POW) || p.is(token.AMPER) {
		// Unparenthesized parameters: `->x { }`, `-> ctx { }`, `->a, b { }`,
		// `-> message do … end`. The list runs up to the block opener (`{`/`do`);
		// parseBlockParams stops at the first token that is not a parameter.
		params, defaults, prepends, splat, blockParam = p.parseBlockParams(token.LBRACE)
		p.scope().explicitParams = true
	}
	bs := p.scope()
	var body []ast.Node
	if p.accept(token.DO) {
		body = p.parseStatements(bodyEnd)
		p.expect(token.END)
	} else {
		p.expect(token.LBRACE)
		body = p.parseStatements(braceBlockEnd)
		p.expect(token.RBRACE)
	}
	params = p.finishImplicitParams(bs, params)
	p.popScope()
	if len(prepends) > 0 {
		body = append(prepends, body...)
	}
	return &ast.Call{Name: "lambda", Block: &ast.Block{Params: params, Defaults: defaults, SplatIndex: splat, BlockParam: blockParam, Body: body}}
}

// parseBraceBlock parses `{ [|params|] body }`.
func (p *Parser) parseBraceBlock() *ast.Block {
	p.expect(token.LBRACE)
	return p.parseBlockRest(map[token.Type]bool{token.RBRACE: true}, token.RBRACE, false)
}

// parseDoBlock parses `do [|params|] body end`. A do…end block body may carry
// rescue/else/ensure clauses without an explicit begin (unlike a brace block).
func (p *Parser) parseDoBlock() *ast.Block {
	p.expect(token.DO)
	return p.parseBlockRest(beginBodyEnd, token.END, true)
}

// parseBlockRest parses a block's optional `|params|` and body, having already
// consumed the opener. end is the closing token; stop marks where the body
// stops. When withRescue is set (a do…end block), a trailing rescue/else/ensure
// run on the body is folded into a Begin, mirroring method/class bodies.
func (p *Parser) parseBlockRest(stop map[token.Type]bool, end token.Type, withRescue bool) *ast.Block {
	p.pushBlockScope()
	var params []string
	var defaults, prepends []ast.Node
	var blockParam string
	splat := -1
	// The `|params|` list may start on the line(s) after the block opener:
	// `foo {`<nl>`|x| … }`, `do`<nl>`|a, b| … end`. Skip the intervening newlines
	// before the leading `|` (a body statement can never begin with a bare `|`, so
	// this is unambiguous). The newlines are no-ops for the body either way.
	if p.is(token.NEWLINE) && p.firstSignificantIs(token.PIPE) {
		p.skipNewlines()
	}
	if p.accept(token.PIPE) {
		params, defaults, prepends, splat, blockParam = p.parseBlockParams(token.PIPE)
		p.expect(token.PIPE)
		p.scope().explicitParams = true
	}
	bs := p.scope()
	// A block body is a fresh statement context: a `do…end` inside it attaches
	// normally even when this block is itself a paren-less command argument whose
	// own trailing `do` was being held back (`include Module.new { … define_method
	// (:x) do … end }`). Clear noDo for the duration of the body.
	savedNoDo := p.noDo
	p.noDo = false
	body := p.parseStatements(stop)
	p.noDo = savedNoDo
	if withRescue && (p.is(token.RESCUE) || p.is(token.ELSE) || p.is(token.ENSURE)) {
		body = []ast.Node{p.parseRescueTail(body)}
	}
	params = p.finishImplicitParams(bs, params)
	p.popScope()
	p.expect(end)
	// A `(a, b)` group param destructures its (array) argument: it becomes a
	// synthetic param plus a leading multiple-assignment at the top of the body.
	if len(prepends) > 0 {
		body = append(prepends, body...)
	}
	return &ast.Block{Params: params, Defaults: defaults, SplatIndex: splat, BlockParam: blockParam, Body: body}
}

// parseBlockParams parses a block's parameter list (the `|...|` form for brace/do
// blocks and the `(...)` form for stabby lambdas), where each parameter is either
// a plain name, an optional `name = default` param, a top-level `*rest` splat, or
// a destructuring group `(a, b)` / `(a, *b)`. A group yields a synthetic flat
// parameter and an mlhs prepend that unpacks it. defaults parallels names: it is
// nil for a required, splat, or group param and the default expression for an
// optional one — mirroring how parseDefParams records method-parameter defaults.
func (p *Parser) parseBlockParams(until token.Type) (names []string, defaults, prepends []ast.Node, splat int, blockParam string) {
	splat = -1
	if p.is(until) || p.is(token.NEWLINE) {
		return names, defaults, prepends, splat, blockParam
	}
	group := 0
	for {
		if p.accept(token.AMPER) { // &block param (always last), mirroring def params
			// `->(&) {}` — anonymous block param, sentinel name "&".
			if p.is(token.IDENT) {
				blockParam = p.advance().Lit
				p.declareLocal(blockParam)
			} else {
				blockParam = "&"
			}
			break
		}
		if p.accept(token.POW) { // **rest keyword-splat / anonymous ** (lambda params)
			// parseBlockParams folds a keyword-splat into the splat slot using the
			// sentinel name "**rest"/"**"; a consumer treats it as the kwrest. (Block
			// kwargs are rare; record it as a trailing rest so the shape round-trips.)
			if p.is(token.IDENT) {
				name := p.advance().Lit
				p.declareLocal(name)
				names = append(names, "**"+name)
			} else {
				names = append(names, "**")
			}
			defaults = append(defaults, nil)
			if !p.accept(token.COMMA) {
				break
			}
			continue
		}
		if p.is(token.LABEL) { // keyword block param: |a, b:| / |c: nil, d: 5|
			// Block keyword parameters are recorded with a trailing-colon sentinel
			// name ("b:") so the shape round-trips distinctly from a positional one;
			// the local is declared under its bare name. A value after the label is
			// the keyword default, else the keyword is required.
			name := p.advance().Lit
			p.declareLocal(name)
			names = append(names, name+":")
			if p.is(token.COMMA) || p.is(until) || p.is(token.NEWLINE) {
				defaults = append(defaults, nil)
			} else {
				saved := p.noPipe
				p.noPipe = until == token.PIPE
				defaults = append(defaults, p.parseExprOrAssign())
				p.noPipe = saved
			}
			if !p.accept(token.COMMA) {
				break
			}
			continue
		}
		if p.is(token.STAR) { // top-level rest param: |*rest| / |a, *rest| / (*rest)
			p.advance()
			if splat >= 0 {
				p.fail("two rest parameters are not allowed")
			}
			splat = len(names)
			// `->(*) {}` / `(a, *)` — anonymous splat, sentinel name "*".
			if p.is(token.IDENT) {
				name := p.advance().Lit
				names = append(names, name)
				p.declareLocal(name)
			} else {
				names = append(names, "*")
			}
			defaults = append(defaults, nil)
		} else if p.accept(token.LPAREN) {
			var gnames []string
			gsplat := -1
			for {
				if p.accept(token.STAR) {
					gsplat = len(gnames)
				}
				gn := p.expect(token.IDENT).Lit
				gnames = append(gnames, gn)
				p.declareLocal(gn)
				if !p.accept(token.COMMA) {
					break
				}
			}
			p.expect(token.RPAREN)
			syn := "(" + strconv.Itoa(group) + ")"
			group++
			names = append(names, syn)
			defaults = append(defaults, nil)
			p.declareLocal(syn)
			prepends = append(prepends, &ast.MultiAssign{Names: gnames, SplatIndex: gsplat, Values: []ast.Node{&ast.VarRef{Name: syn}}})
		} else {
			name := p.expect(token.IDENT).Lit
			names = append(names, name)
			p.declareLocal(name)
			if p.accept(token.ASSIGN) { // optional param: |a, b = 5| / (a, b = 5)
				// In the `|...|` form the closing `|` would otherwise be lexed as a
				// bitwise-or continuing the default, so suppress it; the `(...)` form
				// (stabby lambda) keeps `|` as the real operator.
				saved := p.noPipe
				p.noPipe = until == token.PIPE
				defaults = append(defaults, p.parseExprOrAssign())
				p.noPipe = saved
			} else {
				defaults = append(defaults, nil)
			}
		}
		if !p.accept(token.COMMA) {
			break
		}
	}
	return names, defaults, prepends, splat, blockParam
}

// parseYield parses `yield`, `yield(...)`, or `yield args`.
func (p *Parser) parseYield() ast.Node {
	p.expect(token.YIELD)
	if p.is(token.LPAREN) && !p.cur().SpaceBefore {
		p.advance()
		args := p.parseCallArgs(token.RPAREN)
		p.expect(token.RPAREN)
		return &ast.Yield{Args: args}
	}
	if p.canStartCommandArg() {
		return &ast.Yield{Args: p.parseCommandArgs()}
	}
	return &ast.Yield{}
}

// methodName reads a method name after a '.': an identifier, a constant, or a
// keyword used as a method (e.g. `obj.class`, `x.then`, `a.nil?`).
func (p *Parser) methodName() string {
	t := p.cur()
	if t.Type == token.IDENT || t.Type == token.CONST {
		p.advance()
		return t.Lit
	}
	switch t.Type {
	// Operator methods called explicitly: 1.+(2), a.<=>(b), obj.&(x), …
	case token.SPACESHIP, token.LT, token.GT, token.LE, token.GE, token.EQ, token.EQQ, token.NEQ,
		token.SHOVEL, token.RSHIFT, token.PLUS, token.MINUS, token.STAR, token.SLASH, token.PERCENT, token.POW,
		token.AMPER, token.PIPE, token.CARET, token.TILDE, token.MATCH, token.NMATCH, token.BANG,
		token.XSTRING:
		// XSTRING here is an empty backtick literal `` produced when `` ` `` names the
		// backtick method (`def \`(cmd); end`, `obj.\``).
		p.advance()
		return "`"
	}
	// The index methods `[]` / `[]=` named explicitly: `x&.[](i)`, `arr.[]=(i, v)`.
	if t.Type == token.LBRACKET && p.peekTok().Type == token.RBRACKET {
		p.advance() // [
		p.advance() // ]
		if p.is(token.ASSIGN) && !p.cur().SpaceBefore {
			p.advance()
			return "[]="
		}
		return "[]"
	}
	if _, isKeyword := token.Keywords[t.Lit]; isKeyword {
		p.advance()
		return t.Lit
	}
	p.fail("expected method name after '.'")
	return ""
}

// applyNumSuffix wraps a numeric literal in the rational/imaginary nodes named
// by a numeric suffix ("r", "i", or "ri"); an empty suffix returns base as-is.
func applyNumSuffix(base ast.Node, suffix string) ast.Node {
	for _, c := range suffix {
		switch c {
		case 'r':
			base = &ast.RationalLit{Value: base}
		case 'i':
			base = &ast.ImaginaryLit{Value: base}
		}
	}
	return base
}

func (p *Parser) parsePrimary() ast.Node {
	t := p.cur()
	switch t.Type {
	case token.INT:
		p.advance()
		// Base 0 decodes the radix prefix (0x/0o/0b) and treats a bare leading
		// zero as octal, matching Ruby.
		var base ast.Node
		n, err := strconv.ParseInt(t.Lit, 0, 64)
		if err != nil {
			if z, ok := new(big.Int).SetString(t.Lit, 0); ok {
				base = &ast.BignumLit{Val: z} // valid digits, out of int64 range
			} else {
				p.fail("invalid integer literal: %s", t.Lit) // e.g. an invalid octal 08
			}
		} else {
			base = &ast.IntLit{Value: n}
		}
		return applyNumSuffix(base, t.Flags)
	case token.FLOAT:
		p.advance()
		f, _ := strconv.ParseFloat(t.Lit, 64)
		return applyNumSuffix(&ast.FloatLit{Value: f}, t.Flags)
	case token.STRING, token.STRBEG:
		return p.parseStringConcat()
	case token.SYMBOL:
		p.advance()
		return &ast.SymbolLit{Name: t.Lit}
	case token.REGEXP:
		p.advance()
		return &ast.RegexpLit{Source: t.Lit, Flags: t.Flags}
	case token.XSTRING:
		p.advance()
		return &ast.XStr{Command: t.Lit}
	case token.WORDS, token.SYMBOLS:
		p.advance()
		elems := []ast.Node{}
		for _, w := range strings.Fields(t.Lit) {
			if t.Type == token.SYMBOLS {
				elems = append(elems, &ast.SymbolLit{Name: w})
			} else {
				elems = append(elems, &ast.StringLit{Value: w})
			}
		}
		return &ast.ArrayLit{Elems: elems}
	case token.LBRACKET:
		return p.parseArrayLiteral()
	case token.LBRACE:
		return p.parseHashLiteral()
	case token.ARROW:
		return p.parseLambda()
	case token.TRUE:
		p.advance()
		return &ast.BoolLit{Value: true}
	case token.FALSE:
		p.advance()
		return &ast.BoolLit{Value: false}
	case token.NIL:
		p.advance()
		return &ast.NilLit{}
	case token.SELF:
		p.advance()
		return &ast.SelfLit{}
	case token.SUPER:
		return p.parseSuper()
	case token.YIELD:
		return p.parseYield()
	case token.LPAREN:
		p.advance()
		// A parenthesised group is a full statement sequence: it permits the
		// low-precedence keyword operators (`(a and b)`, `(not x)`), trailing
		// modifiers (`(expr if cond)`, `(x if y) || z`), and several
		// semicolon/newline-separated statements (`(a; b)`), evaluating to the
		// last. A single statement returns directly; a sequence is wrapped in a
		// Begin (whose value is its last expression).
		stmts := p.parseStatements(map[token.Type]bool{token.RPAREN: true})
		p.expect(token.RPAREN)
		if len(stmts) == 1 {
			return stmts[0]
		}
		return &ast.Begin{Body: stmts}
	case token.IDENT:
		return p.parseIdentExpr()
	case token.CONST:
		p.advance()
		if p.is(token.LPAREN) && !p.cur().SpaceBefore { // capitalized method call, e.g. Integer("42")
			p.advance()
			args := p.parseCallArgs(token.RPAREN)
			p.expect(token.RPAREN)
			return &ast.Call{Name: t.Lit, Args: args}
		}
		// A string hugging the constant is a paren-less call: `Integer"42"`.
		if p.atHuggingStringArg() {
			return &ast.Call{Name: t.Lit, Args: p.parseCommandArgs()}
		}
		// A space-separated command argument makes the constant a method call:
		// `BigDecimal "0.01"`, `Integer str`. Restricted to unambiguous starts (a
		// string/number/symbol value) so a bare `Foo` followed by an unrelated
		// token still reads as a constant reference.
		if p.constCommandArgFollows() {
			return &ast.Call{Name: t.Lit, Args: p.parseCommandArgs()}
		}
		return &ast.ConstRef{Name: t.Lit}
	case token.SCOPE:
		// Leading `::Name` — a top-level constant lookup (`::Foo`, `defined?(::Foo)`).
		p.advance()
		return &ast.ScopedConst{Name: p.expect(token.CONST).Lit, Global: true}
	case token.IVAR:
		p.advance()
		return &ast.IvarRef{Name: t.Lit}
	case token.GVAR:
		p.advance()
		return &ast.GVarRef{Name: t.Lit}
	case token.CVAR:
		p.advance()
		return &ast.CVarRef{Name: t.Lit}
	case token.DEF:
		// `def` in expression position evaluates to the defined method's name as a
		// symbol; it appears as a command argument (`private def foo; end`).
		return p.parseDef()
	case token.CLASS:
		// A class definition used as an rvalue (`c = class Foo < Bar; …; end`,
		// `sc = class << obj; self; end`) evaluates to the body's last value.
		return p.parseClass()
	case token.MODULE:
		// A module definition as an rvalue (`m = module M; …; end`).
		return p.parseModule()
	case token.BEGIN:
		return p.parseBegin()
	case token.CASE:
		return p.parseCase()
	case token.IF:
		return p.parseIf()
	case token.UNLESS:
		return p.parseUnless()
	case token.WHILE:
		return p.parseWhile()
	case token.UNTIL:
		return p.parseUntil()
	}
	return p.fail("unexpected token %q (%s)", t.Lit, t.Type)
}

// parseSuper parses `super`, `super(...)`, or `super args`. A bare `super`
// forwards the enclosing method's arguments.
func (p *Parser) parseSuper() ast.Node {
	p.expect(token.SUPER)
	if p.is(token.LPAREN) && !p.cur().SpaceBefore {
		p.advance()
		args := p.parseCallArgs(token.RPAREN)
		p.expect(token.RPAREN)
		return &ast.Super{Args: args}
	}
	if p.canStartCommandArg() {
		return &ast.Super{Args: p.parseCommandArgs()}
	}
	return &ast.Super{Forward: true}
}

// parseIdentExpr resolves a bare identifier into a variable read or a method
// call, including paren-less command calls (`puts 1 + 2`).
func (p *Parser) parseIdentExpr() ast.Node {
	name := p.cur().Lit
	next := p.peekTok()

	// foo(...) — paren call (the '(' must hug the name).
	if next.Type == token.LPAREN && !next.SpaceBefore {
		p.advance() // name
		p.advance() // (
		args := p.parseCallArgs(token.RPAREN)
		p.expect(token.RPAREN)
		return &ast.Call{Name: name, Args: args}
	}

	// A string literal hugging the name (no space) is a paren-less argument:
	// `step"Ensure ..." do … end`, `assert"x", y`, `foo"a"`. MRI reads this as a
	// command call even when the name is otherwise a local — `x"y"` is `x("y")`.
	// So this is checked before the local-variable resolution below.
	if p.huggingStringArg() {
		p.advance() // name
		call := &ast.Call{Name: name, Args: p.parseCommandArgs()}
		if p.is(token.DO) && !p.noDo {
			call.Block = p.parseDoBlock()
		}
		return call
	}

	// Known local variable → read.
	if p.is(token.IDENT) && p.isLocal(name) {
		p.advance()
		return &ast.VarRef{Name: name}
	}

	// Numbered implicit block parameter (_1.._9) inside a param-less block.
	if n := numberedParam(name); n > 0 {
		if s := p.implicitParamScope(); s != nil {
			if n > s.maxNum {
				s.maxNum = n
			}
			p.advance()
			return &ast.VarRef{Name: name}
		}
	}

	// Otherwise it is a method call on self.
	p.advance()
	if p.canStartCommandArg() {
		return &ast.Call{Name: name, Args: p.parseCommandArgs()}
	}
	// Bare `it` (no receiver, no args) inside a param-less block is the implicit
	// single parameter (Ruby 3.4). With args/parens it stays a method call.
	if name == "it" {
		if s := p.implicitParamScope(); s != nil {
			s.usedIt = true
			return &ast.VarRef{Name: name}
		}
	}
	return &ast.Call{Name: name}
}

// huggingStringArg reports whether the token after the current name is a string
// literal that hugs it (no intervening space), making the name a command call
// with that string as its first argument: `foo"bar"`, `step"x" do…end`. A
// space-separated string is handled by the ordinary canStartCommandArg path.
func (p *Parser) huggingStringArg() bool { return isHuggingString(p.peekTok()) }

// atHuggingStringArg is huggingStringArg for a name already consumed: the cursor
// itself is the would-be string argument (used after a `.method` in a postfix
// chain, `obj.m"x"`).
func (p *Parser) atHuggingStringArg() bool { return isHuggingString(p.cur()) }

// isHuggingString reports whether t is a string literal with no preceding space.
func isHuggingString(t token.Token) bool {
	return !t.SpaceBefore && (t.Type == token.STRING || t.Type == token.STRBEG)
}

// constCommandArgFollows reports whether a constant is immediately followed by a
// space-separated literal value, making it a conversion-style command call
// (`BigDecimal "0.01"`, `Integer "42"`, `Float 1`). It is deliberately narrow —
// only a literal string/number/symbol argument — so an ordinary `Foo` reference
// next to other tokens (a binary operator, `.method`, `[`, a newline) is not
// mistaken for a call.
func (p *Parser) constCommandArgFollows() bool {
	t := p.cur()
	if !t.SpaceBefore {
		return false
	}
	switch t.Type {
	case token.STRING, token.STRBEG, token.INT, token.FLOAT, token.SYMBOL:
		return true
	}
	return false
}

// canStartCommandArg decides whether the current token begins a paren-less
// argument list. This is the `foo -1` (call) vs `foo - 1` (subtraction)
// disambiguation, driven by SpaceBefore.
func (p *Parser) canStartCommandArg() bool {
	t := p.cur()
	if !t.SpaceBefore {
		return false
	}
	switch t.Type {
	case token.INT, token.FLOAT, token.STRING, token.STRBEG, token.SYMBOL, token.IDENT, token.CONST,
		token.IVAR, token.CVAR, token.GVAR, token.TRUE, token.FALSE, token.NIL, token.SELF, token.BANG, token.TILDE,
		token.LPAREN, token.LBRACKET, token.ARROW, token.WORDS, token.SYMBOLS, token.REGEXP, token.XSTRING,
		token.BEGIN, token.CASE, token.DEF, token.SUPER:
		// Value-producing keywords: `p begin; 1; end`, `p case x; when 1; 2; end`,
		// `def` (which evaluates to the method's name symbol, as in
		// `module_function def foo; end` / `private def bar; end`), and `super`
		// (`number_to_currency super`, `Request.new super, url_helpers`).
		return true
	case token.LABEL:
		// Keyword/hash argument without parens: `render json: x`, `delegate to: :c`.
		return true
	case token.SCOPE:
		// A leading `::Name` (top-level constant) as a command argument: `puts ::Foo`.
		// Only when the `::` hugs a following constant, not the postfix `a ::b` form.
		return p.peekTok().Type == token.CONST
	case token.MINUS, token.PLUS, token.STAR, token.POW, token.AMPER:
		// A sign/splat/double-splat/block-pass operand that hugs the next token is a
		// unary-style command argument: `foo -1`, `foo *args`, `foo **opts`, `foo &blk`.
		// With a space on both sides it is a binary operator: `foo - 1`, `foo * 2`.
		return !p.peekTok().SpaceBefore
	}
	return false
}

// parseCommandArgs parses a paren-less argument list (`foo a, b, key: v`). It
// reuses parseOneCallArg, so it accepts the same positional/splat/block-pass and
// keyword/hash-pair forms a parenthesized call does, collapsing trailing
// `key: value` / `expr => value` pairs into one implicit Hash argument.
func (p *Parser) parseCommandArgs() []ast.Node {
	// A trailing `do…end` binds to the command call, not to an argument that is
	// itself a call (`foo bar do…end` → the block is foo's). Suppress block
	// attachment while parsing the arguments so the enclosing postfix chain picks
	// the `do` up for the command call. A braced `{…}` block is unaffected: it
	// binds tighter and always attaches to the nearest call.
	saved := p.noDo
	p.noDo = true
	var args []ast.Node
	var kw *ast.HashLit
	p.parseOneCallArg(&args, &kw)
	for p.accept(token.COMMA) {
		p.skipNewlines()
		p.parseOneCallArg(&args, &kw)
	}
	p.noDo = saved
	if kw != nil {
		args = append(args, kw)
	}
	return args
}

func (p *Parser) parseCallArgs(until token.Type) []ast.Node {
	var args []ast.Node
	var kw *ast.HashLit
	p.skipNewlines()
	if p.is(until) {
		return args
	}
	p.parseOneCallArg(&args, &kw)
	for p.accept(token.COMMA) {
		p.skipNewlines()
		// A trailing comma before the closing delimiter is allowed: foo(1, 2,).
		if p.is(until) {
			break
		}
		p.parseOneCallArg(&args, &kw)
	}
	p.skipNewlines()
	// Trailing `key: value` / `key => value` pairs collapse into one implicit
	// Hash argument (Ruby's keyword/last-hash sugar): foo(1, a: 2) → foo(1, {a:2}).
	if kw != nil {
		args = append(args, kw)
	}
	return args
}

// parseOneCallArg parses a single call argument, routing `*splat` and positional
// expressions into args, and `label: value` / `expr => value` pairs into kw.
func (p *Parser) parseOneCallArg(args *[]ast.Node, kw **ast.HashLit) {
	// `...` — forward the enclosing method's arguments, but only when it stands
	// alone (closes the arg position). A `...` with an operand is a beginless
	// exclusive range (`foo(...0)`, `[...11, 11]`), handled by the expression path.
	if p.is(token.DOTDOTDOT) && (p.peekTok().Type == token.RPAREN || p.peekTok().Type == token.COMMA) {
		p.advance()
		*args = append(*args, &ast.ForwardArgs{})
		return
	}
	if p.accept(token.AMPER) { // &expr — block-pass (coerced to a Proc)
		// A bare `&` (no operand, at the end of the arg list) forwards the enclosing
		// method's anonymous block parameter: `foo(&)`, `define_method(:x, &)`.
		if p.atAnonForwardEnd() {
			*args = append(*args, &ast.BlockPass{})
			return
		}
		*args = append(*args, &ast.BlockPass{Value: p.parseExprOrAssign()})
		return
	}
	if p.accept(token.POW) { // **expr — double-splat into the keyword hash
		// A bare `**` forwards the enclosing method's anonymous keyword rest:
		// `g(**)`, `view_context.render(inline: <<~ERB.strip, **)`.
		if p.atAnonForwardEnd() {
			p.addKwPair(kw, nil, nil)
			return
		}
		p.addKwPair(kw, nil, p.parseExprOrAssign())
		return
	}
	if p.is(token.LABEL) {
		name := p.advance().Lit
		key := &ast.SymbolLit{Name: name}
		// Value-omitted shorthand (Ruby 3.4): `foo(format:, name:)` means
		// `foo(format: format, name: name)` — when the label is not followed by a
		// value, the value is the same-named local or method call.
		if p.atKwShorthandEnd() {
			p.addKwPair(kw, key, p.barewordValue(name))
			return
		}
		p.addKwPair(kw, key, p.parseExprOrAssign())
		return
	}
	if p.accept(token.STAR) {
		// A bare `*` forwards the enclosing method's anonymous rest parameter:
		// `foo(*)`, `to_str[*]`.
		if p.atAnonForwardEnd() {
			*args = append(*args, &ast.SplatArg{})
			return
		}
		*args = append(*args, &ast.SplatArg{Value: p.parseExprOrAssign()})
		return
	}
	node := p.parseExprOrAssign()
	if p.accept(token.HASHROCKET) {
		p.addKwPair(kw, node, p.parseExprOrAssign())
		return
	}
	// A quoted string key followed by `:` is a symbol-keyed pair (the quoted form
	// of a `key:` keyword argument): `tag(:div, "@click": "f()")`.
	if sym, ok := p.stringKeyColon(node); ok {
		p.addKwPair(kw, sym, p.parseExprOrAssign())
		return
	}
	*args = append(*args, node)
}

// atKwShorthandEnd reports whether the cursor sits where a value-omitted keyword
// shorthand (`key:` with no value) ends: at a comma, newline, EOF, or a closing
// call/array/hash delimiter.
func (p *Parser) atKwShorthandEnd() bool {
	switch p.cur().Type {
	case token.COMMA, token.NEWLINE, token.EOF,
		token.RPAREN, token.RBRACKET, token.RBRACE:
		return true
	}
	return false
}

// atAnonForwardEnd reports whether a just-consumed `*`/`**`/`&` has no operand —
// i.e. it is an anonymous-argument forward at a call site (`foo(&)`, `bar(*)`,
// `g(**)`). That is the case when the next token closes the argument position.
func (p *Parser) atAnonForwardEnd() bool {
	switch p.cur().Type {
	case token.COMMA, token.RPAREN, token.RBRACKET, token.NEWLINE, token.EOF:
		return true
	}
	return false
}

// addKwPair appends a key/value pair to the implicit trailing-hash argument,
// allocating it on first use.
func (p *Parser) addKwPair(kw **ast.HashLit, k, v ast.Node) {
	if *kw == nil {
		*kw = &ast.HashLit{}
	}
	(*kw).Keys = append((*kw).Keys, k)
	(*kw).Values = append((*kw).Values, v)
}
