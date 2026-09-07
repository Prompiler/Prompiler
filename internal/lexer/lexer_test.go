package lexer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	t.Run("recognizes every operator and punctuation token", func(t *testing.T) {
		src := "|| && == != <= >= < > + - * / % ! = ( ) { } [ ] , : ."
		want := []token.TokenType{
			token.OR_OR, token.AND_AND, token.EQ_EQ, token.NOT_EQ, token.LT_EQ, token.GT_EQ,
			token.LT, token.GT, token.PLUS, token.MINUS, token.STAR, token.SLASH, token.PERCENT,
			token.BANG, token.ASSIGN, token.LPAREN, token.RPAREN, token.LBRACE, token.RBRACE,
			token.LBRACKET, token.RBRACKET, token.COMMA, token.COLON, token.DOT,
		}
		toks, diags := lex(t, src)
		require.Empty(t, diags)
		assert.Equal(t, want, typesOf(toks))
	})
}

func TestNumbers(t *testing.T) {
	t.Run("integer literal", func(t *testing.T) {
		toks, _ := lex(t, "42")
		require.Len(t, toks, 1)
		assert.Equal(t, token.INT, toks[0].Type)
		assert.Equal(t, int64(42), toks[0].IntVal)
	})

	t.Run("float literal", func(t *testing.T) {
		toks, _ := lex(t, "3.14")
		require.Len(t, toks, 1)
		assert.Equal(t, token.FLOAT, toks[0].Type)
		assert.Equal(t, 3.14, toks[0].FloatVal)
	})

	t.Run("trailing dot lexes as int then dot", func(t *testing.T) {
		toks, _ := lex(t, "1.")
		require.Len(t, toks, 2)
		assert.Equal(t, token.INT, toks[0].Type)
		assert.Equal(t, token.DOT, toks[1].Type)
	})

	t.Run("leading dot lexes as dot then int", func(t *testing.T) {
		toks, _ := lex(t, ".5")
		require.Len(t, toks, 2)
		assert.Equal(t, token.DOT, toks[0].Type)
		assert.Equal(t, token.INT, toks[1].Type)
		assert.Equal(t, int64(5), toks[1].IntVal)
	})

	t.Run("exponent lexes as int then identifier", func(t *testing.T) {
		toks, _ := lex(t, "1e3")
		require.Len(t, toks, 2)
		assert.Equal(t, token.INT, toks[0].Type)
		assert.Equal(t, token.IDENT, toks[1].Type)
		assert.Equal(t, "e3", toks[1].Lexeme)
	})
}

func TestStrings(t *testing.T) {
	t.Run("double quoted", func(t *testing.T) {
		toks, _ := lex(t, `"hello"`)
		require.Len(t, toks, 1)
		assert.Equal(t, token.STRING, toks[0].Type)
		assert.Equal(t, "hello", toks[0].Lexeme)
	})

	t.Run("single quoted", func(t *testing.T) {
		toks, _ := lex(t, `'world'`)
		require.Len(t, toks, 1)
		assert.Equal(t, token.STRING, toks[0].Type)
		assert.Equal(t, "world", toks[0].Lexeme)
	})

	t.Run("newline escape", func(t *testing.T) {
		toks, _ := lex(t, `"a\nb"`)
		require.Len(t, toks, 1)
		assert.Equal(t, "a\nb", toks[0].Lexeme)
	})

	t.Run("quote escape", func(t *testing.T) {
		toks, _ := lex(t, `"quote\"x"`)
		require.Len(t, toks, 1)
		assert.Equal(t, `quote"x`, toks[0].Lexeme)
	})
}

func TestComments(t *testing.T) {
	t.Run("line comment is skipped", func(t *testing.T) {
		toks, _ := lex(t, "a // comment\nb")
		require.Len(t, toks, 2)
		assert.Equal(t, "a", toks[0].Lexeme)
		assert.Equal(t, "b", toks[1].Lexeme)
	})

	t.Run("block comment is skipped", func(t *testing.T) {
		toks, _ := lex(t, "a /* comment */ b")
		require.Len(t, toks, 2)
		assert.Equal(t, "a", toks[0].Lexeme)
		assert.Equal(t, "b", toks[1].Lexeme)
	})
}

func TestKeywordsAndIdentifiers(t *testing.T) {
	t.Run("reserved words are keywords", func(t *testing.T) {
		src := "enum class interface func template import var if else for in return this true false none string int float bool map array Optional"
		keywords := []token.TokenType{
			token.ENUM, token.CLASS, token.INTERFACE, token.FUNC, token.TEMPLATE, token.IMPORT,
			token.VAR, token.IF, token.ELSE, token.FOR, token.IN, token.RETURN, token.THIS,
			token.TRUE, token.FALSE, token.NONE, token.STRING_KW, token.INT_KW, token.FLOAT_KW,
			token.BOOL_KW, token.MAP, token.ARRAY, token.OPTIONAL,
		}
		toks, _ := lex(t, src)
		require.Len(t, toks, len(keywords))
		assert.Equal(t, keywords, typesOf(toks))
	})

	t.Run("non-reserved name is an identifier", func(t *testing.T) {
		toks, _ := lex(t, "Difficulty")
		require.Len(t, toks, 1)
		assert.Equal(t, token.IDENT, toks[0].Type)
		assert.Equal(t, "Difficulty", toks[0].Lexeme)
	})
}

func TestPromptBodyCapture(t *testing.T) {
	t.Run("captures the verbatim prompt body", func(t *testing.T) {
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
		require.Empty(t, diags)

		var body *token.Token
		for i := range toks {
			if toks[i].Type == token.PROMPT_BODY {
				body = &toks[i]
				break
			}
		}
		require.NotNil(t, body, "expected a PROMPT_BODY token")
		assert.Contains(t, body.Lexeme, "Hello {{ name }}!")
		assert.Contains(t, body.Lexeme, `{% if x %}...{% end %}`)
		assert.Contains(t, body.Lexeme, `{{ include Header(company: company) }}`)
		assert.Contains(t, body.Lexeme, `\{ literal \}`)
		assert.NotEqual(t, "}", body.Lexeme[len(body.Lexeme)-1:], "body must exclude the closing brace")
	})
}

func TestPositions(t *testing.T) {
	t.Run("tracks line and column", func(t *testing.T) {
		toks, _ := lex(t, "abc\ndef")
		require.Len(t, toks, 2)
		assert.Equal(t, 1, toks[0].Span.Start.Line)
		assert.Equal(t, 1, toks[0].Span.Start.Column)
		assert.Equal(t, 2, toks[1].Span.Start.Line)
		assert.Equal(t, 1, toks[1].Span.Start.Column)
	})
}

func TestCRLFNormalization(t *testing.T) {
	t.Run("normalizes CRLF to LF", func(t *testing.T) {
		toks, _ := lex(t, "a\r\nb\r\n")
		require.Len(t, toks, 2)
		assert.Equal(t, 2, toks[1].Span.Start.Line)
	})
}

func TestAllExamplesLexCleanly(t *testing.T) {
	t.Run("every example lexes with zero illegal tokens", func(t *testing.T) {
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
		require.NoError(t, err)
		require.NotEmpty(t, files)

		for _, f := range files {
			data, err := os.ReadFile(f)
			require.NoError(t, err)
			toks, diags := New(string(data)).Lex()
			for _, tok := range toks {
				assert.NotEqual(t, token.ILLEGAL, tok.Type, "%s: illegal token %q", f, tok.Lexeme)
			}
			assert.Empty(t, diags, "%s: lexer diagnostics", f)
		}
	})
}
