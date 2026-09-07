// Package ast defines the immutable abstract syntax tree nodes produced by the
// parser and consumed by the resolver, type checker, and evaluator. Nodes are
// pure data; every node carries its source span via the embedded Base.
package ast

import "github.com/Jh123x/prompiler/internal/token"

// Node is implemented by every AST node.
type Node interface {
	Span() token.Span
}

// Base embeds a source span into a concrete node.
type Base struct {
	S token.Span
}

// Span returns the node's source span.
func (b Base) Span() token.Span { return b.S }

// Expr is a marker interface for expressions.
type Expr interface {
	Node
	exprNode()
}

// Stmt is a marker interface for statements.
type Stmt interface {
	Node
	stmtNode()
}

// TypeRef is a marker interface for syntactic type references.
type TypeRef interface {
	Node
	typeRefNode()
}

// Decl is a marker interface for top-level declarations.
type Decl interface {
	Node
	declNode()
}

// PromptSegment is a marker interface for prompt-body constructs.
type PromptSegment interface {
	Node
	promptSegmentNode()
}

// --- Expressions ---

// IntLit is an integer literal.
type IntLit struct {
	Base
	Value int64
}

// FloatLit is a float literal.
type FloatLit struct {
	Base
	Value float64
}

// StringLit is a string literal (unescaped value).
type StringLit struct {
	Base
	Value string
}

// BoolLit is a boolean literal.
type BoolLit struct {
	Base
	Value bool
}

// NoneLit is the `none` literal.
type NoneLit struct{ Base }

// ArrayLit is an array literal `[1, 2, 3]`.
type ArrayLit struct {
	Base
	Elems []Expr
}

// MapEntry is a single key/value entry in a map literal.
type MapEntry struct {
	Key   Expr
	Value Expr
}

// MapLit is a map literal `{ 'a': 1, 'b': 2 }`.
type MapLit struct {
	Base
	Entries []MapEntry
}

// FieldInit is a field initializer in a class literal.
type FieldInit struct {
	Name  string
	Value Expr
}

// ClassLit is a class literal `Point { x: 1, y: 2 }` (or `Pair<string,int>{...}`).
type ClassLit struct {
	Base
	TypeName string
	TypeArgs []TypeRef
	Fields   []FieldInit
}

// Ident is a bare identifier reference.
type Ident struct {
	Base
	Name string
}

// FieldAccess is `expr.field`. Enum member access (`Difficulty.Easy`) is also
// represented as FieldAccess{Recv: Ident{Difficulty}, Field: Easy}; the
// resolver/checker disambiguates by whether the receiver names an enum type.
type FieldAccess struct {
	Base
	Recv  Expr
	Field string
}

// Index is `expr[expr]`.
type Index struct {
	Base
	Recv Expr
	Idx  Expr
}

// Call is `callee(args)` — a free function, cast, or some(...). TypeArgs holds
// explicit generic type arguments when present.
type Call struct {
	Base
	Callee   Expr
	TypeArgs []TypeRef
	Args     []Expr
}

// MethodCall is `recv.method(args)`.
type MethodCall struct {
	Base
	Recv     Expr
	Method   string
	TypeArgs []TypeRef
	Args     []Expr
}

// Unary is a prefix operation `!` or `-`.
type Unary struct {
	Base
	Op      token.TokenType
	Operand Expr
}

// Binary is a binary operation.
type Binary struct {
	Base
	Op token.TokenType
	L  Expr
	R  Expr
}

func (*IntLit) exprNode()      {}
func (*FloatLit) exprNode()    {}
func (*StringLit) exprNode()   {}
func (*BoolLit) exprNode()     {}
func (*NoneLit) exprNode()     {}
func (*ArrayLit) exprNode()    {}
func (*MapLit) exprNode()      {}
func (*ClassLit) exprNode()    {}
func (*Ident) exprNode()       {}
func (*FieldAccess) exprNode() {}
func (*Index) exprNode()       {}
func (*Call) exprNode()        {}
func (*MethodCall) exprNode()  {}
func (*Unary) exprNode()       {}
func (*Binary) exprNode()      {}

// --- Statements ---

// VarStmt is a mutable local binding `var name = expr`.
type VarStmt struct {
	Base
	Name string
	Init Expr
}

// AssignStmt is `target = value`, where target is an Ident or Index.
type AssignStmt struct {
	Base
	Target Expr
	Value  Expr
}

// IfStmt is `if (cond) { then } else { else }`.
type IfStmt struct {
	Base
	Cond Expr
	Then []Stmt
	Else []Stmt
}

// ForStmt is `for (x in iter) { body }`.
type ForStmt struct {
	Base
	Var  string
	Iter Expr
	Body []Stmt
}

// ReturnStmt is `return value`.
type ReturnStmt struct {
	Base
	Value Expr
}

func (*VarStmt) stmtNode()    {}
func (*AssignStmt) stmtNode() {}
func (*IfStmt) stmtNode()     {}
func (*ForStmt) stmtNode()    {}
func (*ReturnStmt) stmtNode() {}

// --- Type references ---

// NamedType is a named type, optionally instantiated with type arguments.
type NamedType struct {
	Base
	Name     string
	TypeArgs []TypeRef
}

// ArrayType is `T[]`.
type ArrayType struct {
	Base
	Elem TypeRef
}

// MapType is `map<string, T>`.
type MapType struct {
	Base
	Value TypeRef
}

// OptionalType is `Optional<T>`.
type OptionalType struct {
	Base
	Elem TypeRef
}

func (*NamedType) typeRefNode()    {}
func (*ArrayType) typeRefNode()    {}
func (*MapType) typeRefNode()      {}
func (*OptionalType) typeRefNode() {}

// --- Declarations ---

// EnumDecl is `enum Name { A, B }`.
type EnumDecl struct {
	Base
	Name    string
	Members []string
}

// Field is a class/interface field declaration `name: Type`.
type Field struct {
	Name string
	Type TypeRef
}

// Param is a function/method parameter.
type Param struct {
	Name string
	Type TypeRef
}

// TypeParam is a generic type parameter `<Name: Bound>` (Bound nil = unconstrained).
type TypeParam struct {
	Name  string
	Bound TypeRef
}

// MethodDecl is a class method (Body non-nil) or interface signature (Body nil).
type MethodDecl struct {
	Base
	Name       string
	TypeParams []TypeParam
	Params     []Param
	Return     TypeRef
	Body       []Stmt
}

// ClassDecl is `class Name<...> { fields; methods }`.
type ClassDecl struct {
	Base
	Name       string
	TypeParams []TypeParam
	Fields     []Field
	Methods    []MethodDecl
}

// InterfaceDecl is `interface Name<...> { fields; signatures }`.
type InterfaceDecl struct {
	Base
	Name       string
	TypeParams []TypeParam
	Fields     []Field
	Methods    []MethodDecl
}

// FuncDecl is a top-level `func name<...>(params): Ret { body }`.
type FuncDecl struct {
	Base
	Name       string
	TypeParams []TypeParam
	Params     []Param
	Return     TypeRef
	Body       []Stmt
}

// ImportDecl is `import "path.ppl"`.
type ImportDecl struct {
	Base
	Path string
}

// VarDecl is a template variable `name: Type [= default]`.
type VarDecl struct {
	Base
	Name       string
	Type       TypeRef
	Default    Expr
	HasDefault bool
}

// TemplateDecl is `template Name { description; variables; prompt }`.
type TemplateDecl struct {
	Base
	Name        string
	Description string
	Variables   []VarDecl
	Prompt      *PromptBody
}

func (*EnumDecl) declNode()      {}
func (*ClassDecl) declNode()     {}
func (*InterfaceDecl) declNode() {}
func (*FuncDecl) declNode()      {}
func (*ImportDecl) declNode()    {}
func (*TemplateDecl) declNode()  {}

// --- Prompt body ---

// PromptBody is a template's prompt block: raw text plus parsed nested segments.
type PromptBody struct {
	Base
	Raw      string
	Segments []PromptSegment
}

// TextSegment is a run of literal prompt text (escapes resolved).
type TextSegment struct {
	Base
	Text string
}

// InterpSegment is `{{ expr }}`.
type InterpSegment struct {
	Base
	Expr Expr
}

// NamedArg is a named include argument.
type NamedArg struct {
	Name  string
	Value Expr
}

// IncludeSegment is `{{ include Template(name: arg, ...) }}`.
type IncludeSegment struct {
	Base
	Template string
	Args     []NamedArg
}

// ForSegment is `{% for var in iter %} body {% end %}`.
type ForSegment struct {
	Base
	Var  string
	Iter Expr
	Body []PromptSegment
}

// IfSegment is `{% if cond %} then {% else %} else {% end %}`.
type IfSegment struct {
	Base
	Cond Expr
	Then []PromptSegment
	Else []PromptSegment
}

func (*TextSegment) promptSegmentNode()    {}
func (*InterpSegment) promptSegmentNode()  {}
func (*IncludeSegment) promptSegmentNode() {}
func (*ForSegment) promptSegmentNode()     {}
func (*IfSegment) promptSegmentNode()      {}

// File is a single parsed source file.
type File struct {
	Path  string
	Decls []Decl
}
