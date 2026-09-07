// Package parser implements a recoverable recursive-descent parser for
// Prompiler, producing the immutable AST. It consumes the lexer's token stream
// (injected) and collects all syntax errors rather than aborting on the first.
package parser

import (
	"fmt"
	"strings"

	"github.com/Jh123x/prompiler/internal/ast"
	"github.com/Jh123x/prompiler/internal/lexer"
	"github.com/Jh123x/prompiler/internal/token"
)

// Parser is a recursive-descent parser over a token stream.
type Parser struct {
	toks  []token.Token
	pos   int
	diags []token.Diagnostic
}

// New returns a Parser over the given token stream (already lexed).
func New(toks []token.Token) *Parser {
	return &Parser{toks: toks}
}

// ParseFile parses a full source file, never aborting on error. It returns the
// AST (with error nodes where recovery occurred) plus every collected
// diagnostic.
func (p *Parser) ParseFile() (*ast.File, []token.Diagnostic) {
	file := &ast.File{}
	for !p.atEOF() {
		if d := p.parseDecl(); d != nil {
			file.Decls = append(file.Decls, d)
		} else {
			p.syncDecl()
		}
	}
	return file, p.diags
}

// --- token helpers ---

func (p *Parser) cur() token.Token {
	if p.pos >= len(p.toks) {
		return token.Token{Type: token.EOF}
	}
	return p.toks[p.pos]
}

func (p *Parser) advance() token.Token {
	t := p.cur()
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return t
}

func (p *Parser) atEOF() bool { return p.cur().Type == token.EOF }

func (p *Parser) match(t token.TokenType) bool { return p.cur().Type == t }

func (p *Parser) expect(t token.TokenType) token.Token {
	if p.cur().Type == t {
		return p.advance()
	}
	p.error(fmt.Sprintf("expected %v, got %q", t, p.cur().Lexeme), p.cur())
	return p.cur()
}

func (p *Parser) error(msg string, at token.Token) {
	p.diags = append(p.diags, token.Diagnostic{
		Stage:    token.StageParse,
		Category: token.Category("syntax_error"),
		Span:     at.Span,
		Message:  msg,
	})
}

func (p *Parser) span(start token.Position) token.Span {
	return token.Span{Start: start, End: p.cur().Span.Start}
}

// syncDecl advances past an unparsable declaration to the next declaration
// keyword or EOF.
func (p *Parser) syncDecl() {
	for !p.atEOF() {
		switch p.cur().Type {
		case token.IMPORT, token.ENUM, token.CLASS, token.INTERFACE, token.FUNC, token.TEMPLATE:
			return
		}
		p.advance()
	}
}

// syncStmt advances past an unparsable statement to a statement boundary.
func (p *Parser) syncStmt() {
	for !p.atEOF() {
		switch p.cur().Type {
		case token.VAR, token.IF, token.FOR, token.RETURN, token.RBRACE:
			return
		}
		p.advance()
	}
}

// --- declarations ---

func (p *Parser) parseDecl() ast.Decl {
	switch p.cur().Type {
	case token.IMPORT:
		return p.parseImport()
	case token.ENUM:
		return p.parseEnum()
	case token.CLASS:
		return p.parseClass()
	case token.INTERFACE:
		return p.parseInterface()
	case token.FUNC:
		return p.parseFunc()
	case token.TEMPLATE:
		return p.parseTemplate()
	default:
		p.error(fmt.Sprintf("expected a declaration, got %q", p.cur().Lexeme), p.cur())
		return nil
	}
}

func (p *Parser) parseImport() ast.Decl {
	start := p.cur().Span.Start
	p.advance() // import
	path := p.expect(token.STRING)
	return &ast.ImportDecl{Base: ast.Base{S: p.span(start)}, Path: path.Lexeme}
}

func (p *Parser) parseEnum() ast.Decl {
	start := p.cur().Span.Start
	p.advance() // enum
	name := p.expect(token.IDENT)
	p.expect(token.LBRACE)
	var members []string
	for !p.atEOF() && p.cur().Type != token.RBRACE {
		m := p.expect(token.IDENT)
		members = append(members, m.Lexeme)
		if p.match(token.COMMA) {
			p.advance()
			if p.match(token.RBRACE) {
				break // trailing comma
			}
			continue
		}
		break
	}
	p.expect(token.RBRACE)
	return &ast.EnumDecl{Base: ast.Base{S: p.span(start)}, Name: name.Lexeme, Members: members}
}

func (p *Parser) parseClass() ast.Decl {
	start := p.cur().Span.Start
	p.advance() // class
	name := p.expect(token.IDENT)
	typeParams := p.parseTypeParams()
	p.expect(token.LBRACE)
	var fields []ast.Field
	var methods []ast.MethodDecl
	for !p.atEOF() && p.cur().Type != token.RBRACE {
		if p.match(token.FUNC) {
			methods = append(methods, p.parseMethod(true))
		} else {
			fields = append(fields, p.parseField())
		}
	}
	p.expect(token.RBRACE)
	return &ast.ClassDecl{Base: ast.Base{S: p.span(start)}, Name: name.Lexeme, TypeParams: typeParams, Fields: fields, Methods: methods}
}

func (p *Parser) parseInterface() ast.Decl {
	start := p.cur().Span.Start
	p.advance() // interface
	name := p.expect(token.IDENT)
	typeParams := p.parseTypeParams()
	p.expect(token.LBRACE)
	var fields []ast.Field
	var methods []ast.MethodDecl
	for !p.atEOF() && p.cur().Type != token.RBRACE {
		if p.match(token.FUNC) {
			methods = append(methods, p.parseMethod(false))
		} else {
			fields = append(fields, p.parseField())
		}
	}
	p.expect(token.RBRACE)
	return &ast.InterfaceDecl{Base: ast.Base{S: p.span(start)}, Name: name.Lexeme, TypeParams: typeParams, Fields: fields, Methods: methods}
}

func (p *Parser) parseField() ast.Field {
	name := p.expect(token.IDENT)
	p.expect(token.COLON)
	t := p.parseTypeRef()
	return ast.Field{Name: name.Lexeme, Type: t}
}

func (p *Parser) parseMethod(hasBody bool) ast.MethodDecl {
	start := p.cur().Span.Start
	p.advance() // func
	name := p.expect(token.IDENT)
	typeParams := p.parseTypeParams()
	params := p.parseParams()
	p.expect(token.COLON)
	ret := p.parseTypeRef()
	m := ast.MethodDecl{Base: ast.Base{S: p.span(start)}, Name: name.Lexeme, TypeParams: typeParams, Params: params, Return: ret}
	if hasBody {
		m.Body = p.parseBlock()
	}
	return m
}

func (p *Parser) parseParams() []ast.Param {
	p.expect(token.LPAREN)
	var params []ast.Param
	if p.cur().Type != token.RPAREN {
		params = append(params, p.parseParam())
		for p.match(token.COMMA) {
			p.advance()
			if p.match(token.RPAREN) {
				break // trailing comma
			}
			params = append(params, p.parseParam())
		}
	}
	p.expect(token.RPAREN)
	return params
}

func (p *Parser) parseParam() ast.Param {
	name := p.expect(token.IDENT)
	p.expect(token.COLON)
	t := p.parseTypeRef()
	return ast.Param{Name: name.Lexeme, Type: t}
}

func (p *Parser) parseFunc() ast.Decl {
	start := p.cur().Span.Start
	p.advance() // func
	name := p.expect(token.IDENT)
	typeParams := p.parseTypeParams()
	params := p.parseParams()
	p.expect(token.COLON)
	ret := p.parseTypeRef()
	body := p.parseBlock()
	return &ast.FuncDecl{Base: ast.Base{S: p.span(start)}, Name: name.Lexeme, TypeParams: typeParams, Params: params, Return: ret, Body: body}
}

func (p *Parser) parseTemplate() ast.Decl {
	start := p.cur().Span.Start
	p.advance() // template
	name := p.expect(token.IDENT)
	p.expect(token.LBRACE)
	decl := &ast.TemplateDecl{Base: ast.Base{S: p.span(start)}, Name: name.Lexeme}
	if p.cur().Type == token.IDENT && p.cur().Lexeme == "description" {
		p.advance()
		p.expect(token.COLON)
		s := p.expect(token.STRING)
		decl.Description = s.Lexeme
	}
	if p.match(token.VARIABLES) {
		p.advance()
		p.expect(token.LBRACE)
		for !p.atEOF() && p.cur().Type != token.RBRACE {
			decl.Variables = append(decl.Variables, p.parseVarDecl())
		}
		p.expect(token.RBRACE)
	}
	if p.match(token.PROMPT) {
		p.advance()
		p.expect(token.LBRACE)
		bodyTok := p.expect(token.PROMPT_BODY)
		p.expect(token.RBRACE)
		decl.Prompt = p.parsePromptBody(bodyTok.Lexeme, bodyTok.Span)
	}
	p.expect(token.RBRACE)
	return decl
}

func (p *Parser) parseVarDecl() ast.VarDecl {
	start := p.cur().Span.Start
	name := p.expect(token.IDENT)
	p.expect(token.COLON)
	t := p.parseTypeRef()
	decl := ast.VarDecl{Base: ast.Base{S: p.span(start)}, Name: name.Lexeme, Type: t}
	if p.match(token.ASSIGN) {
		p.advance()
		decl.Default = p.parseExpr()
		decl.HasDefault = true
	}
	return decl
}

// --- types ---

func (p *Parser) parseTypeParams() []ast.TypeParam {
	if p.cur().Type != token.LT {
		return nil
	}
	p.advance() // <
	var params []ast.TypeParam
	params = append(params, p.parseTypeParam())
	for p.match(token.COMMA) {
		p.advance()
		if p.match(token.GT) {
			break // trailing comma
		}
		params = append(params, p.parseTypeParam())
	}
	p.expect(token.GT)
	return params
}

func (p *Parser) parseTypeParam() ast.TypeParam {
	name := p.expect(token.IDENT)
	tp := ast.TypeParam{Name: name.Lexeme}
	if p.match(token.COLON) {
		p.advance()
		tp.Bound = p.parseTypeRef()
	}
	return tp
}

func (p *Parser) parseTypeRef() ast.TypeRef {
	t := p.parseTypeRefPrimary()
	for p.match(token.LBRACKET) && p.toks[p.pos+1].Type == token.RBRACKET {
		p.advance() // [
		p.advance() // ]
		t = &ast.ArrayType{Base: ast.Base{S: p.span(t.Span().Start)}, Elem: t}
	}
	return t
}

func (p *Parser) parseTypeRefPrimary() ast.TypeRef {
	tok := p.cur()
	start := tok.Span.Start
	switch tok.Type {
	case token.STRING_KW, token.INT_KW, token.FLOAT_KW, token.BOOL_KW:
		p.advance()
		return &ast.NamedType{Base: ast.Base{S: p.span(start)}, Name: tok.Lexeme}
	case token.MAP:
		p.advance()
		p.expect(token.LT)
		p.expect(token.STRING_KW)
		p.expect(token.COMMA)
		v := p.parseTypeRef()
		p.expect(token.GT)
		return &ast.MapType{Base: ast.Base{S: p.span(start)}, Value: v}
	case token.OPTIONAL:
		p.advance()
		p.expect(token.LT)
		e := p.parseTypeRef()
		p.expect(token.GT)
		return &ast.OptionalType{Base: ast.Base{S: p.span(start)}, Elem: e}
	case token.IDENT:
		p.advance()
		var args []ast.TypeRef
		if p.match(token.LT) {
			args = p.parseTypeArgs()
		}
		return &ast.NamedType{Base: ast.Base{S: p.span(start)}, Name: tok.Lexeme, TypeArgs: args}
	default:
		p.error(fmt.Sprintf("expected a type, got %q", tok.Lexeme), tok)
		p.advance()
		return &ast.NamedType{Base: ast.Base{S: p.span(start)}, Name: "?"}
	}
}

func (p *Parser) parseTypeArgs() []ast.TypeRef {
	p.expect(token.LT)
	var args []ast.TypeRef
	args = append(args, p.parseTypeRef())
	for p.match(token.COMMA) {
		p.advance()
		if p.match(token.GT) {
			break // trailing comma
		}
		args = append(args, p.parseTypeRef())
	}
	p.expect(token.GT)
	return args
}

// tryParseTypeArgs tentatively parses an explicit `<T, U>` type-argument list in
// expression position, committing only if it is followed by `(`. Returns the
// args and true on success, restoring the position otherwise (so `<` is treated
// as a comparison).
func (p *Parser) tryParseTypeArgs() ([]ast.TypeRef, bool) {
	if p.cur().Type != token.LT {
		return nil, false
	}
	save := p.pos
	p.advance() // <
	var args []ast.TypeRef
	args = append(args, p.tryParseTypeRef())
	if args[0] == nil {
		p.pos = save
		return nil, false
	}
	for p.match(token.COMMA) {
		p.advance()
		if p.match(token.GT) {
			break
		}
		args = append(args, p.tryParseTypeRef())
		if args[len(args)-1] == nil {
			p.pos = save
			return nil, false
		}
	}
	if p.cur().Type != token.GT {
		p.pos = save
		return nil, false
	}
	p.advance() // >
	if p.cur().Type != token.LPAREN {
		p.pos = save
		return nil, false
	}
	return args, true
}

// tryParseTypeRef parses a type reference non-destructively, returning nil on
// failure (without emitting diagnostics).
func (p *Parser) tryParseTypeRef() ast.TypeRef {
	switch p.cur().Type {
	case token.STRING_KW, token.INT_KW, token.FLOAT_KW, token.BOOL_KW, token.IDENT, token.OPTIONAL, token.MAP:
		return p.parseTypeRefPrimary()
	default:
		return nil
	}
}

// --- statements ---

func (p *Parser) parseBlock() []ast.Stmt {
	p.expect(token.LBRACE)
	var stmts []ast.Stmt
	for !p.atEOF() && p.cur().Type != token.RBRACE {
		if s := p.parseStmt(); s != nil {
			stmts = append(stmts, s)
		} else {
			p.syncStmt()
		}
	}
	p.expect(token.RBRACE)
	return stmts
}

func (p *Parser) parseStmt() ast.Stmt {
	switch p.cur().Type {
	case token.VAR:
		return p.parseVarStmt()
	case token.IF:
		return p.parseIfStmt()
	case token.FOR:
		return p.parseForStmt()
	case token.RETURN:
		return p.parseReturnStmt()
	case token.IDENT, token.THIS:
		return p.parseAssignStmt()
	default:
		p.error(fmt.Sprintf("expected a statement, got %q", p.cur().Lexeme), p.cur())
		return nil
	}
}

func (p *Parser) parseVarStmt() ast.Stmt {
	start := p.cur().Span.Start
	p.advance() // var
	name := p.expect(token.IDENT)
	p.expect(token.ASSIGN)
	init := p.parseExpr()
	return &ast.VarStmt{Base: ast.Base{S: p.span(start)}, Name: name.Lexeme, Init: init}
}

func (p *Parser) parseAssignStmt() ast.Stmt {
	start := p.cur().Span.Start
	target := p.parsePostfix()
	p.expect(token.ASSIGN)
	value := p.parseExpr()
	return &ast.AssignStmt{Base: ast.Base{S: p.span(start)}, Target: target, Value: value}
}

func (p *Parser) parseIfStmt() ast.Stmt {
	start := p.cur().Span.Start
	p.advance() // if
	p.expect(token.LPAREN)
	cond := p.parseExpr()
	p.expect(token.RPAREN)
	then := p.parseBlock()
	var els []ast.Stmt
	if p.match(token.ELSE) {
		p.advance()
		els = p.parseBlock()
	}
	return &ast.IfStmt{Base: ast.Base{S: p.span(start)}, Cond: cond, Then: then, Else: els}
}

func (p *Parser) parseForStmt() ast.Stmt {
	start := p.cur().Span.Start
	p.advance() // for
	p.expect(token.LPAREN)
	v := p.expect(token.IDENT)
	p.expect(token.IN)
	iter := p.parseExpr()
	p.expect(token.RPAREN)
	body := p.parseBlock()
	return &ast.ForStmt{Base: ast.Base{S: p.span(start)}, Var: v.Lexeme, Iter: iter, Body: body}
}

func (p *Parser) parseReturnStmt() ast.Stmt {
	start := p.cur().Span.Start
	p.advance() // return
	value := p.parseExpr()
	return &ast.ReturnStmt{Base: ast.Base{S: p.span(start)}, Value: value}
}

// --- expressions (precedence climbing) ---

func binaryPrec(t token.TokenType) int {
	switch t {
	case token.OR_OR:
		return 1
	case token.AND_AND:
		return 2
	case token.EQ_EQ, token.NOT_EQ:
		return 3
	case token.LT, token.LT_EQ, token.GT, token.GT_EQ:
		return 4
	case token.PLUS, token.MINUS:
		return 5
	case token.STAR, token.SLASH, token.PERCENT:
		return 6
	}
	return 0
}

func (p *Parser) parseExpr() ast.Expr {
	return p.parseBinary(1)
}

func (p *Parser) parseBinary(minPrec int) ast.Expr {
	left := p.parseUnary()
	for {
		op := p.cur().Type
		prec := binaryPrec(op)
		if prec < minPrec {
			break
		}
		opTok := p.advance()
		right := p.parseBinary(prec + 1) // left-associative
		left = &ast.Binary{Base: ast.Base{S: p.span(left.Span().Start)}, Op: op, L: left, R: right}
		_ = opTok
	}
	return left
}

func (p *Parser) parseUnary() ast.Expr {
	if p.match(token.BANG) || p.match(token.MINUS) {
		start := p.cur().Span.Start
		op := p.advance()
		operand := p.parseUnary()
		return &ast.Unary{Base: ast.Base{S: p.span(start)}, Op: op.Type, Operand: operand}
	}
	return p.parsePostfix()
}

func (p *Parser) parsePostfix() ast.Expr {
	e := p.parsePrimary()
	for {
		switch p.cur().Type {
		case token.LBRACKET:
			p.advance()
			idx := p.parseExpr()
			p.expect(token.RBRACKET)
			e = &ast.Index{Base: ast.Base{S: p.span(e.Span().Start)}, Recv: e, Idx: idx}
		case token.DOT:
			p.advance()
			name := p.expect(token.IDENT)
			if p.match(token.LT) {
				if typeArgs, ok := p.tryParseTypeArgs(); ok {
					args := p.parseArgs()
					e = &ast.MethodCall{Base: ast.Base{S: p.span(e.Span().Start)}, Recv: e, Method: name.Lexeme, TypeArgs: typeArgs, Args: args}
					continue
				}
			}
			if p.match(token.LPAREN) {
				args := p.parseArgs()
				e = &ast.MethodCall{Base: ast.Base{S: p.span(e.Span().Start)}, Recv: e, Method: name.Lexeme, Args: args}
				continue
			}
			e = &ast.FieldAccess{Base: ast.Base{S: p.span(e.Span().Start)}, Recv: e, Field: name.Lexeme}
		case token.LPAREN:
			args := p.parseArgs()
			e = &ast.Call{Base: ast.Base{S: p.span(e.Span().Start)}, Callee: e, Args: args}
		default:
			return e
		}
	}
}

func (p *Parser) parseArgs() []ast.Expr {
	p.expect(token.LPAREN)
	var args []ast.Expr
	if p.cur().Type != token.RPAREN {
		args = append(args, p.parseExpr())
		for p.match(token.COMMA) {
			p.advance()
			if p.match(token.RPAREN) {
				break // trailing comma
			}
			args = append(args, p.parseExpr())
		}
	}
	p.expect(token.RPAREN)
	return args
}

func (p *Parser) parsePrimary() ast.Expr {
	tok := p.cur()
	start := tok.Span.Start
	switch tok.Type {
	case token.INT:
		p.advance()
		return &ast.IntLit{Base: ast.Base{S: p.span(start)}, Value: tok.IntVal}
	case token.FLOAT:
		p.advance()
		return &ast.FloatLit{Base: ast.Base{S: p.span(start)}, Value: tok.FloatVal}
	case token.STRING:
		p.advance()
		return &ast.StringLit{Base: ast.Base{S: p.span(start)}, Value: tok.Lexeme}
	case token.TRUE:
		p.advance()
		return &ast.BoolLit{Base: ast.Base{S: p.span(start)}, Value: true}
	case token.FALSE:
		p.advance()
		return &ast.BoolLit{Base: ast.Base{S: p.span(start)}, Value: false}
	case token.NONE:
		p.advance()
		return &ast.NoneLit{Base: ast.Base{S: p.span(start)}}
	case token.THIS:
		p.advance()
		return &ast.Ident{Base: ast.Base{S: p.span(start)}, Name: "this"}
	case token.LPAREN:
		p.advance()
		e := p.parseExpr()
		p.expect(token.RPAREN)
		return e
	case token.LBRACKET:
		return p.parseArrayLit()
	case token.LBRACE:
		return p.parseMapLit()
	case token.IDENT:
		p.advance()
		if p.match(token.LBRACE) {
			return p.parseClassLit(tok.Lexeme, start)
		}
		ident := &ast.Ident{Base: ast.Base{S: p.span(start)}, Name: tok.Lexeme}
		if p.match(token.LT) {
			if typeArgs, ok := p.tryParseTypeArgs(); ok {
				args := p.parseArgs()
				return &ast.Call{Base: ast.Base{S: p.span(start)}, Callee: ident, TypeArgs: typeArgs, Args: args}
			}
		}
		if p.match(token.LPAREN) {
			args := p.parseArgs()
			return &ast.Call{Base: ast.Base{S: p.span(start)}, Callee: ident, Args: args}
		}
		return ident
	case token.STRING_KW, token.INT_KW, token.FLOAT_KW, token.BOOL_KW:
		// cast: int(x) / float(x) / string(x) / bool(x)
		p.advance()
		p.expect(token.LPAREN)
		arg := p.parseExpr()
		p.expect(token.RPAREN)
		return &ast.Call{Base: ast.Base{S: p.span(start)}, Callee: &ast.Ident{Base: ast.Base{S: p.span(start)}, Name: tok.Lexeme}, Args: []ast.Expr{arg}}
	default:
		p.error(fmt.Sprintf("expected an expression, got %q", tok.Lexeme), tok)
		p.advance()
		return &ast.Ident{Base: ast.Base{S: p.span(start)}, Name: "?"}
	}
}

func (p *Parser) parseClassLit(name string, start token.Position) ast.Expr {
	p.expect(token.LBRACE)
	var fields []ast.FieldInit
	if p.cur().Type != token.RBRACE {
		fields = append(fields, p.parseFieldInit())
		for p.match(token.COMMA) {
			p.advance()
			if p.match(token.RBRACE) {
				break // trailing comma
			}
			fields = append(fields, p.parseFieldInit())
		}
	}
	p.expect(token.RBRACE)
	return &ast.ClassLit{Base: ast.Base{S: p.span(start)}, TypeName: name, Fields: fields}
}

func (p *Parser) parseFieldInit() ast.FieldInit {
	name := p.expect(token.IDENT)
	p.expect(token.COLON)
	value := p.parseExpr()
	return ast.FieldInit{Name: name.Lexeme, Value: value}
}

func (p *Parser) parseArrayLit() ast.Expr {
	start := p.cur().Span.Start
	p.advance() // [
	var elems []ast.Expr
	if p.cur().Type != token.RBRACKET {
		elems = append(elems, p.parseExpr())
		for p.match(token.COMMA) {
			p.advance()
			if p.match(token.RBRACKET) {
				break // trailing comma
			}
			elems = append(elems, p.parseExpr())
		}
	}
	p.expect(token.RBRACKET)
	return &ast.ArrayLit{Base: ast.Base{S: p.span(start)}, Elems: elems}
}

func (p *Parser) parseMapLit() ast.Expr {
	start := p.cur().Span.Start
	p.advance() // {
	var entries []ast.MapEntry
	if p.cur().Type != token.RBRACE {
		entries = append(entries, p.parseMapEntry())
		for p.match(token.COMMA) {
			p.advance()
			if p.match(token.RBRACE) {
				break // trailing comma
			}
			entries = append(entries, p.parseMapEntry())
		}
	}
	p.expect(token.RBRACE)
	return &ast.MapLit{Base: ast.Base{S: p.span(start)}, Entries: entries}
}

func (p *Parser) parseMapEntry() ast.MapEntry {
	key := p.parseExpr()
	p.expect(token.COLON)
	value := p.parseExpr()
	return ast.MapEntry{Key: key, Value: value}
}

// --- prompt body ---

func (p *Parser) parsePromptBody(raw string, span token.Span) *ast.PromptBody {
	sc := &promptScanner{p: p, src: dedent(raw)}
	segs, stop := sc.parseBody()
	if stop != "" {
		p.error(fmt.Sprintf("unexpected closing tag %q in prompt body", stop), token.Token{Span: span})
	}
	return &ast.PromptBody{Base: ast.Base{S: span}, Raw: raw, Segments: segs}
}

// dedent strips the common leading whitespace over non-blank lines (so the
// least-indented line sits at column 0) and trims leading/trailing blank lines.
func dedent(raw string) string {
	lines := strings.Split(raw, "\n")
	minIndent := -1
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		n := 0
		for n < len(l) && (l[n] == ' ' || l[n] == '\t') {
			n++
		}
		if minIndent == -1 || n < minIndent {
			minIndent = n
		}
	}
	if minIndent > 0 {
		for i, l := range lines {
			if strings.TrimSpace(l) == "" {
				continue
			}
			if len(l) >= minIndent {
				lines[i] = l[minIndent:]
			}
		}
	}
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	if end <= start {
		return ""
	}
	return strings.Join(lines[start:end], "\n") + "\n"
}

// parseExprText lexes a substring as an expression and parses it, merging any
// diagnostics into the main parser's list.
func (p *Parser) parseExprText(s string) ast.Expr {
	toks, _ := lexer.New(s).Lex()
	sub := New(toks)
	e := sub.parseExpr()
	if sub.cur().Type != token.EOF {
		sub.error(fmt.Sprintf("unexpected token %q in expression", sub.cur().Lexeme), sub.cur())
	}
	p.diags = append(p.diags, sub.diags...)
	return e
}

// promptScanner walks the raw prompt-body text, splitting literal runs from
// {{ }} interpolations and {% %} control tags.
type promptScanner struct {
	p   *Parser
	src string
	pos int
}

func (s *promptScanner) eof() bool { return s.pos >= len(s.src) }

// parseBody parses segments until {% end %}, {% else %}, or EOF. It returns the
// segments and the stop tag ("end", "else", or "" for EOF).
func (s *promptScanner) parseBody() ([]ast.PromptSegment, string) {
	var segs []ast.PromptSegment
	var text strings.Builder
	flush := func() {
		if text.Len() > 0 {
			segs = append(segs, &ast.TextSegment{Text: resolvePromptEscapes(text.String())})
			text.Reset()
		}
	}
	for !s.eof() {
		c := s.src[s.pos]
		switch {
		case c == '\\':
			text.WriteByte(c)
			s.pos++
			if !s.eof() {
				text.WriteByte(s.src[s.pos])
				s.pos++
			}
		case strings.HasPrefix(s.src[s.pos:], "{{"):
			flush()
			s.pos += 2
			inner, ok := s.readUntil("}}")
			if !ok {
				s.p.error("unterminated interpolation in prompt body", token.Token{})
				return segs, ""
			}
			segs = append(segs, s.parseInterp(inner))
		case strings.HasPrefix(s.src[s.pos:], "{%"):
			flush()
			s.pos += 2
			inner, ok := s.readUntil("%}")
			if !ok {
				s.p.error("unterminated control tag in prompt body", token.Token{})
				return segs, ""
			}
			s.skipNewline() // control tags occupy their own line; drop its newline
			tag := strings.TrimSpace(inner)
			switch {
			case tag == "end" || tag == "else":
				return segs, tag
			case strings.HasPrefix(tag, "for "):
				segs = append(segs, s.parseForTag(tag))
			case strings.HasPrefix(tag, "if "):
				segs = append(segs, s.parseIfTag(tag))
			default:
				s.p.error(fmt.Sprintf("unknown control tag %q", tag), token.Token{})
			}
		default:
			text.WriteByte(c)
			s.pos++
		}
	}
	flush()
	return segs, ""
}

func (s *promptScanner) skipNewline() {
	if !s.eof() && s.src[s.pos] == '\n' {
		s.pos++
	}
}

func (s *promptScanner) readUntil(delim string) (string, bool) {
	if i := strings.Index(s.src[s.pos:], delim); i >= 0 {
		inner := s.src[s.pos : s.pos+i]
		s.pos += i + len(delim)
		return inner, true
	}
	inner := s.src[s.pos:]
	s.pos = len(s.src)
	return inner, false
}

func (s *promptScanner) parseInterp(inner string) ast.PromptSegment {
	inner = strings.TrimSpace(inner)
	if strings.HasPrefix(inner, "include") && (len(inner) == len("include") || inner[len("include")] == ' ' || inner[len("include")] == '\t') {
		seg := s.parseInclude(strings.TrimSpace(inner[len("include"):]))
		s.skipNewline() // an own-line include consumes its newline (child emits it)
		return seg
	}
	expr := s.p.parseExprText(inner)
	return &ast.InterpSegment{Expr: expr}
}

func (s *promptScanner) parseInclude(rest string) ast.PromptSegment {
	open := strings.Index(rest, "(")
	if open < 0 {
		s.p.error("expected '(' in include", token.Token{})
		return &ast.IncludeSegment{Template: rest}
	}
	name := strings.TrimSpace(rest[:open])
	argsText := rest[open+1:]
	closeIdx := strings.LastIndex(argsText, ")")
	argsStr := ""
	if closeIdx >= 0 {
		argsStr = strings.TrimSpace(argsText[:closeIdx])
	}
	seg := &ast.IncludeSegment{Template: name}
	if argsStr != "" {
		for _, part := range splitTopLevel(argsStr, ',') {
			part = strings.TrimSpace(part)
			colon := strings.Index(part, ":")
			if colon < 0 {
				s.p.error("expected 'name: value' in include arguments", token.Token{})
				continue
			}
			argName := strings.TrimSpace(part[:colon])
			argExpr := s.p.parseExprText(strings.TrimSpace(part[colon+1:]))
			seg.Args = append(seg.Args, ast.NamedArg{Name: argName, Value: argExpr})
		}
	}
	return seg
}

func (s *promptScanner) parseForTag(tag string) ast.PromptSegment {
	rest := strings.TrimSpace(strings.TrimPrefix(tag, "for"))
	parts := strings.SplitN(rest, " in ", 2)
	if len(parts) != 2 {
		s.p.error("expected 'for <name> in <expr>'", token.Token{})
		return &ast.ForSegment{Var: strings.TrimSpace(parts[0])}
	}
	varName := strings.TrimSpace(parts[0])
	iter := s.p.parseExprText(strings.TrimSpace(parts[1]))
	body, stop := s.parseBody()
	if stop != "end" {
		s.p.error("expected {% end %} to close for-loop", token.Token{})
	}
	return &ast.ForSegment{Var: varName, Iter: iter, Body: body}
}

func (s *promptScanner) parseIfTag(tag string) ast.PromptSegment {
	condText := strings.TrimSpace(strings.TrimPrefix(tag, "if"))
	cond := s.p.parseExprText(condText)
	then, stop := s.parseBody()
	var els []ast.PromptSegment
	if stop == "else" {
		els, stop = s.parseBody()
	}
	if stop != "end" {
		s.p.error("expected {% end %} to close if-block", token.Token{})
	}
	return &ast.IfSegment{Cond: cond, Then: then, Else: els}
}

// splitTopLevel splits s on sep at depth 0 (ignoring nested (), []).
func splitTopLevel(s string, sep byte) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case sep:
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// resolvePromptEscapes resolves the prompt-text escapes \{ \} \\ into their
// literal forms. Any other backslash is kept verbatim.
func resolvePromptEscapes(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case '{':
				b.WriteByte('{')
				i++
				continue
			case '}':
				b.WriteByte('}')
				i++
				continue
			case '\\':
				b.WriteByte('\\')
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
