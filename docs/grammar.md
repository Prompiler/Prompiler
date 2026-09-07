# Prompiler Grammar

The full syntax of Prompiler (`.ppl`), written as EBNF. This is the normative
reference for the parser and companion to `spec.md`.

**Notation**

```
X           non-terminal
"token"     literal terminal
'a'         literal character
A | B       alternation
[ A ]       optional (zero or one)
{ A }       repetition (zero or more)
( A B )     grouping
// ...      line comment describing a production
```

Terminal classes are written in UPPER_CASE (`INT`, `IDENT`, `STRING`, `TYPE_NAME`,
`FUNC_NAME`, `TEMPLATE_NAME`).

---

## 1. Lexical grammar

```
source_char   := any Unicode code point except NUL

// Comments (both are skipped; they never produce tokens)
line_comment  := "//" { source_char - newline }
block_comment := "/*" { source_char } "*/"

ident         := ("A".."Z" | "a".."z" | "_") { "A".."Z" | "a".."z" | "0".."9" | "_" }

keyword       := "import" | "enum" | "class" | "interface" | "func" | "template"
               | "variables" | "prompt" | "include"
               | "var" | "if" | "else" | "for" | "in" | "return" | "this"
               | "true" | "false" | "none"
               | "string" | "int" | "float" | "bool" | "map" | "array" | "Optional"

IDENT         := ident - keyword

// Literals
int_lit       := "0".."9" { "0".."9" }                          // no hex/octal/underscore
float_lit     := "0".."9" { "0".."9" } "." "0".."9" { "0".."9" } // no exponent, no leading/trailing dot
bool_lit      := "true" | "false"
none_lit      := "none"

string_lit    := '"' { string_char } '"' | "'" { string_char } "'"
string_char   := source_char - ('"' | "'" | "\" | newline) | escape
escape        := "\" ( "n" | "t" | "r" | "\\" | '"' | "'" )

// Operators (longest-match first)
"||" "&&" "==" "!=" "<=" ">=" "<" ">" "+" "-" "*" "/" "%" "!" "="
"(" ")" "{" "}" "[" "]" "," ":" "."
```

The source is treated as UTF-8 and CRLF is normalized to LF. String literals
have no multi-line form; `\n` is the newline escape.

---

## 2. Program structure

```
file           := { declaration }

declaration    := import_decl
                | enum_decl
                | class_decl
                | interface_decl
                | func_decl
                | template_decl

import_decl    := "import" STRING
```

A program is a set of files. `import "path.ppl"` is whole-file and relative to
the importing file; imports form an acyclic graph.

---

## 3. Types

```
type           := named_type
                | array_type
                | map_type
                | optional_type

named_type     := TYPE_NAME [ type_args ]
array_type     := type "[]"
map_type       := "map" "<" "string" "," type ">"
optional_type  := "Optional" "<" type ">"

type_args      := "<" type { "," type } [ "," ] ">"     // trailing comma allowed

type_param     := ident [ ":" named_type ]              // bound must be an interface
type_params    := "<" type_param { "," type_param } [ "," ] ">"
```

`TYPE_NAME` is a declared `enum`, `class`, or `interface` name (or a generic
type parameter). Generic type parameters appear in `class`/`interface`/`func`/
method headers; a bound is a structural interface the argument must satisfy.

---

## 4. Declarations

```
enum_decl      := "enum" TYPE_NAME "{" enum_member { "," enum_member } [ "," ] "}"
enum_member    := IDENT

class_decl     := "class" TYPE_NAME [ type_params ] "{"
                     { field }
                     { method }
                  "}"

interface_decl := "interface" TYPE_NAME [ type_params ] "{"
                     { field }
                     { method_signature }
                  "}"

field          := IDENT ":" type

func_decl      := "func" FUNC_NAME [ type_params ] "(" params ")" ":" type block

method         := "func" FUNC_NAME [ type_params ] "(" params ")" ":" type block
method_signature := "func" FUNC_NAME [ type_params ] "(" params ")" ":" type

params         := [ param { "," param } [ "," ] ]
param          := IDENT ":" type
```

---

## 5. Templates

```
template_decl  := "template" TEMPLATE_NAME "{"
                     [ "description" ":" STRING ]
                     "variables" "{" { variable } "}"
                     "prompt" "{" prompt_body "}"
                  "}"

variable       := IDENT ":" type [ "=" constant ]
constant       := literal | array_literal | map_literal | class_literal | enum_member | none_lit
```

A variable with `= constant` is optional (its default applies when the user
omits it); a variable without one is required. Defaults are constant-literal
expressions only.

---

## 6. Statements

```
block          := "{" { statement } "}"

statement      := var_stmt | assign_stmt | if_stmt | for_stmt | return_stmt

var_stmt       := "var" IDENT "=" expr
assign_stmt    := assign_target "=" expr
assign_target  := IDENT | expr "[" expr "]"

if_stmt        := "if" "(" expr ")" block [ "else" block ]
for_stmt       := "for" "(" IDENT "in" expr ")" block
return_stmt    := "return" expr
```

There is no `while` and no expression statement. Reassignment (`IDENT "=" expr`)
is allowed for local `var` bindings and parameters; element assignment
(`expr "[" expr "]" "=" expr`) is allowed on arrays and maps only. Loop
variables are immutable.

---

## 7. Expressions

Precedence, lowest to highest:

```
expr        := or_expr

or_expr     := and_expr { "||" and_expr }                    // left-assoc
and_expr    := equality { "&&" equality }                    // left-assoc
equality    := comparison { ("==" | "!=") comparison }       // left-assoc
comparison  := additive { ("<" | "<=" | ">" | ">=") additive } // left-assoc
additive    := multiplicative { ("+" | "-") multiplicative } // left-assoc
multiplicative := unary { ("*" | "/" | "%") unary }          // left-assoc
unary       := ( "!" | "-" ) unary | postfix

postfix     := primary { postfix_op }
postfix_op  := "[" expr "]"                                  // index
             | "." IDENT                                     // field access
             | "." IDENT [ type_args ] "(" args ")"          // method call
             | "(" args ")"                                  // (only on a callable primary)

primary     := int_lit | float_lit | string_lit | bool_lit | none_lit
             | "(" expr ")"
             | array_literal | map_literal | class_literal
             | enum_member                                    // Type.Member
             | IDENT
             | FUNC_NAME [ type_args ] "(" args ")"           // free function / cast

args        := [ expr { "," expr } [ "," ] ]

enum_member := TYPE_NAME "." IDENT
field_access := expr "." IDENT

array_literal  := "[" [ expr { "," expr } [ "," ] ] "]"
map_literal    := "{" [ map_entry { "," map_entry } [ "," ] ] "}"
map_entry      := expr ":" expr                                // keys must be string literals
class_literal  := TYPE_NAME [ type_args ] "{" [ field_init { "," field_init } [ "," ] ] "}"
field_init     := IDENT ":" expr

cast          := ("int" | "float" | "string" | "bool") "(" expr ")"
```

`some(x)` is a plain call to the builtin `some` (no special grammar). Explicit
generic type arguments may follow a `FUNC_NAME`, `TYPE_NAME` (class literal), or
method name; when omitted, type arguments are inferred from the arguments.
At `IDENT "<"` in expression position the parser tentatively reads a
`type_args` list and backtracks to a comparison if that fails.

Operator semantics: `/` always yields `float` (`int/int` is `float`); `%` is
integer-only (truncated toward the dividend's sign); `+` on `string` is
concatenation; `==`/`!=` are structural for scalars/enums/classes/arrays/maps;
`<` `<=` `>` `>=` are numeric only; `&&`/`||` short-circuit.

---

## 8. Prompt body

The prompt block is not tokenized by the lexer. Its raw text is scanned by a
sub-parser honoring the escapes `\{`, `\}`, `\\`, then split into literal runs
and constructs:

```
prompt_body  := { text | interp | control | include }

interp       := "{{" expr "}}"                              // interpolation

control      := for_tag | if_tag
for_tag      := "{%" "for" IDENT "in" expr "%}" prompt_body "{%" "end" "%}"
if_tag       := "{%" "if" expr "%}" prompt_body
                  [ "{%" "else" "%}" prompt_body ]
                "{%" "end" "%}"

include      := "{{" "include" TEMPLATE_NAME "(" named_args ")" "}}"
named_args   := [ IDENT ":" expr { "," IDENT ":" expr } [ "," ] ]
```

Prompt text uses C-style backslash escapes: `\{` → `{`, `\}` → `}`, `\\` → `\`.
Any other backslash is a compile error; a lone `{` not starting `{{`/`{%` is
literal; a literal `}` must be written `\}`.

---

## 9. Whitespace rule (dedent + anchoring)

Before rendering, the `prompt` body is **dedented**: the common leading
whitespace over non-blank lines is stripped. Then, per line:

- a line that is purely a control tag (`{% for %}` / `{% if %}` / `{% else %}` /
  `{% end %}`) emits nothing and **drops** its newline;
- a line that is an own-line `{{ include … }}` is replaced by the included
  template's output spliced at the include's column and **consumes** its own
  newline (every template's body ends with one trailing `\n`, so the child's
  trailing newline is the separator);
- a content line emits its literal text verbatim (keeping its newline), with
  each `{{ expr }}` replaced in place.

A multi-line `{{ expr }}` value anchors its first line at the expression's
column and adds that column to each subsequent line's own relative indentation.
