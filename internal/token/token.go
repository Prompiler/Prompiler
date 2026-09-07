// Package token defines the lexical token type, source positions/spans, and
// the shared Diagnostic currency used across every compiler stage.
package token

// Position is a source position. Offset is a 0-based byte offset; Line and
// Column are 1-based.
type Position struct {
	Offset int
	Line   int
	Column int
}

// Span is a half-open source range [Start, End).
type Span struct {
	Start Position
	End   Position
}

// TokenType enumerates every lexical token kind.
type TokenType int

const (
	ILLEGAL TokenType = iota
	EOF

	// Identifiers and literals.
	IDENT
	INT
	FLOAT
	STRING

	// Raw text of a `prompt { ... }` block. The lexer captures the verbatim
	// body (between the opening and closing brace, honoring `\{ \} \\` and the
	// `{{ }}` / `{% %}` construct delimiters) as this single token; the parser's
	// prompt sub-parser scans its Lexeme.
	PROMPT_BODY

	// Keywords.
	IMPORT
	ENUM
	CLASS
	INTERFACE
	FUNC
	TEMPLATE
	VARIABLES
	PROMPT
	INCLUDE
	VAR
	IF
	ELSE
	FOR
	IN
	RETURN
	THIS
	TRUE
	FALSE
	NONE
	STRING_KW // string
	INT_KW    // int
	FLOAT_KW  // float
	BOOL_KW   // bool
	MAP       // map
	ARRAY     // array (reserved; arrays use T[])
	OPTIONAL  // Optional

	// Operators and punctuation.
	PLUS     // +
	MINUS    // -
	STAR     // *
	SLASH    // /
	PERCENT  // %
	BANG     // !
	AND_AND  // &&
	OR_OR    // ||
	EQ_EQ    // ==
	NOT_EQ   // !=
	LT       // <
	LT_EQ    // <=
	GT       // >
	GT_EQ    // >=
	ASSIGN   // =
	LPAREN   // (
	RPAREN   // )
	LBRACE   // {
	RBRACE   // }
	LBRACKET // [
	RBRACKET // ]
	COMMA    // ,
	COLON    // :
	DOT      // .
)

// Token is a single lexical token.
type Token struct {
	Type     TokenType
	Lexeme   string
	Span     Span
	IntVal   int64
	FloatVal float64
}

var keywords = map[string]TokenType{
	"import":    IMPORT,
	"enum":      ENUM,
	"class":     CLASS,
	"interface": INTERFACE,
	"func":      FUNC,
	"template":  TEMPLATE,
	"variables": VARIABLES,
	"prompt":    PROMPT,
	"include":   INCLUDE,
	"var":       VAR,
	"if":        IF,
	"else":      ELSE,
	"for":       FOR,
	"in":        IN,
	"return":    RETURN,
	"this":      THIS,
	"true":      TRUE,
	"false":     FALSE,
	"none":      NONE,
	"string":    STRING_KW,
	"int":       INT_KW,
	"float":     FLOAT_KW,
	"bool":      BOOL_KW,
	"map":       MAP,
	"array":     ARRAY,
	"Optional":  OPTIONAL,
}

// LookupKeyword returns the keyword TokenType for an identifier, if any.
func LookupKeyword(ident string) (TokenType, bool) {
	t, ok := keywords[ident]
	return t, ok
}

// Stage is the compiler stage a diagnostic originates from.
type Stage string

const (
	StageParse     Stage = "parse"
	StageTypecheck Stage = "typecheck"
	StageRuntime   Stage = "runtime"
)

// Category is a diagnostic category. The exact strings are asserted by the
// conformance harness against examples/**/solution.json.
type Category string

// Type-check categories (spec §11, §16.2).
const (
	CatUnknownType          Category = "unknown_type"
	CatTypeMismatch         Category = "type_mismatch"
	CatIterateNonArray      Category = "iterate_non_array"
	CatThisOutsideMethod    Category = "this_outside_method"
	CatUnknownMethod        Category = "unknown_method"
	CatInterfaceUnsatisfied Category = "interface_unsatisfied"
	CatDuplicateMember      Category = "duplicate_member"
	CatDuplicateName        Category = "duplicate_name"
	CatDefaultNotAssignable Category = "default_not_assignable"
	CatEnumDefaultNotMember Category = "enum_default_not_member"
	CatCyclicInclude        Category = "cyclic_include"
)

// Runtime categories (spec §16.1).
const (
	CatIndexOutOfBounds Category = "index_out_of_bounds"
	CatDivisionByZero   Category = "division_by_zero"
	CatFailedCast       Category = "failed_cast"
	CatRangeNegative    Category = "range_negative"
	CatRangeStepZero    Category = "range_step_zero"
	CatUnwrapEmpty      Category = "unwrap_empty_optional"
)

// Placeholder categories (not fixture-asserted).
const (
	CatCyclicImport Category = "cyclic_import"
	CatUnknownName  Category = "unknown_name"
	CatOverflow     Category = "overflow"
)

// Diagnostic is a positioned compiler problem, the shared currency of all
// error reporting (architecture.md ubiquitous language).
type Diagnostic struct {
	Stage    Stage
	Category Category
	Span     Span
	Message  string
}
