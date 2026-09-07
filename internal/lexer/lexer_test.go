package lexer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jh123x/prompiler/internal/token"
)

// lex returns the token stream (without the trailing EOF) and diagnostics.
func lex(t *testing.T, src string) ([]token.Token, []token.Diagnostic) {
	t.Helper()
	toks, diags := New(src).Lex()
	if len(toks) > 0 && toks[len(toks)-1].Type == token.EOF {
		toks = toks[:len(toks)-1]
	}
	return toks, diags
}

func typesOf(toks []token.Token) []token.TokenType {
	out := make([]token.TokenType, len(toks))
	for i, t := range toks {
		out[i] = t.Type
	}
	return out
}

func TestOperators(t *testing.T) {
	src := "|| && == != <= >= < > + - * / % ! = ( ) { } [ ] , : ."
	want := []token.TokenType{
		token.OR_OR, token.AND_AND, token.EQ_EQ, token.NOT_EQ, token.LT_EQ, token.GT_EQ,
		token.LT, token.GT, token.PLUS, token.MINUS, token.STAR, token.SLASH, token.PERCENT,
		token.BANG, token.ASSIGN, token.LPAREN, token.RPAREN, token.LBRACE, token.RBRACE,
		token.LBRACKET, token.RBRACKET, token.COMMA, token.COLON, token.DOT,
	}
	toks, diags := lex(t, src)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	got := typesOf(toks)
	if len(got) != len(want) {
		t.Fatalf("got %d tokens, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("token %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestNumbers(t *testing.T) {
	toks, _ := lex(t, "42 3.14 1. .5 1e3 0")
	if len(toks) != 9 {
		t.Fatalf("got %d tokens: %v", len(toks), typesOf(toks))
	}
	if toks[0].Type != token.INT || toks[0].IntVal != 42 {
		t.Errorf("42 -> %v %v", toks[0].Type, toks[0].IntVal)
	}
	if toks[1].Type != token.FLOAT || toks[1].FloatVal != 3.14 {
		t.Errorf("3.14 -> %v %v", toks[1].Type, toks[1].FloatVal)
	}
	// "1." lexes as INT(1) DOT
	if toks[2].Type != token.INT || toks[2].IntVal != 1 || toks[3].Type != token.DOT {
		t.Errorf("1. -> %v %v %v", toks[2].Type, toks[2].IntVal, toks[3].Type)
	}
	// ".5" lexes as DOT INT(5)
	if toks[4].Type != token.DOT || toks[5].Type != token.INT || toks[5].IntVal != 5 {
		t.Errorf(".5 -> %v %v", toks[4].Type, toks[5].Type)
	}
	// "1e3" lexes as INT(1) IDENT(e3)
	if toks[6].Type != token.INT || toks[7].Type != token.IDENT || toks[7].Lexeme != "e3" {
		t.Errorf("1e3 -> %v %v", toks[6].Type, toks[7].Type)
	}
}

func TestStrings(t *testing.T) {
	toks, _ := lex(t, `"hello" 'world' "a\nb" "quote\"x"`)
	if toks[0].Type != token.STRING || toks[0].Lexeme != "hello" {
		t.Errorf("double: %v %q", toks[0].Type, toks[0].Lexeme)
	}
	if toks[1].Type != token.STRING || toks[1].Lexeme != "world" {
		t.Errorf("single: %v %q", toks[1].Type, toks[1].Lexeme)
	}
	if toks[2].Lexeme != "a\nb" {
		t.Errorf("newline escape: %q", toks[2].Lexeme)
	}
	if toks[3].Lexeme != `quote"x` {
		t.Errorf("quote escape: %q", toks[3].Lexeme)
	}
}

func TestComments(t *testing.T) {
	toks, _ := lex(t, "a // line comment\nb /* block\ncomment */ c")
	got := typesOf(toks)
	want := []token.TokenType{token.IDENT, token.IDENT, token.IDENT}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if toks[0].Lexeme != "a" || toks[1].Lexeme != "b" || toks[2].Lexeme != "c" {
		t.Errorf("lexemes: %v %v %v", toks[0].Lexeme, toks[1].Lexeme, toks[2].Lexeme)
	}
}

func TestKeywordsAndIdentifiers(t *testing.T) {
	toks, _ := lex(t, "enum class interface func template import var if else for in return this true false none string int float bool map array Optional Difficulty")
	keywords := []token.TokenType{
		token.ENUM, token.CLASS, token.INTERFACE, token.FUNC, token.TEMPLATE, token.IMPORT,
		token.VAR, token.IF, token.ELSE, token.FOR, token.IN, token.RETURN, token.THIS,
		token.TRUE, token.FALSE, token.NONE, token.STRING_KW, token.INT_KW, token.FLOAT_KW,
		token.BOOL_KW, token.MAP, token.ARRAY, token.OPTIONAL,
	}
	// The last identifier "Difficulty" is a plain IDENT (not reserved).
	got := typesOf(toks)
	if len(got) != len(keywords)+1 {
		t.Fatalf("got %d tokens, want %d", len(got), len(keywords)+1)
	}
	for i, kw := range keywords {
		if got[i] != kw {
			t.Errorf("token %d: got %v, want %v", i, got[i], kw)
		}
	}
	if got[len(keywords)] != token.IDENT {
		t.Errorf("Difficulty should be IDENT, got %v", got[len(keywords)])
	}
}

func TestPromptBodyCapture(t *testing.T) {
	src := `template Greet {
  variables { name: string }
  prompt {
    Hello {{ name }}!
    {% if x %}...{% end %}
    {{ include Header(company: company) }}
    \{ literal \}
  }
}`
	toks, diags := lex(t, src)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	// Find the PROMPT_BODY token.
	var body *token.Token
	for i := range toks {
		if toks[i].Type == token.PROMPT_BODY {
			body = &toks[i]
			break
		}
	}
	if body == nil {
		t.Fatal("no PROMPT_BODY token found")
	}
	// The raw body must be captured verbatim (escapes and constructs intact) and
	// must NOT include the closing brace.
	if !strings.Contains(body.Lexeme, "Hello {{ name }}!") {
		t.Errorf("body missing interpolation text: %q", body.Lexeme)
	}
	if !strings.Contains(body.Lexeme, `{% if x %}...{% end %}`) {
		t.Errorf("body missing control tag: %q", body.Lexeme)
	}
	if !strings.Contains(body.Lexeme, `{{ include Header(company: company) }}`) {
		t.Errorf("body missing include: %q", body.Lexeme)
	}
	if !strings.Contains(body.Lexeme, `\{ literal \}`) {
		t.Errorf("body missing escaped braces: %q", body.Lexeme)
	}
	if strings.HasSuffix(body.Lexeme, "}") {
		t.Errorf("body should exclude the closing brace: %q", body.Lexeme)
	}
}

func TestPositions(t *testing.T) {
	toks, _ := lex(t, "abc\ndef")
	// "def" starts at line 2, column 1, offset 4.
	if toks[1].Span.Start.Line != 2 || toks[1].Span.Start.Column != 1 {
		t.Errorf("def position: %+v", toks[1].Span.Start)
	}
	if toks[0].Span.Start.Line != 1 || toks[0].Span.Start.Column != 1 {
		t.Errorf("abc position: %+v", toks[0].Span.Start)
	}
}

func TestCRLFNormalization(t *testing.T) {
	toks, _ := lex(t, "a\r\nb\r\n")
	if len(toks) != 2 {
		t.Fatalf("got %d tokens", len(toks))
	}
	if toks[1].Span.Start.Line != 2 {
		t.Errorf("b should be on line 2, got %+v", toks[1].Span.Start)
	}
}

func TestAllExamplesLexCleanly(t *testing.T) {
	var files []string
	err := filepath.Walk("../../examples", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".ppl") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk examples: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no example files found")
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		toks, diags := New(string(data)).Lex()
		for _, tok := range toks {
			if tok.Type == token.ILLEGAL {
				t.Errorf("%s: illegal token %q at %+v", f, tok.Lexeme, tok.Span.Start)
			}
		}
		if len(diags) != 0 {
			t.Errorf("%s: %d diagnostics: %v", f, len(diags), diags)
		}
	}
}
