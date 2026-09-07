package eval

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Jh123x/prompiler/internal/ast"
	"github.com/Jh123x/prompiler/internal/token"
	"github.com/Jh123x/prompiler/internal/types"
)

// InputValues is the set of typed values bound to a template's variables.
type InputValues map[string]Value

// TemplateComposer renders a template (and its include DAG) to a string.
type TemplateComposer struct {
	eval      *Evaluator
	templates map[string]*ast.TemplateDecl
}

// NewComposer wires the composer to the parsed files and semantic model (DI).
func NewComposer(files map[string]*ast.File, sem *types.SemanticModel) *TemplateComposer {
	return &TemplateComposer{eval: NewEvaluator(files, sem), templates: collectTemplates(files)}
}

func collectTemplates(files map[string]*ast.File) map[string]*ast.TemplateDecl {
	t := map[string]*ast.TemplateDecl{}
	for _, f := range files {
		for _, d := range f.Decls {
			if td, ok := d.(*ast.TemplateDecl); ok {
				t[td.Name] = td
			}
		}
	}
	return t
}

// Render renders the named template given its bound inputs.
func (c *TemplateComposer) Render(name string, inputs InputValues) (string, *RuntimeError) {
	td := c.templates[name]
	if td == nil {
		return "", rterr(token.CatUnknownName, token.Span{}, fmt.Sprintf("unknown template %q", name))
	}
	env := newEvalEnv(nil)
	varTypes := c.eval.sem.TemplateVars[name]
	for _, v := range td.Variables {
		if val, ok := inputs[v.Name]; ok {
			env.define(v.Name, val)
			continue
		}
		if v.HasDefault {
			dv, err := c.evalDefault(v.Default, varTypes[v.Name])
			if err != nil {
				return "", err
			}
			env.define(v.Name, dv)
			continue
		}
		return "", rterr(token.CatUnknownName, v.Span(), fmt.Sprintf("missing value for variable %q", v.Name))
	}
	return c.renderSegments(td.Prompt.Segments, env)
}

// evalDefault evaluates a variable default, which may be an enum member
// (`level: LogLevel = Warning`) or `none` for an Optional.
func (c *TemplateComposer) evalDefault(d ast.Expr, target types.Type) (Value, *RuntimeError) {
	switch x := d.(type) {
	case *ast.NoneLit:
		if opt, ok := target.(*types.Optional); ok {
			return NoneVal(opt.Elem), nil
		}
	case *ast.Ident:
		if enum, ok := target.(*types.Enum); ok {
			return EnumVal(enum.Name, x.Name), nil
		}
	}
	return c.eval.EvalExpr(d, newEvalEnv(nil))
}

func (c *TemplateComposer) renderSegments(segs []ast.PromptSegment, env *evalEnv) (string, *RuntimeError) {
	var sb strings.Builder
	col := 0
	for _, seg := range segs {
		switch s := seg.(type) {
		case *ast.TextSegment:
			for _, ch := range s.Text {
				sb.WriteRune(ch)
				if ch == '\n' {
					col = 0
				} else {
					col++
				}
			}
		case *ast.InterpSegment:
			v, err := c.eval.EvalExpr(s.Expr, env)
			if err != nil {
				return "", err
			}
			str, ok := Stringify(v)
			if !ok {
				return "", rterr(token.CatTypeMismatch, s.Span(), "not stringable")
			}
			writeAnchored(&sb, str, col)
			col += lastLineLen(str)
		case *ast.ForSegment:
			it, err := c.eval.EvalExpr(s.Iter, env)
			if err != nil {
				return "", err
			}
			for _, el := range *it.arr {
				loop := newEvalEnv(env)
				loop.define(s.Var, el)
				out, err := c.renderSegments(s.Body, loop)
				if err != nil {
					return "", err
				}
				sb.WriteString(out)
				col = lastLineLen(out)
			}
		case *ast.IfSegment:
			cv, err := c.eval.EvalExpr(s.Cond, env)
			if err != nil {
				return "", err
			}
			branch := s.Else
			if cv.b {
				branch = s.Then
			}
			out, err := c.renderSegments(branch, env)
			if err != nil {
				return "", err
			}
			sb.WriteString(out)
			col = lastLineLen(out)
		case *ast.IncludeSegment:
			child := c.templates[s.Template]
			if child == nil || child.Prompt == nil {
				return "", rterr(token.CatUnknownName, s.Span(), fmt.Sprintf("unknown template %q", s.Template))
			}
			childEnv := newEvalEnv(nil)
			for _, a := range s.Args {
				v, err := c.eval.EvalExpr(a.Value, env)
				if err != nil {
					return "", err
				}
				childEnv.define(a.Name, v)
			}
			out, err := c.renderSegments(child.Prompt.Segments, childEnv)
			if err != nil {
				return "", err
			}
			writeAnchored(&sb, out, col)
			col += lastLineLen(out)
		}
	}
	return sb.String(), nil
}

// writeAnchored writes a (possibly multi-line) value at the current column,
// anchoring each subsequent line to that column plus its own relative indent.
func writeAnchored(sb *strings.Builder, str string, col int) {
	lines := strings.Split(str, "\n")
	sb.WriteString(lines[0])
	for _, line := range lines[1:] {
		sb.WriteByte('\n')
		sb.WriteString(strings.Repeat(" ", col))
		sb.WriteString(line)
	}
}

func lastLineLen(s string) int {
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return utf8.RuneCountInString(s[i+1:])
	}
	return utf8.RuneCountInString(s)
}
