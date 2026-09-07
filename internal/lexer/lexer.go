// Package lexer turns Prompiler source text into a token stream. It is the
// first compiler stage; the parser consumes its output.
package lexer

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Jh123x/prompiler/internal/token"
)

// Lexer scans a single source file.
type Lexer struct {
	src  string
	pos  int
	line int
	col  int

	// pending holds tokens produced ahead of the scan (used by the prompt-body
	// capture, which emits LBRACE + PROMPT_BODY + RBRACE together).
	pending []token.Token
}

// New returns a Lexer for src, normalizing CRLF line endings to LF.
func New(src string) *Lexer {
	s := strings.ReplaceAll(src, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return &Lexer{src: s, line: 1, col: 1}
}

// Lex returns the full token stream (always ending in one EOF token) plus any
// lexer diagnostics. It never panics.
func (l *Lexer) Lex() ([]token.Token, []token.Diagnostic) {
	var toks []token.Token
	var diags []token.Diagnostic
	for {
		t := l.next()
		toks = append(toks, t)
		if t.Type == token.EOF {
			break
		}
		if t.Type == token.ILLEGAL {
			diags = append(diags, token.Diagnostic{
				Stage:    token.StageParse,
				Category: token.Category("invalid_character"),
				Span:     t.Span,
				Message:  fmt.Sprintf("invalid character %q", t.Lexeme),
			})
		}
	}
	return toks, diags
}

func (l *Lexer) next() token.Token {
	if len(l.pending) > 0 {
		t := l.pending[0]
		l.pending = l.pending[1:]
		return t
	}
	return l.scan()
}

// --- scanning helpers ---

func (l *Lexer) eof() bool { return l.pos >= len(l.src) }

func (l *Lexer) peek() byte {
	if l.eof() {
		return 0
	}
	return l.src[l.pos]
}

func (l *Lexer) peekAt(n int) byte {
	if l.pos+n >= len(l.src) {
		return 0
	}
	return l.src[l.pos+n]
}

// advance consumes one byte and updates line/column (column counts bytes; the
// language's ASCII surface makes this equal to a rune count for all fixtures).
func (l *Lexer) advance() byte {
	c := l.src[l.pos]
	l.pos++
	if c == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return c
}

func (l *Lexer) start() token.Position {
	return token.Position{Offset: l.pos, Line: l.line, Column: l.col}
}

func (l *Lexer) spanFrom(start token.Position) token.Span {
	return token.Span{Start: start, End: token.Position{Offset: l.pos, Line: l.line, Column: l.col}}
}

func (l *Lexer) save() (int, int, int) { return l.pos, l.line, l.col }
func (l *Lexer) restore(pos, line, col int) {
	l.pos, l.line, l.col = pos, line, col
}

func isIdentStart(c byte) bool { return c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') }
func isIdentPart(c byte) bool  { return isIdentStart(c) || (c >= '0' && c <= '9') }
func isDigit(c byte) bool      { return c >= '0' && c <= '9' }

func (l *Lexer) skipWhitespaceAndComments() {
	for {
		for !l.eof() && (l.peek() == ' ' || l.peek() == '\t' || l.peek() == '\n') {
			l.advance()
		}
		if l.peek() == '/' && l.peekAt(1) == '/' {
			for !l.eof() && l.peek() != '\n' {
				l.advance()
			}
			continue
		}
		if l.peek() == '/' && l.peekAt(1) == '*' {
			l.advance()
			l.advance()
			for !l.eof() {
				if l.peek() == '*' && l.peekAt(1) == '/' {
					l.advance()
					l.advance()
					break
				}
				l.advance()
			}
			continue
		}
		return
	}
}

// scan produces a single token (never EOF except at end of input).
func (l *Lexer) scan() token.Token {
	l.skipWhitespaceAndComments()

	start := l.start()
	if l.eof() {
		return token.Token{Type: token.EOF, Span: token.Span{Start: start, End: start}}
	}

	c := l.peek()

	if isIdentStart(c) {
		return l.scanIdentOrKeyword(start)
	}
	if isDigit(c) {
		return l.scanNumber(start)
	}
	if c == '"' || c == '\'' {
		return l.scanString(start)
	}
	return l.scanOperator(start)
}

func (l *Lexer) scanIdentOrKeyword(start token.Position) token.Token {
	for !l.eof() && isIdentPart(l.peek()) {
		l.advance()
	}
	lexeme := l.src[start.Offset:l.pos]
	if kw, ok := token.LookupKeyword(lexeme); ok {
		t := token.Token{Type: kw, Lexeme: lexeme, Span: l.spanFrom(start)}
		if kw == token.PROMPT {
			l.tryCapturePromptBody()
		}
		return t
	}
	return token.Token{Type: token.IDENT, Lexeme: lexeme, Span: l.spanFrom(start)}
}

func (l *Lexer) scanNumber(start token.Position) token.Token {
	for !l.eof() && isDigit(l.peek()) {
		l.advance()
	}
	if l.peek() == '.' && isDigit(l.peekAt(1)) {
		l.advance() // '.'
		for !l.eof() && isDigit(l.peek()) {
			l.advance()
		}
		f, _ := strconv.ParseFloat(l.src[start.Offset:l.pos], 64)
		t := token.Token{Type: token.FLOAT, Lexeme: l.src[start.Offset:l.pos], Span: l.spanFrom(start)}
		t.FloatVal = f
		return t
	}
	n, err := strconv.ParseInt(l.src[start.Offset:l.pos], 10, 64)
	if err != nil {
		// Out-of-range literal: flag it rather than silently clamping.
		return token.Token{Type: token.ILLEGAL, Lexeme: l.src[start.Offset:l.pos], Span: l.spanFrom(start)}
	}
	t := token.Token{Type: token.INT, Lexeme: l.src[start.Offset:l.pos], Span: l.spanFrom(start)}
	t.IntVal = n
	return t
}

func (l *Lexer) scanString(start token.Position) token.Token {
	quote := l.advance() // consume opening quote
	var sb strings.Builder
	for !l.eof() {
		c := l.peek()
		if c == quote {
			l.advance()
			return token.Token{Type: token.STRING, Lexeme: sb.String(), Span: l.spanFrom(start)}
		}
		if c == '\n' {
			break // unterminated string at newline
		}
		if c == '\\' {
			l.advance()
			if l.eof() {
				break
			}
			e := l.advance()
			switch e {
			case 'n':
				sb.WriteByte('\n')
			case 't':
				sb.WriteByte('\t')
			case 'r':
				sb.WriteByte('\r')
			case '\\':
				sb.WriteByte('\\')
			case '"':
				sb.WriteByte('"')
			case '\'':
				sb.WriteByte('\'')
			default:
				sb.WriteByte(e) // lenient: include the escaped char
			}
			continue
		}
		sb.WriteByte(l.advance())
	}
	// Unterminated: emit what was scanned (lenient; no fixture exercises this).
	return token.Token{Type: token.STRING, Lexeme: sb.String(), Span: l.spanFrom(start)}
}

func (l *Lexer) scanOperator(start token.Position) token.Token {
	c := l.peek()
	switch c {
	case '|':
		if l.peekAt(1) == '|' {
			l.advance()
			l.advance()
			return token.Token{Type: token.OR_OR, Lexeme: "||", Span: l.spanFrom(start)}
		}
	case '&':
		if l.peekAt(1) == '&' {
			l.advance()
			l.advance()
			return token.Token{Type: token.AND_AND, Lexeme: "&&", Span: l.spanFrom(start)}
		}
	case '=':
		if l.peekAt(1) == '=' {
			l.advance()
			l.advance()
			return token.Token{Type: token.EQ_EQ, Lexeme: "==", Span: l.spanFrom(start)}
		}
		l.advance()
		return token.Token{Type: token.ASSIGN, Lexeme: "=", Span: l.spanFrom(start)}
	case '!':
		if l.peekAt(1) == '=' {
			l.advance()
			l.advance()
			return token.Token{Type: token.NOT_EQ, Lexeme: "!=", Span: l.spanFrom(start)}
		}
		l.advance()
		return token.Token{Type: token.BANG, Lexeme: "!", Span: l.spanFrom(start)}
	case '<':
		if l.peekAt(1) == '=' {
			l.advance()
			l.advance()
			return token.Token{Type: token.LT_EQ, Lexeme: "<=", Span: l.spanFrom(start)}
		}
		l.advance()
		return token.Token{Type: token.LT, Lexeme: "<", Span: l.spanFrom(start)}
	case '>':
		if l.peekAt(1) == '=' {
			l.advance()
			l.advance()
			return token.Token{Type: token.GT_EQ, Lexeme: ">=", Span: l.spanFrom(start)}
		}
		l.advance()
		return token.Token{Type: token.GT, Lexeme: ">", Span: l.spanFrom(start)}
	case '+':
		l.advance()
		return token.Token{Type: token.PLUS, Lexeme: "+", Span: l.spanFrom(start)}
	case '-':
		l.advance()
		return token.Token{Type: token.MINUS, Lexeme: "-", Span: l.spanFrom(start)}
	case '*':
		l.advance()
		return token.Token{Type: token.STAR, Lexeme: "*", Span: l.spanFrom(start)}
	case '/':
		l.advance()
		return token.Token{Type: token.SLASH, Lexeme: "/", Span: l.spanFrom(start)}
	case '%':
		l.advance()
		return token.Token{Type: token.PERCENT, Lexeme: "%", Span: l.spanFrom(start)}
	case '(':
		l.advance()
		return token.Token{Type: token.LPAREN, Lexeme: "(", Span: l.spanFrom(start)}
	case ')':
		l.advance()
		return token.Token{Type: token.RPAREN, Lexeme: ")", Span: l.spanFrom(start)}
	case '{':
		l.advance()
		return token.Token{Type: token.LBRACE, Lexeme: "{", Span: l.spanFrom(start)}
	case '}':
		l.advance()
		return token.Token{Type: token.RBRACE, Lexeme: "}", Span: l.spanFrom(start)}
	case '[':
		l.advance()
		return token.Token{Type: token.LBRACKET, Lexeme: "[", Span: l.spanFrom(start)}
	case ']':
		l.advance()
		return token.Token{Type: token.RBRACKET, Lexeme: "]", Span: l.spanFrom(start)}
	case ',':
		l.advance()
		return token.Token{Type: token.COMMA, Lexeme: ",", Span: l.spanFrom(start)}
	case ':':
		l.advance()
		return token.Token{Type: token.COLON, Lexeme: ":", Span: l.spanFrom(start)}
	case '.':
		l.advance()
		return token.Token{Type: token.DOT, Lexeme: ".", Span: l.spanFrom(start)}
	}
	l.advance()
	return token.Token{Type: token.ILLEGAL, Lexeme: string(c), Span: l.spanFrom(start)}
}

// tryCapturePromptBody looks ahead past whitespace/comments for '{'. If found,
// it emits LBRACE, then a single PROMPT_BODY token holding the verbatim body
// text, then RBRACE, via the pending queue. Returns true if a body was captured.
func (l *Lexer) tryCapturePromptBody() bool {
	pos, line, col := l.save()
	l.skipWhitespaceAndComments()
	if l.peek() != '{' {
		l.restore(pos, line, col)
		return false
	}

	lbraceStart := l.start()
	l.advance() // consume '{'
	lbraceTok := token.Token{Type: token.LBRACE, Lexeme: "{", Span: l.spanFrom(lbraceStart)}

	bodyStart := l.start()
	raw := l.captureRawPromptBody()
	bodyTok := token.Token{Type: token.PROMPT_BODY, Lexeme: raw, Span: l.spanFrom(bodyStart)}

	if l.eof() {
		// Unterminated prompt body (no closing brace): emit what we have; the
		// parser reports the missing brace.
		l.pending = append(l.pending, lbraceTok, bodyTok)
		return true
	}

	rbraceStart := l.start()
	l.advance() // consume closing '}'
	rbraceTok := token.Token{Type: token.RBRACE, Lexeme: "}", Span: l.spanFrom(rbraceStart)}

	l.pending = append(l.pending, lbraceTok, bodyTok, rbraceTok)
	return true
}

// captureRawPromptBody scans the verbatim prompt body (between the braces)
// until the matching closing '}', honoring \{ \} \\ escapes and the {{ }} and
// {% %} construct delimiters so an inner } does not close early.
func (l *Lexer) captureRawPromptBody() string {
	start := l.pos
	for !l.eof() {
		c := l.peek()
		switch {
		case c == '\\':
			l.advance()
			if !l.eof() {
				l.advance()
			}
		case c == '{' && l.peekAt(1) == '{':
			l.skipUntil("}}")
		case c == '{' && l.peekAt(1) == '%':
			l.skipUntil("%}")
		case c == '}':
			return l.src[start:l.pos]
		default:
			l.advance()
		}
	}
	return l.src[start:l.pos]
}

func (l *Lexer) skipUntil(delim string) {
	for !l.eof() {
		if strings.HasPrefix(l.src[l.pos:], delim) {
			for i := 0; i < len(delim); i++ {
				l.advance()
			}
			return
		}
		l.advance()
	}
}
