// Package lexer turns source bytes into tokens.
//
// Phase 0 carries the seeds of MRI's stateful lexer — SpaceBefore on every
// token and a lexState field — without yet exercising the hard cases (regex vs
// division, heredocs, interpolation). Those land in later phases (plan §10);
// the state plumbing is here from the start so they slot in without a rewrite.
package lexer

import (
	"strings"

	"github.com/go-ruby-parser/parser/token"
)

// lexState mirrors MRI's EXPR_* family. Phase 0 only distinguishes "a value /
// operand may come next" (begin) from "a value just ended" (end); future
// phases add the rest to disambiguate ambiguous characters.
type lexState int

const (
	exprBegin lexState = iota // expecting an operand (start of expression)
	exprEnd                   // just finished an operand
)

type Lexer struct {
	src   []byte
	pos   int
	line  int
	col   int
	state lexState
	// interpBraces tracks open '{' counts per active string interpolation, so a
	// '}' that closes an interpolation is distinguished from a hash/block brace.
	interpBraces []int
	// pending holds tokens produced ahead of the cursor (a heredoc value is lexed
	// where `<<ID` appears, then drained before the rest of the line continues).
	pending []token.Token
	// htResume, when > 0, is where the cursor jumps after the current line's
	// newline: past the heredoc body lines consumed out of band (and the start of
	// the next heredoc's body when several share a line). htLines counts the
	// source lines those bodies span, to keep l.line correct after the jump.
	htResume int
	htLines  int
	// prevType is the type of the last token next() returned, used to recognise a
	// trailing-operator line continuation (a line ending in an infix operator
	// joins the next line, as MRI does).
	prevType token.Type
}

func New(src string) *Lexer {
	return &Lexer{src: []byte(src), line: 1, col: 0, state: exprBegin}
}

func (l *Lexer) peek() byte {
	if l.pos >= len(l.src) {
		return 0
	}
	return l.src[l.pos]
}

func (l *Lexer) peek2() byte {
	if l.pos+1 >= len(l.src) {
		return 0
	}
	return l.src[l.pos+1]
}

func (l *Lexer) advance() byte {
	c := l.src[l.pos]
	l.pos++
	if c == '\n' {
		l.line++
		l.col = 0
	} else {
		l.col++
	}
	return c
}

// Tokenize returns the full token stream, terminated by an EOF token.
func (l *Lexer) Tokenize() []token.Token {
	var toks []token.Token
	for {
		t := l.next()
		toks = append(toks, t)
		if t.Type == token.EOF {
			return toks
		}
	}
}

// next returns the next token and records its type, so the lexer can recognise a
// trailing-operator line continuation. The lexing itself is in lexToken.
func (l *Lexer) next() token.Token {
	t := l.lexToken()
	l.prevType = t.Type
	return t
}

// isContinuationOp reports whether a line ending in token type t is incomplete,
// so the trailing newline continues onto the next line rather than terminating
// the statement. These are infix operators (plus comma and a trailing dot) that
// require a right-hand operand. Deliberately excluded as ambiguous: `|` and `&`
// (block params / block-pass), `^` (pattern pin), `:` (label/symbol/ternary),
// `<<` (heredoc opener).
func isContinuationOp(t token.Type) bool {
	switch t {
	case token.PLUS, token.MINUS, token.STAR, token.POW, token.SLASH, token.PERCENT,
		token.EQ, token.EQQ, token.MATCH, token.NEQ, token.LT, token.GT, token.LE,
		token.GE, token.SPACESHIP, token.ANDAND, token.OROR,
		token.ASSIGN, token.OPASSIGN, token.COMMA, token.HASHROCKET,
		token.DOT, token.SAFEDOT, token.QUESTION:
		return true
	}
	return false
}

func (l *Lexer) lexToken() token.Token {
	// Drain any tokens produced ahead of the cursor (a spliced heredoc value).
	if len(l.pending) > 0 {
		t := l.pending[0]
		l.pending = l.pending[1:]
		return t
	}
	spaceBefore := l.skipSpaceAndComments()
	line, col := l.line, l.col+1
	mk := func(tt token.Type, lit string) token.Token {
		return token.Token{Type: tt, Lit: lit, Line: line, Col: col, SpaceBefore: spaceBefore}
	}

	c := l.peek()
	switch {
	case c == 0:
		return mk(token.EOF, "")
	case c == '/' && l.state == exprBegin:
		// At expression-begin position a '/' opens a regexp literal, not division
		// (the same disambiguation MRI uses via its lexer state).
		return l.lexRegexp(spaceBefore, line, col)
	case c == '%' && l.percentBeginsLiteral(spaceBefore) && l.atPercentArray():
		// %w[…] / %i[…] / %W[…] / %I[…] word- and symbol-array literals.
		return l.lexPercentArray(spaceBefore, line, col)
	case c == '%' && l.percentBeginsLiteral(spaceBefore) && l.atPercentRXS():
		// %r{…}flags regexp, %x{…} backtick command, %s{…} symbol literals.
		return l.lexPercentRXS(spaceBefore, line, col)
	case c == '%' && l.percentBeginsLiteral(spaceBefore) && l.atPercentString():
		// %q(…) / %Q(…) / %(…) / %W(…) string literals.
		return l.lexPercentString(spaceBefore, line, col)
	case c == '\n' || c == ';':
		l.advance()
		l.state = exprBegin
		// A newline that ends a line carrying heredoc(s) skips past their bodies,
		// which were already consumed when the `<<ID` was lexed.
		if c == '\n' && l.htResume > 0 {
			l.pos = l.htResume
			l.line += l.htLines
			l.col = 0
			l.htResume, l.htLines = 0, 0
		}
		// Leading-dot continuation: a newline whose next significant token is `.`
		// or `&.` does not terminate the statement — the dot line chains onto the
		// previous expression (MRI joins such lines in the lexer). `;` is an
		// explicit terminator and is never suppressed this way.
		if c == '\n' && l.nextLineStartsWithDot() {
			return l.next()
		}
		// Trailing-operator continuation: a line ending in an infix operator
		// (`a ||`, `x +`, a trailing comma, …) is incomplete and joins the next
		// line. `;` is an explicit terminator and is never suppressed this way.
		if c == '\n' && isContinuationOp(l.prevType) {
			return l.next()
		}
		return mk(token.NEWLINE, "\\n")
	case isDigit(c):
		return l.lexNumber(spaceBefore, line, col)
	case isIdentStart(c):
		return l.lexIdent(spaceBefore, line, col)
	case c == '"':
		return l.lexString(spaceBefore, line, col)
	case c == '\'':
		return l.lexSingleQuote(spaceBefore, line, col)
	case c == '@':
		return l.lexIvar(spaceBefore, line, col)
	case c == '$':
		return l.lexGvar(spaceBefore, line, col)
	case c == ':' && (isIdentStart(l.peek2()) || l.peek2() == '@' || l.peek2() == '$'):
		return l.lexSymbol(spaceBefore, line, col)
	case c == ':' && symbolOpAt(l.src, l.pos+1) != "":
		// Operator-method symbol: :+, :<<, :[]=, …
		return l.lexSymbol(spaceBefore, line, col)
	case c == ':' && l.peek2() == '"':
		// Quoted symbol, possibly interpolated: :"foo bar", :"a#{x}".
		return l.lexSymbol(spaceBefore, line, col)
	}

	// Operators and delimiters.
	l.advance()
	switch c {
	case '+':
		if l.peek() == '=' {
			l.advance()
			l.state = exprBegin
			return mk(token.OPASSIGN, "+")
		}
		l.state = exprBegin
		return mk(token.PLUS, "+")
	case '-':
		if l.peek() == '=' {
			l.advance()
			l.state = exprBegin
			return mk(token.OPASSIGN, "-")
		}
		if l.peek() == '>' { // -> stabby lambda
			l.advance()
			l.state = exprBegin
			return mk(token.ARROW, "->")
		}
		l.state = exprBegin
		return mk(token.MINUS, "-")
	case '*':
		if l.peek() == '*' {
			l.advance()
			l.state = exprBegin
			return mk(token.POW, "**")
		}
		if l.peek() == '=' {
			l.advance()
			l.state = exprBegin
			return mk(token.OPASSIGN, "*")
		}
		l.state = exprBegin
		return mk(token.STAR, "*")
	case '/':
		if l.peek() == '=' {
			l.advance()
			l.state = exprBegin
			return mk(token.OPASSIGN, "/")
		}
		l.state = exprBegin
		return mk(token.SLASH, "/")
	case '%':
		if l.peek() == '=' {
			l.advance()
			l.state = exprBegin
			return mk(token.OPASSIGN, "%")
		}
		l.state = exprBegin
		return mk(token.PERCENT, "%")
	case '(':
		l.state = exprBegin
		return mk(token.LPAREN, "(")
	case ')':
		l.state = exprEnd
		return mk(token.RPAREN, ")")
	case '{':
		l.state = exprBegin
		if n := len(l.interpBraces); n > 0 {
			l.interpBraces[n-1]++
		}
		return mk(token.LBRACE, "{")
	case '}':
		if n := len(l.interpBraces); n > 0 {
			if l.interpBraces[n-1] == 0 {
				l.interpBraces = l.interpBraces[:n-1] // this '}' closes the interpolation
				return l.continueString(line, col)
			}
			l.interpBraces[n-1]--
		}
		l.state = exprEnd
		return mk(token.RBRACE, "}")
	case '[':
		l.state = exprBegin
		return mk(token.LBRACKET, "[")
	case ']':
		l.state = exprEnd
		return mk(token.RBRACKET, "]")
	case '|':
		if l.peek() == '|' {
			l.advance()
			if l.peek() == '=' {
				l.advance()
				l.state = exprBegin
				return mk(token.OPASSIGN, "||")
			}
			l.state = exprBegin
			return mk(token.OROR, "||")
		}
		l.state = exprBegin
		return mk(token.PIPE, "|")
	case '&':
		if l.peek() == '&' {
			l.advance()
			if l.peek() == '=' {
				l.advance()
				l.state = exprBegin
				return mk(token.OPASSIGN, "&&")
			}
			l.state = exprBegin
			return mk(token.ANDAND, "&&")
		}
		if l.peek() == '.' { // &. safe navigation
			l.advance()
			l.state = exprBegin
			return mk(token.SAFEDOT, "&.")
		}
		l.state = exprBegin
		return mk(token.AMPER, "&")
	case ',':
		l.state = exprBegin
		return mk(token.COMMA, ",")
	case '.':
		if l.peek() == '.' {
			l.advance()
			if l.peek() == '.' {
				l.advance()
				l.state = exprBegin
				return mk(token.DOTDOTDOT, "...")
			}
			l.state = exprBegin
			return mk(token.DOTDOT, "..")
		}
		l.state = exprBegin
		return mk(token.DOT, ".")
	case '=':
		if l.peek() == '=' {
			l.advance()
			if l.peek() == '=' {
				l.advance()
				l.state = exprBegin
				return mk(token.EQQ, "===")
			}
			l.state = exprBegin
			return mk(token.EQ, "==")
		}
		if l.peek() == '>' {
			l.advance()
			l.state = exprBegin
			return mk(token.HASHROCKET, "=>")
		}
		if l.peek() == '~' { // =~ match operator
			l.advance()
			l.state = exprBegin
			return mk(token.MATCH, "=~")
		}
		l.state = exprBegin
		return mk(token.ASSIGN, "=")
	case '!':
		if l.peek() == '=' {
			l.advance()
			l.state = exprBegin
			return mk(token.NEQ, "!=")
		}
		l.state = exprBegin
		return mk(token.BANG, "!")
	case '<':
		if l.peek() == '=' {
			l.advance()
			if l.peek() == '>' { // <=>
				l.advance()
				l.state = exprBegin
				return mk(token.SPACESHIP, "<=>")
			}
			l.state = exprBegin
			return mk(token.LE, "<=")
		}
		if l.peek() == '<' { // <<, <<= or a heredoc
			l.advance()
			if l.peek() == '=' {
				l.advance()
				l.state = exprBegin
				return mk(token.OPASSIGN, "<<")
			}
			if l.atHeredoc(spaceBefore) {
				return l.lexHeredoc(spaceBefore, line, col)
			}
			l.state = exprBegin
			return mk(token.SHOVEL, "<<")
		}
		l.state = exprBegin
		return mk(token.LT, "<")
	case '>':
		if l.peek() == '=' {
			l.advance()
			l.state = exprBegin
			return mk(token.GE, ">=")
		}
		if l.peek() == '>' { // >> (right shift)
			l.advance()
			l.state = exprBegin
			return mk(token.RSHIFT, ">>")
		}
		l.state = exprBegin
		return mk(token.GT, ">")
	case '?':
		l.state = exprBegin
		return mk(token.QUESTION, "?")
	case ':':
		if l.peek() == ':' { // :: constant scope resolution
			l.advance()
			l.state = exprBegin
			return mk(token.SCOPE, "::")
		}
		l.state = exprBegin
		return mk(token.COLON, ":")
	case '^':
		l.state = exprBegin
		return mk(token.CARET, "^")
	case '~':
		l.state = exprBegin
		return mk(token.TILDE, "~")
	}
	return mk(token.ILLEGAL, string(c))
}

// skipSpaceAndComments consumes spaces, tabs, comments and line continuations,
// returning whether any whitespace was seen (feeds SpaceBefore). Newlines are
// significant and are NOT skipped here.
func (l *Lexer) skipSpaceAndComments() bool {
	seen := false
	for {
		c := l.peek()
		switch {
		case c == ' ' || c == '\t' || c == '\r':
			l.advance()
			seen = true
		case c == '\\' && l.peek2() == '\n': // line continuation
			l.advance()
			l.advance()
			seen = true
		case c == '#': // comment to end of line
			for l.peek() != '\n' && l.peek() != 0 {
				l.advance()
			}
			seen = true
		default:
			return seen
		}
	}
}

// nextLineStartsWithDot reports whether the first significant character at or
// after the cursor — skipping spaces, tabs, CR, blank lines, and comments — is a
// leading `.` (method-chain dot, not a `..`/`...` range) or `&.` safe-nav dot.
// It does not advance the cursor.
func (l *Lexer) nextLineStartsWithDot() bool {
	p := l.pos
	for p < len(l.src) {
		switch c := l.src[p]; {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f' || c == '\v':
			p++
		case c == '#': // comment to end of line
			for p < len(l.src) && l.src[p] != '\n' {
				p++
			}
		case c == '.':
			// A `.` chains; a `..`/`...` range does not.
			return p+1 >= len(l.src) || l.src[p+1] != '.'
		case c == '&':
			return p+1 < len(l.src) && l.src[p+1] == '.'
		default:
			return false
		}
	}
	return false
}

func (l *Lexer) lexNumber(spaceBefore bool, line, col int) token.Token {
	// Radix-prefixed integer literals: 0x/0X (hex), 0o/0O (octal), 0b/0B
	// (binary), 0d/0D (explicit decimal). A bare leading zero (0NN) is octal and
	// is handled by base-0 decoding in the parser, so it needs no special lexing.
	if l.peek() == '0' {
		switch l.peek2() {
		case 'x', 'X', 'o', 'O', 'b', 'B', 'd', 'D':
			return l.lexRadixInt(spaceBefore, line, col)
		}
	}
	start := l.pos
	for isDigit(l.peek()) || l.peek() == '_' {
		l.advance()
	}
	isFloat := false
	if l.peek() == '.' && isDigit(l.peek2()) {
		isFloat = true
		l.advance() // '.'
		for isDigit(l.peek()) || l.peek() == '_' {
			l.advance()
		}
	}
	// Exponent: e/E with an optional sign and digits. An exponent always makes
	// the literal a Float, even without a fractional part (Ruby: 1e3 == 1000.0).
	if c := l.peek(); c == 'e' || c == 'E' {
		n := l.peek2()
		expDigits := isDigit(n) || ((n == '+' || n == '-') && l.pos+2 < len(l.src) && isDigit(l.src[l.pos+2]))
		if expDigits {
			isFloat = true
			l.advance() // e/E
			if l.peek() == '+' || l.peek() == '-' {
				l.advance()
			}
			for isDigit(l.peek()) || l.peek() == '_' {
				l.advance()
			}
		}
	}
	lit := stripUnderscores(string(l.src[start:l.pos]))
	l.state = exprEnd
	tt := token.INT
	if isFloat {
		tt = token.FLOAT
	}
	return token.Token{Type: tt, Lit: lit, Line: line, Col: col, SpaceBefore: spaceBefore}
}

// lexRadixInt lexes a prefixed integer literal (cursor on the leading '0'). The
// emitted Lit keeps a Go-recognisable lowercase prefix (0x/0o/0b) so the parser
// can decode it with base 0; an explicit-decimal 0d literal drops its prefix.
// Underscores between digits are allowed. With no digits after the prefix it
// returns ILLEGAL.
func (l *Lexer) lexRadixInt(spaceBefore bool, line, col int) token.Token {
	l.advance()                // '0'
	kind := l.advance() | 0x20 // letter, lower-cased
	ok := radixDigit(kind)
	var digits []byte
	for {
		c := l.peek()
		if c == '_' {
			l.advance()
			continue
		}
		if !ok(c) {
			break
		}
		digits = append(digits, l.advance())
	}
	l.state = exprEnd
	if len(digits) == 0 {
		return token.Token{Type: token.ILLEGAL, Lit: "invalid numeric literal", Line: line, Col: col, SpaceBefore: spaceBefore}
	}
	lit := string(digits)
	if kind != 'd' {
		lit = "0" + string(kind) + lit
	}
	return token.Token{Type: token.INT, Lit: lit, Line: line, Col: col, SpaceBefore: spaceBefore}
}

// radixDigit returns the digit-membership test for a radix prefix letter.
func radixDigit(kind byte) func(byte) bool {
	switch kind {
	case 'x':
		return func(c byte) bool {
			return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
		}
	case 'o':
		return func(c byte) bool { return c >= '0' && c <= '7' }
	case 'b':
		return func(c byte) bool { return c == '0' || c == '1' }
	default: // 'd'
		return isDigit
	}
}

func (l *Lexer) lexIdent(spaceBefore bool, line, col int) token.Token {
	start := l.pos
	for isIdentPart(l.peek()) {
		l.advance()
	}
	// A plain identifier immediately followed by a single ':' is a hash label
	// (`name:`), as in Ruby. The ':' is consumed; Lit holds the name.
	if l.peek() == ':' && l.peek2() != ':' {
		lit := string(l.src[start:l.pos])
		l.advance() // ':'
		l.state = exprBegin
		return token.Token{Type: token.LABEL, Lit: lit, Line: line, Col: col, SpaceBefore: spaceBefore}
	}
	// Trailing ? or ! is part of a method name (e.g. empty?, save!).
	if c := l.peek(); c == '?' || c == '!' {
		l.advance()
	}
	lit := string(l.src[start:l.pos])
	tt := token.LookupIdent(lit)
	// After a value-like keyword/identifier, the next state is "end"; after a
	// keyword that introduces an expression it stays "begin".
	switch tt {
	case token.IDENT, token.CONST, token.NIL, token.TRUE, token.FALSE, token.SELF, token.END:
		l.state = exprEnd
	default:
		l.state = exprBegin
	}
	return token.Token{Type: tt, Lit: lit, Line: line, Col: col, SpaceBefore: spaceBefore}
}

// lexSymbol lexes :name (the leading ':' is at the cursor and the next byte is
// known to start an identifier). Lit holds the name without the colon.
// symbolOps are the operator method names that can appear as a symbol (`:+`,
// `:[]=`, …), ordered so the first prefix match is the longest.
var symbolOps = []string{
	"[]=", "<=>", "===", "[]", "==", "=~", "!=", "<<", ">>", "<=", ">=", "**",
	"+@", "-@", "+", "-", "*", "/", "%", "<", ">", "&", "|", "^", "~", "!",
}

// symbolOpAt returns the operator-symbol name starting at src[i], or "".
func symbolOpAt(src []byte, i int) string {
	if i >= len(src) {
		return ""
	}
	rest := src[i:]
	for _, op := range symbolOps {
		if len(rest) >= len(op) && string(rest[:len(op)]) == op {
			return op
		}
	}
	return ""
}

func (l *Lexer) lexSymbol(spaceBefore bool, line, col int) token.Token {
	l.advance() // ':'
	// Quoted symbols: :"foo bar" (and the interpolated :"a#{x}" form).
	if l.peek() == '"' {
		return l.lexQuotedSymbol(spaceBefore, line, col)
	}
	// Operator-method symbols: :+, :<=>, :[]=, …
	if op := symbolOpAt(l.src, l.pos); op != "" {
		for range op {
			l.advance()
		}
		l.state = exprEnd
		return token.Token{Type: token.SYMBOL, Lit: op, Line: line, Col: col, SpaceBefore: spaceBefore}
	}
	start := l.pos
	// Variable-name symbols: :@ivar, :@@cvar, :$global.
	switch l.peek() {
	case '@':
		l.advance()
		if l.peek() == '@' {
			l.advance()
		}
	case '$':
		l.advance()
	}
	for isIdentPart(l.peek()) {
		l.advance()
	}
	switch c := l.peek(); {
	case c == '?' || c == '!': // :empty?, :save!
		l.advance()
	case c == '=' && l.peek2() != '=' && l.peek2() != '~' && l.peek2() != '>':
		l.advance() // setter symbol :name= (but not :foo== / :foo=~ / :foo=>)
	}
	l.state = exprEnd
	return token.Token{Type: token.SYMBOL, Lit: string(l.src[start:l.pos]), Line: line, Col: col, SpaceBefore: spaceBefore}
}

// lexQuotedSymbol lexes a :"…" symbol (cursor on the opening quote). With no
// interpolation it is a single SYMBOL whose value is the escape-processed body;
// an interpolated body becomes a spliced `"…".to_sym`.
func (l *Lexer) lexQuotedSymbol(spaceBefore bool, line, col int) token.Token {
	content, ok := l.scanQuotedRaw()
	l.state = exprEnd
	if !ok {
		return token.Token{Type: token.ILLEGAL, Lit: "unterminated symbol", Line: line, Col: col, SpaceBefore: spaceBefore}
	}
	// Re-lex the body as a double-quoted string (the raw bytes already form a
	// valid one, since the closing quote was found respecting escapes and #{}).
	var body []token.Token
	for _, t := range New(`"` + content + `"`).Tokenize() {
		if t.Type != token.EOF {
			body = append(body, t)
		}
	}
	if len(body) == 1 && body[0].Type == token.STRING {
		return token.Token{Type: token.SYMBOL, Lit: body[0].Lit, Line: line, Col: col, SpaceBefore: spaceBefore}
	}
	body[0].Line, body[0].Col, body[0].SpaceBefore = line, col, spaceBefore
	rest := append(body[1:],
		token.Token{Type: token.DOT, Lit: "."},
		token.Token{Type: token.IDENT, Lit: "to_sym"})
	l.pending = append(l.pending, rest...)
	return body[0]
}

// scanQuotedRaw reads a double-quoted body (cursor on the opening quote) and
// returns its raw bytes (escapes and #{…} left intact), consuming the closing
// quote. ok is false if the input ends first. A quote inside an escape or an
// interpolation does not close the literal.
func (l *Lexer) scanQuotedRaw() (string, bool) {
	l.advance() // opening quote
	var buf []byte
	depth := 0
	for {
		c := l.peek()
		switch {
		case c == 0:
			return string(buf), false
		case depth == 0 && c == '"':
			l.advance()
			return string(buf), true
		case c == '\\':
			buf = append(buf, l.advance())
			if l.peek() != 0 {
				buf = append(buf, l.advance())
			}
		case c == '#' && l.peek2() == '{':
			depth++
			buf = append(buf, l.advance(), l.advance())
		default:
			if depth > 0 {
				switch c {
				case '{':
					depth++
				case '}':
					depth--
				}
			}
			buf = append(buf, l.advance())
		}
	}
}

// lexGvar lexes a global variable: $name, the match-data specials $~ $& $` $',
// or $1.. (last-match group references).
func (l *Lexer) lexGvar(spaceBefore bool, line, col int) token.Token {
	l.advance() // '$'
	start := l.pos
	switch c := l.peek(); {
	case c == '~' || c == '&' || c == '`' || c == '\'':
		l.advance()
	case c >= '1' && c <= '9':
		for l.peek() >= '0' && l.peek() <= '9' {
			l.advance()
		}
	default:
		for isIdentPart(l.peek()) {
			l.advance()
		}
		if start == l.pos { // a bare '$' with no name is illegal
			return token.Token{Type: token.ILLEGAL, Lit: "$", Line: line, Col: col, SpaceBefore: spaceBefore}
		}
	}
	l.state = exprEnd
	return token.Token{Type: token.GVAR, Lit: "$" + string(l.src[start:l.pos]), Line: line, Col: col, SpaceBefore: spaceBefore}
}

func (l *Lexer) lexIvar(spaceBefore bool, line, col int) token.Token {
	l.advance() // '@'
	// A second '@' makes this a class variable (@@name).
	kind, prefix := token.IVAR, "@"
	if l.peek() == '@' {
		l.advance()
		kind, prefix = token.CVAR, "@@"
	}
	start := l.pos
	for isIdentPart(l.peek()) {
		l.advance()
	}
	if start == l.pos { // a bare '@' / '@@' with no name is illegal
		return token.Token{Type: token.ILLEGAL, Lit: prefix, Line: line, Col: col, SpaceBefore: spaceBefore}
	}
	l.state = exprEnd
	return token.Token{Type: kind, Lit: prefix + string(l.src[start:l.pos]), Line: line, Col: col, SpaceBefore: spaceBefore}
}

// lexRegexp lexes a /pattern/flags regexp literal. The opening '/' is at the
// cursor. Escapes are preserved verbatim into the source (so \d, \. and the
// like reach the engine untouched) except that an escaped delimiter \/ becomes
// a literal '/'. Trailing flag letters i, m, x are collected into Flags; any
// other trailing letters are ignored gracefully (consumed but not recorded).
func (l *Lexer) lexRegexp(spaceBefore bool, line, col int) token.Token {
	l.advance() // opening '/'
	var src []byte
	for {
		c := l.peek()
		if c == 0 {
			break // unterminated; emit what we have (parser will still build a literal)
		}
		if c == '/' {
			l.advance() // closing '/'
			break
		}
		if c == '\\' {
			l.advance()
			esc := l.peek()
			if esc == 0 {
				src = append(src, '\\')
				break
			}
			l.advance()
			if esc == '/' {
				src = append(src, '/') // \/ → literal slash
			} else {
				src = append(src, '\\', esc) // keep the escape for the engine
			}
			continue
		}
		src = append(src, l.advance())
	}
	var flags []byte
	for {
		c := l.peek()
		if c < 'a' || c > 'z' {
			break
		}
		l.advance()
		if c == 'i' || c == 'm' || c == 'x' {
			flags = append(flags, c)
		}
	}
	l.state = exprEnd
	return token.Token{Type: token.REGEXP, Lit: string(src), Flags: string(flags), Line: line, Col: col, SpaceBefore: spaceBefore}
}

// percentDelimClose returns the closing delimiter for a %-literal opener: the
// mate of a bracket pair, or the same character for a symmetric delimiter.
func percentDelimClose(open byte) byte {
	switch open {
	case '[':
		return ']'
	case '(':
		return ')'
	case '{':
		return '}'
	case '<':
		return '>'
	}
	return open
}

// percentBeginsLiteral reports whether a '%' at the cursor introduces a
// percent-literal rather than the binary modulo operator. This mirrors the
// '/' regexp-vs-division rule: a literal starts whenever a value/operand is
// expected (exprBegin). It additionally fires in command-argument position —
// just after a value (exprEnd), when the '%' has a space before it but the
// percent-kind/delimiter hugs it with no space after — so `p %w[a b]` lexes
// the argument as a literal while `a % b` and `a %b` stay modulo.
func (l *Lexer) percentBeginsLiteral(spaceBefore bool) bool {
	if l.state == exprBegin {
		return true
	}
	return spaceBefore
}

// atPercentArray reports whether the cursor (positioned at '%') begins a
// %w/%i array literal: the kind letter must be followed by a delimiter.
func (l *Lexer) atPercentArray() bool {
	switch l.peek2() {
	case 'w', 'i', 'W', 'I':
	default:
		return false
	}
	if l.pos+2 >= len(l.src) {
		return false
	}
	switch l.src[l.pos+2] {
	case '[', '(', '{', '<', '|', '!', '/':
		return true
	}
	return false
}

// lexPercentArray lexes a %w/%i/%W/%I array literal. The non-interpolating
// %w/%i forms become a single WORDS/SYMBOLS token whose Lit the parser splits.
// The interpolating %W/%I forms are spliced into the equivalent array of
// (interpolated) string/symbol elements. Bracket delimiters nest.
func (l *Lexer) lexPercentArray(spaceBefore bool, line, col int) token.Token {
	l.advance() // %
	kind := l.advance()
	open := l.advance()
	closing := percentDelimClose(open)
	depth := 1
	var content []byte
	for {
		c := l.peek()
		if c == 0 {
			return token.Token{Type: token.ILLEGAL, Lit: "unterminated %-array literal", Line: line, Col: col, SpaceBefore: spaceBefore}
		}
		if open != closing && c == open {
			depth++
		} else if c == closing {
			depth--
			if depth == 0 {
				l.advance() // closing delimiter
				break
			}
		}
		content = append(content, l.advance())
	}
	l.state = exprEnd
	if kind == 'W' || kind == 'I' {
		return l.splicePercentInterp(string(content), kind == 'I', spaceBefore, line, col)
	}
	tt := token.WORDS
	if kind == 'i' {
		tt = token.SYMBOLS
	}
	return token.Token{Type: tt, Lit: string(content), Line: line, Col: col, SpaceBefore: spaceBefore}
}

// isPercentDelim reports whether b can open a %-literal.
func isPercentDelim(b byte) bool {
	switch b {
	case '(', '[', '{', '<', '|', '!', '/':
		return true
	}
	return false
}

// atPercentRXS reports whether the cursor (at '%') begins a %r (regexp),
// %x (backtick command), or %s (symbol) literal: the kind letter must be
// followed by a delimiter.
func (l *Lexer) atPercentRXS() bool {
	switch l.peek2() {
	case 'r', 'x', 's':
	default:
		return false
	}
	return l.pos+2 < len(l.src) && isPercentDelim(l.src[l.pos+2])
}

// lexPercentRXS lexes a %r{…}flags regexp, %x{…} backtick command, or %s{…}
// symbol literal. As with the existing /…/ regexp form, the body is kept as raw
// source (interpolation markers are not expanded here). Bracket delimiters nest;
// a backslash keeps its following byte verbatim so an escaped delimiter does not
// close the literal.
func (l *Lexer) lexPercentRXS(spaceBefore bool, line, col int) token.Token {
	l.advance() // %
	kind := l.advance()
	open := l.advance()
	closing := percentDelimClose(open)
	depth := 1
	var body []byte
	for {
		c := l.peek()
		if c == 0 {
			return token.Token{Type: token.ILLEGAL, Lit: "unterminated %-literal", Line: line, Col: col, SpaceBefore: spaceBefore}
		}
		if c == '\\' {
			l.advance()
			esc := l.peek()
			if esc == 0 {
				body = append(body, '\\')
				break
			}
			l.advance()
			switch {
			case kind == 'r':
				// %r keeps the escape pair verbatim for the regexp engine — even an
				// escaped delimiter (MRI: `%r{a\}b}.source == "a\\}b"`). The escape
				// only prevents the delimiter from closing the literal.
				body = append(body, '\\', esc)
			case esc == open || esc == closing || esc == '\\':
				// %x/%s are string-like: an escaped delimiter or backslash becomes the
				// literal character (the backslash is dropped).
				body = append(body, esc)
			default:
				body = append(body, '\\', esc)
			}
			continue
		}
		if open != closing && c == open {
			depth++
		} else if c == closing {
			depth--
			if depth == 0 {
				l.advance() // closing delimiter
				break
			}
		}
		body = append(body, l.advance())
	}
	l.state = exprEnd
	switch kind {
	case 'r':
		var flags []byte
		for {
			c := l.peek()
			if c < 'a' || c > 'z' {
				break
			}
			l.advance()
			if c == 'i' || c == 'm' || c == 'x' {
				flags = append(flags, c)
			}
		}
		return token.Token{Type: token.REGEXP, Lit: string(body), Flags: string(flags), Line: line, Col: col, SpaceBefore: spaceBefore}
	case 'x':
		return token.Token{Type: token.XSTRING, Lit: string(body), Line: line, Col: col, SpaceBefore: spaceBefore}
	default: // 's' — a symbol; its name is the (un-interpolated) body
		return token.Token{Type: token.SYMBOL, Lit: string(body), Line: line, Col: col, SpaceBefore: spaceBefore}
	}
}

// atPercentString reports whether the cursor (at '%') begins a %q/%Q/%(…)
// string literal: %q or %Q followed by a delimiter, or a bare % directly
// followed by a delimiter (== %Q).
func (l *Lexer) atPercentString() bool {
	if c := l.peek2(); c == 'q' || c == 'Q' {
		return l.pos+2 < len(l.src) && isPercentDelim(l.src[l.pos+2])
	}
	return isPercentDelim(l.peek2())
}

// lexPercentString lexes %q(…) (non-interpolating; only \<delim> and \\ escape),
// and %Q(…) / %(…) (interpolating, double-quote semantics). Bracket delimiters
// nest. The interpolating forms are spliced as the equivalent "…" string.
func (l *Lexer) lexPercentString(spaceBefore bool, line, col int) token.Token {
	l.advance() // %
	interp := true
	if c := l.peek(); c == 'q' || c == 'Q' {
		interp = c == 'Q'
		l.advance()
	}
	open := l.advance()
	closing := percentDelimClose(open)
	depth := 1
	var body []byte
	for {
		c := l.peek()
		if c == 0 {
			return token.Token{Type: token.ILLEGAL, Lit: "unterminated %-string literal", Line: line, Col: col, SpaceBefore: spaceBefore}
		}
		if c == '\\' { // keep escape pairs verbatim; an escaped delimiter never nests
			body = append(body, l.advance())
			if l.peek() != 0 {
				body = append(body, l.advance())
			}
			continue
		}
		if open != closing && c == open {
			depth++
		} else if c == closing {
			depth--
			if depth == 0 {
				l.advance() // closing delimiter
				break
			}
		}
		body = append(body, l.advance())
	}
	l.state = exprEnd
	if !interp {
		return token.Token{Type: token.STRING, Lit: unescapePercentQ(string(body), open, closing), Line: line, Col: col, SpaceBefore: spaceBefore}
	}
	// Interpolating: lex the body as the equivalent double-quoted string and
	// splice its tokens (dropping the trailing EOF) ahead of the cursor.
	hts := New(`"` + wrapHeredocDQ(string(body)) + `"`).Tokenize()
	first := hts[0]
	first.Line, first.Col, first.SpaceBefore = line, col, spaceBefore
	rest := hts[1:]
	for len(rest) > 0 && rest[len(rest)-1].Type == token.EOF {
		rest = rest[:len(rest)-1]
	}
	l.pending = append(l.pending, rest...)
	return first
}

// unescapePercentQ resolves the single-quote-style escapes of a %q body: only
// \\ and the (escaped) delimiters become literal; every other backslash stays.
func unescapePercentQ(body string, open, closing byte) string {
	var b strings.Builder
	for i := 0; i < len(body); i++ {
		if body[i] == '\\' && i+1 < len(body) {
			if n := body[i+1]; n == '\\' || n == open || n == closing {
				b.WriteByte(n)
				i++
				continue
			}
		}
		b.WriteByte(body[i])
	}
	return b.String()
}

// splicePercentInterp turns a %W/%I body into the tokens of the equivalent
// array literal: each whitespace-separated word becomes an interpolated string
// (for %W) or, for %I, a plain symbol when it has no interpolation else a
// `"…".to_sym`. The first token (`[`) is returned; the rest are queued.
func (l *Lexer) splicePercentInterp(content string, symbols, spaceBefore bool, line, col int) token.Token {
	toks := []token.Token{{Type: token.LBRACKET, Lit: "[", Line: line, Col: col, SpaceBefore: spaceBefore}}
	for wi, w := range splitPercentWords(content) {
		if wi > 0 {
			toks = append(toks, token.Token{Type: token.COMMA, Lit: ",", Line: line, Col: col})
		}
		if symbols && !strings.Contains(w, "#{") {
			toks = append(toks, token.Token{Type: token.SYMBOL, Lit: w, Line: line, Col: col})
			continue
		}
		for _, t := range New(`"` + wrapHeredocDQ(w) + `"`).Tokenize() {
			if t.Type != token.EOF {
				toks = append(toks, t)
			}
		}
		if symbols {
			toks = append(toks,
				token.Token{Type: token.DOT, Lit: ".", Line: line, Col: col},
				token.Token{Type: token.IDENT, Lit: "to_sym", Line: line, Col: col})
		}
	}
	toks = append(toks, token.Token{Type: token.RBRACKET, Lit: "]", Line: line, Col: col})
	l.pending = append(l.pending, toks[1:]...)
	return toks[0]
}

// splitPercentWords splits a %W/%I body on whitespace, keeping whitespace that
// falls inside a #{…} interpolation as part of the word.
func splitPercentWords(s string) []string {
	var words []string
	var cur []byte
	depth := 0
	flush := func() {
		if len(cur) > 0 {
			words = append(words, string(cur))
			cur = nil
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if depth == 0 && (c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v') {
			flush()
			continue
		}
		if c == '#' && i+1 < len(s) && s[i+1] == '{' {
			depth++
			cur = append(cur, '#', '{')
			i++
			continue
		}
		if depth > 0 {
			switch c {
			case '{':
				depth++
			case '}':
				depth--
			}
		}
		cur = append(cur, c)
	}
	flush()
	return words
}

// atHeredoc reports whether the `<<` just consumed begins a heredoc rather than
// the shift/append operator. A heredoc is recognised only where a value is
// expected (expression-begin, or a command argument signalled by a space before
// `<<`); the marker must be a quote, or — for the bare `<<ID` form — an
// uppercase/underscore-led identifier (the `<<-`/`<<~` forms accept any
// identifier). This keeps `a << b` a shift while accepting the usual `<<HEREDOC`,
// `puts <<~SQL`, etc.
func (l *Lexer) atHeredoc(spaceBefore bool) bool {
	if l.state != exprBegin && !spaceBefore {
		return false
	}
	p := l.pos
	if p < len(l.src) && (l.src[p] == '-' || l.src[p] == '~') {
		p++
		if p >= len(l.src) {
			return false
		}
		c := l.src[p]
		return c == '"' || c == '\'' || isIdentStart(c)
	}
	if p >= len(l.src) {
		return false
	}
	c := l.src[p]
	return c == '"' || c == '\'' || (c >= 'A' && c <= 'Z') || c == '_'
}

// lexHeredoc lexes a heredoc starting just after `<<`. It parses the marker
// (optional `-`/`~`, optional quoting, the terminator), collects the body from
// the following lines, applies squiggly dedent, and yields the string value: a
// single STRING for a non-interpolating (`'TERM'`) body, or the tokens of the
// equivalent double-quoted string (spliced through l.pending) for an
// interpolating one. The body source is skipped when the current line's newline
// is reached.
func (l *Lexer) lexHeredoc(spaceBefore bool, line, col int) token.Token {
	squiggly := false
	indented := false
	if c := l.peek(); c == '-' || c == '~' {
		l.advance()
		indented = true
		squiggly = c == '~'
	}
	interp := true
	var quote byte
	if c := l.peek(); c == '"' || c == '\'' {
		quote = l.advance()
		interp = quote != '\''
	}
	var term []byte
	for {
		c := l.peek()
		if c == 0 || c == '\n' {
			break
		}
		if quote != 0 {
			if c == quote {
				l.advance()
				break
			}
		} else if !isIdentPart(c) {
			break
		}
		term = append(term, l.advance())
	}
	terminator := string(term)

	bodyStart := l.htResume
	if bodyStart == 0 {
		bodyStart = l.indexAfterNextNewline(l.pos)
	}
	body, postHeredoc := collectHeredoc(l.src, bodyStart, terminator, indented)
	if squiggly {
		body = squiggleDedent(body)
	}
	l.htLines += countByte(l.src[bodyStart:postHeredoc], '\n')
	l.htResume = postHeredoc

	l.state = exprEnd
	if !interp {
		return token.Token{Type: token.STRING, Lit: body, Line: line, Col: col, SpaceBefore: spaceBefore}
	}
	// Interpolating: lex the body as the equivalent double-quoted string and
	// splice its tokens (dropping the trailing EOF) ahead of the cursor.
	hts := New(`"` + wrapHeredocDQ(body) + `"`).Tokenize()
	first := hts[0]
	first.Line, first.Col, first.SpaceBefore = line, col, spaceBefore
	rest := hts[1:]
	for len(rest) > 0 && rest[len(rest)-1].Type == token.EOF {
		rest = rest[:len(rest)-1]
	}
	l.pending = append(l.pending, rest...)
	return first
}

// indexAfterNextNewline returns the index just past the next '\n' at or after p,
// or len(src) when there is none (the heredoc body runs to end of input).
func (l *Lexer) indexAfterNextNewline(p int) int {
	for i := p; i < len(l.src); i++ {
		if l.src[i] == '\n' {
			return i + 1
		}
	}
	return len(l.src)
}

// collectHeredoc gathers body lines from start until a line equal to terminator
// (leading whitespace allowed when indented). It returns the body text (the
// body lines with their newlines) and the index just past the terminator line.
func collectHeredoc(src []byte, start int, terminator string, indented bool) (string, int) {
	i := start
	var b []byte
	for i < len(src) {
		j := i
		for j < len(src) && src[j] != '\n' {
			j++
		}
		if isTerminatorLine(src[i:j], terminator, indented) {
			if j < len(src) {
				j++ // skip the terminator's own newline
			}
			return string(b), j
		}
		b = append(b, src[i:j]...)
		if j < len(src) {
			b = append(b, '\n')
		}
		if j >= len(src) {
			break
		}
		i = j + 1
	}
	return string(b), len(src)
}

// isTerminatorLine reports whether line is the heredoc terminator (after
// stripping leading whitespace when indented, and tolerating a trailing CR).
func isTerminatorLine(line []byte, terminator string, indented bool) bool {
	s := line
	if indented {
		k := 0
		for k < len(s) && (s[k] == ' ' || s[k] == '\t') {
			k++
		}
		s = s[k:]
	}
	if len(s) > 0 && s[len(s)-1] == '\r' {
		s = s[:len(s)-1]
	}
	return string(s) == terminator
}

// squiggleDedent removes the common leading whitespace of the non-blank body
// lines (the `<<~` form), as Ruby does.
func squiggleDedent(body string) string {
	lines := strings.Split(body, "\n")
	minIndent := -1
	for _, ln := range lines {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		ind := len(ln) - len(strings.TrimLeft(ln, " \t"))
		if minIndent < 0 || ind < minIndent {
			minIndent = ind
		}
	}
	if minIndent <= 0 {
		return body
	}
	for i, ln := range lines {
		if len(ln) >= minIndent {
			lines[i] = ln[minIndent:]
		} else {
			lines[i] = strings.TrimLeft(ln, " \t")
		}
	}
	return strings.Join(lines, "\n")
}

// wrapHeredocDQ escapes a heredoc body for embedding inside `"..."`: backslash
// escapes are preserved, and only a `"` that sits in literal text (outside an
// `#{…}` interpolation) is escaped — a `"` inside an interpolation belongs to
// the embedded expression and must pass through so the body lexes exactly like
// a double-quoted string.
func wrapHeredocDQ(body string) string {
	var b strings.Builder
	depth := 0       // brace nesting inside an active #{ … }
	escaped := false // the previous byte was a backslash escaping this one
	for i := 0; i < len(body); i++ {
		c := body[i]
		if escaped {
			b.WriteByte(c)
			escaped = false
			continue
		}
		if depth > 0 {
			switch c {
			case '{':
				depth++
			case '}':
				depth--
			}
			b.WriteByte(c)
			continue
		}
		switch {
		case c == '\\':
			escaped = true
			b.WriteByte(c)
		case c == '#' && i+1 < len(body) && body[i+1] == '{':
			b.WriteString("#{")
			i++
			depth = 1
		case c == '"':
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// countByte counts occurrences of b in s.
func countByte(s []byte, b byte) int {
	n := 0
	for _, c := range s {
		if c == b {
			n++
		}
	}
	return n
}

func (l *Lexer) lexString(spaceBefore bool, line, col int) token.Token {
	l.advance() // opening quote
	lit, interp := l.scanStringSegment()
	if !interp {
		l.state = exprEnd
		return token.Token{Type: token.STRING, Lit: lit, Line: line, Col: col, SpaceBefore: spaceBefore}
	}
	l.interpBraces = append(l.interpBraces, 0)
	l.state = exprBegin
	return token.Token{Type: token.STRBEG, Lit: lit, Line: line, Col: col, SpaceBefore: spaceBefore}
}

// lexSingleQuote lexes a single-quoted string. It does not interpolate; the
// only escapes are \' (a literal quote) and \\ (a literal backslash) — every
// other character, including a backslash before anything else, is taken
// verbatim, exactly as in MRI. It emits a finished STRING token.
func (l *Lexer) lexSingleQuote(spaceBefore bool, line, col int) token.Token {
	l.advance() // opening quote
	var b []byte
	for {
		c := l.peek()
		if c == 0 {
			return token.Token{Type: token.ILLEGAL, Lit: "unterminated string literal", Line: line, Col: col, SpaceBefore: spaceBefore}
		}
		if c == '\'' {
			l.advance() // closing quote
			break
		}
		if c == '\\' && (l.peek2() == '\'' || l.peek2() == '\\') {
			l.advance() // backslash
			b = append(b, l.advance())
			continue
		}
		b = append(b, l.advance())
	}
	l.state = exprEnd
	return token.Token{Type: token.STRING, Lit: string(b), Line: line, Col: col, SpaceBefore: spaceBefore}
}

// continueString resumes lexing a string after an interpolation's closing '}',
// returning STRMID if another interpolation follows or STREND at the close.
func (l *Lexer) continueString(line, col int) token.Token {
	lit, interp := l.scanStringSegment()
	if interp {
		l.interpBraces = append(l.interpBraces, 0)
		l.state = exprBegin
		return token.Token{Type: token.STRMID, Lit: lit, Line: line, Col: col}
	}
	l.state = exprEnd
	return token.Token{Type: token.STREND, Lit: lit, Line: line, Col: col}
}

// scanStringSegment reads a run of string content (with escapes) up to the
// closing quote (consumed) or an unescaped "#{" (consumed); the bool reports
// whether an interpolation follows.
func (l *Lexer) scanStringSegment() (string, bool) {
	var b []byte
	for {
		c := l.peek()
		if c == 0 || c == '"' {
			if c == '"' {
				l.advance()
			}
			return string(b), false
		}
		if c == '#' && l.peek2() == '{' {
			l.advance()
			l.advance()
			return string(b), true
		}
		if c == '\\' {
			l.advance()
			esc := l.advance()
			switch esc {
			case 'n':
				b = append(b, '\n')
			case 't':
				b = append(b, '\t')
			case 'r':
				b = append(b, '\r')
			case 'a':
				b = append(b, 0x07)
			case 'b':
				b = append(b, 0x08)
			case 'v':
				b = append(b, 0x0b)
			case 'f':
				b = append(b, 0x0c)
			case 's':
				b = append(b, ' ')
			case '\\':
				b = append(b, '\\')
			case '"':
				b = append(b, '"')
			case 'e':
				b = append(b, 0x1b)
			case '0':
				b = append(b, 0)
			default:
				b = append(b, esc)
			}
			continue
		}
		b = append(b, l.advance())
	}
}

func isDigit(c byte) bool      { return c >= '0' && c <= '9' }
func isIdentStart(c byte) bool { return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
func isIdentPart(c byte) bool  { return isIdentStart(c) || isDigit(c) }

func stripUnderscores(s string) string {
	if !hasUnderscore(s) {
		return s
	}
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '_' {
			b = append(b, s[i])
		}
	}
	return string(b)
}

func hasUnderscore(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '_' {
			return true
		}
	}
	return false
}
