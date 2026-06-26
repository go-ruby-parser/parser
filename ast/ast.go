// Package ast defines the Phase 0 abstract syntax tree.
//
// Everything in Ruby is an expression, so there is no statement/expression
// split: a body is a slice of Node and its value is the value of its last node.
package ast

import "math/big"

// Node is any AST node.
type Node interface{ node() }

// Program is the top-level sequence of expressions.
type Program struct{ Body []Node }

// IntLit is an integer literal that fits in int64.
type IntLit struct{ Value int64 }

// BignumLit is an integer literal too large for int64 (held as a *big.Int).
type BignumLit struct{ Val *big.Int }

// FloatLit is a floating-point literal.
type FloatLit struct{ Value float64 }

// StringLit is a (Phase 0: non-interpolated) string literal.
type StringLit struct{ Value string }

// SymbolLit is a symbol literal (:name); Name excludes the leading colon.
type SymbolLit struct{ Name string }

// RegexpLit is a regexp literal /source/flags. Flags holds the subset of the
// flag letters i, m, x that were present.
type RegexpLit struct {
	Source string
	Flags  string
}

// XStr is a backtick command literal (`cmd` or %x{cmd}): its (Phase-0,
// non-interpolated) Command source is run by the shell, yielding its stdout.
type XStr struct{ Command string }

// ArrayLit is an array literal [a, b, c].
type ArrayLit struct{ Elems []Node }

// HashLit is a hash literal {k => v, …}; Keys[i] maps to Values[i].
type HashLit struct {
	Keys   []Node
	Values []Node
}

// RangeLit is a range literal: Lo..Hi (inclusive) or Lo...Hi (Exclusive).
type RangeLit struct {
	Lo, Hi    Node
	Exclusive bool
}

// BoolLit is true or false.
type BoolLit struct{ Value bool }

// NilLit is nil.
type NilLit struct{}

// SelfLit is self.
type SelfLit struct{}

// VarRef references a local variable known to the current scope.
type VarRef struct{ Name string }

// Assign is `Name = Value` (local assignment).
type Assign struct {
	Name  string
	Value Node
}

// BinaryExpr is `Left Op Right` for the Phase 0 fast-path operators.
type BinaryExpr struct {
	Op    string
	Left  Node
	Right Node
}

// UnaryExpr is `Op Operand` (- or !).
type UnaryExpr struct {
	Op      string
	Operand Node
}

// Call is a method call. Recv is nil for a self/funcall (e.g. `puts x`, `fib(n)`).
// Block is an optional literal block ({…} or do…end) attached to the call.
type Call struct {
	Recv  Node
	Name  string
	Args  []Node
	Block *Block
	Safe  bool // &. safe navigation: nil receiver short-circuits to nil
}

// Block is a literal block: parameters and a body. It is a closure over the
// scope in which it appears. Defaults parallels Params (nil for a required or
// *splat param; non-nil for an optional `name = expr` param), exactly as
// MethodDef records its method-parameter defaults.
type Block struct {
	Params     []string
	Defaults   []Node // parallel to Params; nil for a required or *splat param
	SplatIndex int    // index of the top-level *splat param in Params, or -1
	BlockParam string // name of the &block param, or "" if none (parallels MethodDef.BlockParam)
	Body       []Node
}

// Yield invokes the block passed to the enclosing method.
type Yield struct {
	Args []Node
}

// If is an if/elsif/else expression.
type If struct {
	Cond   Node
	Then   []Node
	Elsifs []Elsif
	Else   []Node // nil if absent
}

// Elsif is one elsif branch.
type Elsif struct {
	Cond Node
	Body []Node
}

// While is a while loop. Its value is nil.
type While struct {
	Cond Node
	Body []Node
}

// MethodDef defines a method on the current self.
type MethodDef struct {
	Name       string
	Params     []string
	Defaults   []Node    // parallel to Params; nil for a required param
	SplatIndex int       // index of the *splat param in Params, or -1
	KwParams   []KwParam // keyword parameters (a:, b: default), after positionals
	KwRest     string    // name of the **rest keyword-splat param, or "" if none
	BlockParam string    // name of the &block param, or "" if none
	Singleton  bool      // def self.foo — a singleton (class) method
	Recv       Node      // def recv.foo — explicit non-self receiver (nil otherwise)
	Forward    bool      // def f(...) / def f(a, ...) — accepts forwarded arguments
	Body       []Node
}

// KwParam is a single keyword parameter. Default is nil for a required keyword
// (`a:`); non-nil for an optional one (`a: expr`).
type KwParam struct {
	Name    string
	Default Node
}

// Return is an explicit return.
type Return struct{ Value Node } // Value may be nil

// ConstRef references a constant (e.g. a class name) by name.
type ConstRef struct{ Name string }

// ScopedConst is a constant looked up through ::, e.g. Math::PI or Foo::BAR.
// Recv evaluates to the module/class whose constant table is consulted. A nil
// Recv with Global true is a leading-`::` top-level lookup (`::Foo`).
type ScopedConst struct {
	Recv   Node
	Name   string
	Global bool // true for a leading `::Name` (top-level constant lookup)
}

// GVarRef references a global variable by name ("$~", "$1", "$stdout", …).
type GVarRef struct{ Name string }

// GVarAssign is `$Name = Value`.
type GVarAssign struct {
	Name  string
	Value Node
}

// MultiAssign is a destructuring assignment to local targets: a, b = 1, 2 or
// a, *b = list. SplatIndex is the index of the *splat target in Names, or -1.
//
// Targets, when non-nil, is parallel to Names and holds the general LHS target
// node for each position (e.g. a *ConstRef for a constant target `A, B = …`).
// It is left nil for the all-locals case so consumers that only understand local
// targets keep working; when any target is not a plain local, Targets is
// populated for every position and Names mirrors it (the local name, or "" for a
// non-local target whose splat capture, if any, has no local name).
type MultiAssign struct {
	Names      []string
	Targets    []Node // nil for all-locals; else parallel to Names
	SplatIndex int
	Values     []Node
}

// ConstAssign assigns to a constant: NAME = value.
type ConstAssign struct {
	Name  string
	Value Node
}

// IvarRef reads an instance variable (@name) of self.
type IvarRef struct{ Name string }

// IvarAssign is `@Name = Value`.
type IvarAssign struct {
	Name  string
	Value Node
}

// CVarRef reads a class variable (@@name).
type CVarRef struct{ Name string }

// CVarAssign is `@@Name = Value`.
type CVarAssign struct {
	Name  string
	Value Node
}

// ClassDef defines or reopens a class. Super is the optional superclass name.
//
// Name carries the simple constant name; for a scope-resolution path
// (`class Foo::Bar`) or a leading-`::` name (`class ::Bar`) NamePath holds the
// full ScopedConst and Name is the trailing segment. SuperExpr, when non-nil,
// holds a superclass that is a `::`-path or otherwise not a bare constant; for a
// bare-constant superclass Super holds its name and SuperExpr is nil.
type ClassDef struct {
	Name      string
	NamePath  Node   // *ScopedConst for `class A::B` / `class ::B`, else nil
	Super     string // "" if none and SuperExpr is nil
	SuperExpr Node   // non-nil superclass expression (e.g. a `::`-path), else nil
	Body      []Node
}

// ModuleDef defines or reopens a module. NamePath mirrors ClassDef.NamePath: it
// is non-nil for a scope-resolution or leading-`::` module name, in which case
// Name is the trailing segment.
type ModuleDef struct {
	Name     string
	NamePath Node // *ScopedConst for `module A::B` / `module ::B`, else nil
	Body     []Node
}

// SingletonClassDef opens the singleton (metaclass) of Target: `class << Target;
// Body; end`. Methods and constants defined in Body attach to Target's singleton
// class. `class << self` (Target is *SelfLit) is the idiomatic form for defining
// class methods; `class << obj` opens an arbitrary object's singleton class.
type SingletonClassDef struct {
	Target Node
	Body   []Node
}

// Super calls the same-named method in the ancestor chain. Forward is true for a
// bare `super` (passes the enclosing method's own arguments); otherwise Args are
// the explicit arguments of `super(...)`.
type Super struct {
	Args    []Node
	Forward bool
}

// Break exits the innermost block (terminating its iterator) or loop. Value may
// be nil.
type Break struct{ Value Node }

// Next skips to the next iteration of the innermost block or loop. Value may be
// nil.
type Next struct{ Value Node }

// OpAssign is compound assignment to a local: `Name Op= Value` (e.g. x += 1,
// x ||= 5). Compiled so a fresh local is allocated before its read.
type OpAssign struct {
	Name  string
	Op    string
	Value Node
}

// StrInterp is an interpolated double-quoted string: the parts alternate string
// literals and embedded expressions, all coerced with to_s and concatenated.
type StrInterp struct{ Parts []Node }

// Case is `case [Subject] (when …)* [else …] end`. With a Subject each `when`
// matches via `cond === Subject`; without one, each `when` is a boolean test.
type Case struct {
	Subject Node // nil for the condition form
	Whens   []WhenClause
	Else    []Node // nil if no else
}

// WhenClause is one `when COND[, COND…] [then] BODY`.
type WhenClause struct {
	Conds []Node
	Body  []Node
}

// CaseIn is `case Subject (in PATTERN [guard]; BODY)* [else BODY] end` — Ruby
// pattern matching. Unlike Case, each clause matches the subject against a
// structural Pattern (deconstruction + binding), and a failed match with no
// else raises NoMatchingPatternError.
type CaseIn struct {
	Subject Node
	Clauses []InClause
	Else    []Node // nil if no else
}

// InClause is one `in PATTERN [if cond | unless cond] [then] BODY`. GuardNeg is
// true for an `unless` guard. Guard is nil when there is no guard.
type InClause struct {
	Pattern  Pattern
	Guard    Node
	GuardNeg bool
	Body     []Node
}

// MatchPattern is a one-line pattern match: `Subject => Pattern` (Bool false:
// rightward assignment — binds on success, raises NoMatchingPatternError on
// failure) or `Subject in Pattern` (Bool true: a boolean test — true/false,
// binding on success).
type MatchPattern struct {
	Subject Node
	Pattern Pattern
	Bool    bool
}

// Pattern is any case/in pattern node.
type Pattern interface{ pattern() }

// ValuePattern matches when `Value === subject` (literals, ranges, pinned
// expressions): the subject is tested but not destructured.
type ValuePattern struct{ Value Node }

// BindPattern binds the whole subject to a local (`in x`, the wildcard `in _`).
type BindPattern struct{ Name string }

// ConstPattern matches when `subject.is_a?(Const)` (`in Integer`, `in String`).
type ConstPattern struct{ Const Node }

// BindingPattern is `SubPattern => name`: it matches SubPattern, then binds the
// subject to name.
type BindingPattern struct {
	Sub  Pattern
	Name string
}

// HashPattern matches a hash-like subject via the deconstruct_keys protocol.
// Keys[i] is the symbol key; Values[i] is its sub-pattern, or nil for the
// shorthand `{name:}` that binds local `name`. RestNil requires no extra keys
// (`**nil`); RestName captures the remaining keys into a Hash (`**rest`);
// HasRest distinguishes `**rest` from no double-splat. Const, when non-nil,
// additionally requires `subject.is_a?(Const)` (`in Point(x:, y:)`).
type HashPattern struct {
	Const    Node
	Keys     []string
	Values   []Pattern
	HasRest  bool
	RestName string
	RestNil  bool
}

// ArrayPattern matches an array-like subject via the deconstruct protocol.
// Pre are the patterns before the splat, Post those after; HasSplat indicates a
// `*[name]` is present, with SplatName the (possibly empty) capture name. Const,
// when non-nil, additionally requires `subject.is_a?(Const)` (`in Point[x, y]`).
type ArrayPattern struct {
	Const     Node
	Pre       []Pattern
	HasSplat  bool
	SplatName string
	Post      []Pattern
}

// AltPattern is `p1 | p2 | …`: it matches when any alternative matches. Ruby
// forbids variable bindings inside alternatives, so the alternatives are value/
// constant/structural patterns tested without binding.
type AltPattern struct{ Alts []Pattern }

// FindPattern is the array find pattern `[*pre, mid…, *post]`: it scans the
// deconstructed array for the first window where every Mid sub-pattern matches
// consecutively, binding PreName to the elements before it and PostName to those
// after. Const, when non-nil, additionally requires subject.is_a?(Const).
type FindPattern struct {
	Const    Node
	PreName  string
	Mid      []Pattern
	PostName string
}

// Retry restarts the enclosing begin body from inside a rescue clause.
type Retry struct{}

// SplatArg is a `*expr` argument or array element: its (array) value is spliced
// into the surrounding argument list / array.
type SplatArg struct{ Value Node }

// BlockPass is a `&expr` argument: its value is coerced to a Proc (via to_proc)
// and passed as the call's block. It only ever appears last in a Call's args.
type BlockPass struct{ Value Node }

// ForwardArgs is the `...` forwarding argument inside a call (`g(...)`): it
// splices the enclosing method's forwarded positional, keyword, and block
// arguments into this call. It only appears in a method that declared `...`.
type ForwardArgs struct{}

// Begin is `begin BODY (rescue …)* [else …] [ensure …] end`.
type Begin struct {
	Body       []Node
	Rescues    []RescueClause
	ElseBody   []Node // nil if no else
	EnsureBody []Node // nil if no ensure
}

// RescueClause is one `rescue [Classes] [=> Var] BODY`. Empty Classes means
// rescue StandardError.
type RescueClause struct {
	Classes []Node
	Var     string
	Body    []Node
}

func (*Program) node()           {}
func (*ScopedConst) node()       {}
func (*IntLit) node()            {}
func (*BignumLit) node()         {}
func (*FloatLit) node()          {}
func (*StringLit) node()         {}
func (*SymbolLit) node()         {}
func (*RegexpLit) node()         {}
func (*XStr) node()              {}
func (*ArrayLit) node()          {}
func (*HashLit) node()           {}
func (*RangeLit) node()          {}
func (*BoolLit) node()           {}
func (*NilLit) node()            {}
func (*SelfLit) node()           {}
func (*VarRef) node()            {}
func (*Assign) node()            {}
func (*BinaryExpr) node()        {}
func (*UnaryExpr) node()         {}
func (*Call) node()              {}
func (*If) node()                {}
func (*While) node()             {}
func (*MethodDef) node()         {}
func (*Return) node()            {}
func (*ConstRef) node()          {}
func (*ConstAssign) node()       {}
func (*GVarRef) node()           {}
func (*GVarAssign) node()        {}
func (*CVarRef) node()           {}
func (*CVarAssign) node()        {}
func (*MultiAssign) node()       {}
func (*MatchPattern) node()      {}
func (*IvarRef) node()           {}
func (*IvarAssign) node()        {}
func (*ClassDef) node()          {}
func (*SingletonClassDef) node() {}
func (*ModuleDef) node()         {}
func (*Super) node()             {}
func (*Yield) node()             {}
func (*Break) node()             {}
func (*Next) node()              {}
func (*OpAssign) node()          {}
func (*Begin) node()             {}
func (*StrInterp) node()         {}
func (*Case) node()              {}
func (*Retry) node()             {}
func (*SplatArg) node()          {}
func (*BlockPass) node()         {}
func (*ForwardArgs) node()       {}
func (*CaseIn) node()            {}

func (*ValuePattern) pattern()   {}
func (*BindPattern) pattern()    {}
func (*ConstPattern) pattern()   {}
func (*BindingPattern) pattern() {}
func (*ArrayPattern) pattern()   {}
func (*HashPattern) pattern()    {}
func (*AltPattern) pattern()     {}
func (*FindPattern) pattern()    {}
