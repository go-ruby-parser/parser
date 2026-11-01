// Package token defines the lexical tokens for the Phase 0 subset.
//
// The token set is intentionally richer than strictly necessary (it carries
// SpaceBefore, the seed of MRI's spaceSeen) so the parser can disambiguate
// command calls (`foo -1` vs `foo - 1`) without the lexer being rewritten as
// the grammar grows (plan-rbgo.md §10).
package token

import (
	"unicode"
	"unicode/utf8"
)

type Type int

const (
	EOF Type = iota
	ILLEGAL
	NEWLINE // \n or ;

	INT
	FLOAT
	STRING
	STRBEG  // "…#{  (start of an interpolated string; Lit = literal prefix)
	STRMID  // }…#{  (literal between two interpolations)
	STREND  // }…"   (literal suffix ending an interpolated string)
	IDENT   // local variable or method name (lowercase / _ leading)
	CONST   // Capitalized identifier
	IVAR    // @instance_variable
	CVAR    // @@class_variable
	GVAR    // $global / $~ / $1
	SYMBOL  // :name
	LABEL   // name: in a hash literal
	REGEXP  // /pattern/flags (Lit = pattern source, Flags = matched flag letters)
	XSTRING // `cmd` / %x{cmd} backtick command literal (Lit = raw command source)
	WORDS   // %w[…] word-array literal (Lit = raw body, Words = the split words)
	SYMBOLS // %i[…] symbol-array literal (Lit = raw body, Words = the split words)

	// Keywords.
	DEF
	CLASS
	MODULE
	END
	IF
	ELSIF
	ELSE
	UNLESS
	WHILE
	UNTIL
	RETURN
	BREAK
	NEXT
	BEGIN
	RESCUE
	ENSURE
	CASE
	WHEN
	IN
	FOR
	RETRY
	THEN
	DO
	TRUE
	FALSE
	NIL
	SELF
	SUPER
	YIELD
	AND   // `and` low-precedence logical-and keyword
	OR    // `or` low-precedence logical-or keyword
	NOT   // `not` low-precedence logical-not keyword
	ALIAS // `alias` method-aliasing keyword
	UNDEF // `undef` method-removal keyword

	// Operators and delimiters.
	PLUS
	MINUS
	STAR
	POW // **
	SLASH
	PERCENT
	ASSIGN
	EQ
	EQQ   // ===
	MATCH // =~
	NEQ
	LT
	GT
	LE
	GE
	SPACESHIP // <=>
	OPASSIGN  // compound assignment (+=, -=, ||=, …); Lit holds the operator
	SHOVEL    // <<
	ANDAND    // &&
	OROR      // ||
	BANG
	QUESTION // ?
	COLON    // : (ternary separator)
	SCOPE    // :: (constant scope resolution)
	LPAREN
	RPAREN
	LBRACE
	RBRACE
	LBRACKET
	RBRACKET
	PIPE
	HASHROCKET // =>
	COMMA
	DOT
	DOTDOT    // ..
	DOTDOTDOT // ...
	AMPER     // & (block-pass / block param)
	SAFEDOT   // &. (safe navigation)
	ARROW     // -> (stabby lambda)
	CARET     // ^ (pattern-matching pin operator)
	RSHIFT    // >>
	TILDE     // ~ (bitwise complement)
	NMATCH    // !~ (does-not-match operator)
)

var typeNames = map[Type]string{
	EOF: "EOF", ILLEGAL: "ILLEGAL", NEWLINE: "NEWLINE", INT: "INT", FLOAT: "FLOAT",
	STRING: "STRING", STRBEG: "STRBEG", STRMID: "STRMID", STREND: "STREND", IDENT: "IDENT", CONST: "CONST", IVAR: "IVAR", CVAR: "CVAR", GVAR: "GVAR", SYMBOL: "SYMBOL", LABEL: "LABEL", REGEXP: "REGEXP", XSTRING: "XSTRING", WORDS: "WORDS", SYMBOLS: "SYMBOLS",
	DEF: "def", CLASS: "class", MODULE: "module", END: "end",
	IF: "if", ELSIF: "elsif", ELSE: "else", UNLESS: "unless", WHILE: "while",
	UNTIL: "until", RETURN: "return", BREAK: "break", NEXT: "next", BEGIN: "begin", RESCUE: "rescue", ENSURE: "ensure", CASE: "case", WHEN: "when", IN: "in", FOR: "for", RETRY: "retry",
	THEN: "then", DO: "do", TRUE: "true", FALSE: "false", NIL: "nil", SELF: "self",
	SUPER: "super", YIELD: "yield", AND: "and", OR: "or", NOT: "not",
	ALIAS: "alias", UNDEF: "undef",
	PLUS: "+", MINUS: "-", STAR: "*", POW: "**", SLASH: "/", PERCENT: "%", ASSIGN: "=",
	EQ: "==", EQQ: "===", MATCH: "=~", NEQ: "!=", LT: "<", GT: ">", LE: "<=", GE: ">=", BANG: "!",
	SPACESHIP: "<=>", SHOVEL: "<<", ANDAND: "&&", OROR: "||", OPASSIGN: "op=", QUESTION: "?", COLON: ":", SCOPE: "::",
	LPAREN: "(", RPAREN: ")", LBRACE: "{", RBRACE: "}", LBRACKET: "[", RBRACKET: "]",
	PIPE: "|", HASHROCKET: "=>", COMMA: ",", DOT: ".", DOTDOT: "..", DOTDOTDOT: "...",
	AMPER: "&", SAFEDOT: "&.", ARROW: "->", CARET: "^", RSHIFT: ">>", TILDE: "~", NMATCH: "!~",
}

func (t Type) String() string {
	if s, ok := typeNames[t]; ok {
		return s
	}
	return "Type?"
}

// Keywords maps reserved words to their token type.
var Keywords = map[string]Type{
	"def": DEF, "class": CLASS, "module": MODULE, "end": END,
	"if": IF, "elsif": ELSIF, "else": ELSE,
	"unless": UNLESS, "while": WHILE, "until": UNTIL, "return": RETURN,
	"break": BREAK, "next": NEXT,
	"begin": BEGIN, "rescue": RESCUE, "ensure": ENSURE,
	"case": CASE, "when": WHEN, "in": IN, "for": FOR, "retry": RETRY,
	"then": THEN, "do": DO,
	"true": TRUE, "false": FALSE, "nil": NIL, "self": SELF, "super": SUPER,
	"yield": YIELD, "and": AND, "or": OR, "not": NOT,
	"alias": ALIAS, "undef": UNDEF,
}

// Token is a single lexed token.
type Token struct {
	Type        Type
	Lit         string
	Flags       string   // regexp flag letters in source order (i, m, x, o and the encoding flags n, u, e, s), only set for REGEXP tokens
	Words       []string // the words of a WORDS/SYMBOLS literal, already split and unescaped; only set for those two types
	Line        int
	Col         int
	SpaceBefore bool // whitespace immediately preceded this token (MRI spaceSeen)
}

// LookupIdent returns the keyword type for s, or IDENT/CONST otherwise.
func LookupIdent(s string) Type {
	if kw, ok := Keywords[s]; ok {
		return kw
	}
	if IsConstStart(s) {
		return CONST
	}
	return IDENT
}

// IsConstStart reports whether s begins with a character Ruby treats as the
// start of a CONSTANT, rather than of a local variable.
//
// ⛔ The test used to be `s[0] >= 'A' && s[0] <= 'Z'`, which reads a BYTE. For
// any multibyte character s[0] is a UTF-8 lead byte of 0xC2 or more, never in
// 'A'..'Z', so every non-ASCII name was classified as a local — `ἍBB = 1`
// created a local variable and no constant existed afterwards, where MRI reports
// `defined?(ἍBB)` as "constant".
//
// The rule is not the lexer's: parse.y assumes tCONSTANT provisionally and
// DOWNGRADES it with `if (result == tCONSTANT && is_local_id(ident)) result =
// tIDENTIFIER` (parse.y:10453 at v4.0.9), so the decision belongs to the ID
// system. rb_sym_constant_char_p (symbol.c:508-542) is where it is made:
//
//	ASCII                     -> ISUPPER
//	non-ASCII, rb_enc_isupper -> constant
//	non-ASCII, rb_enc_islower -> local
//	titlecase letter, Unicode -> constant
//	anything else (caseless)  -> local
//
// Go's unicode.IsUpper is that isupper and unicode.IsTitle that titlecase
// property, so the two clauses below are those four lines. Measured against
// /usr/bin/ruby through `Object.const_defined?`, which raises NameError for a
// name that is not a constant name (rb_is_const_name, symbol.c:1287): Greek Ἅ,
// Cyrillic Д and Turkish İ are constants; the titlecase digraphs ǅ and ǈ are
// constants too; Greek μ, CJK 日, Hebrew א and Arabic ب are not — a caseless
// letter starts a LOCAL variable in Ruby, which is why neither clause may be
// loosened to "not lower".
func IsConstStart(s string) bool {
	if s == "" {
		return false
	}
	if c := s[0]; c < utf8.RuneSelf {
		return c >= 'A' && c <= 'Z'
	}
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError && size <= 1 {
		// Invalid UTF-8: rb_sym_constant_char_p returns FALSE when
		// rb_enc_precise_mbclen finds no character, so this is a local.
		return false
	}
	return unicode.IsUpper(r) || unicode.IsTitle(r)
}
