// Package jsonvalue is a ValueSource adapter that coerces untyped JSON (a
// template's variables.json) into typed input values, preserving map key order.
package jsonvalue

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"

	"github.com/Jh123x/prompiler/internal/domain"
	"github.com/Jh123x/prompiler/internal/eval"
	"github.com/Jh123x/prompiler/internal/types"
)

// jsonNode is an order-preserving JSON value.
type jsonNode struct {
	kind byte // 'o' object, 'a' array, 's' string, 'n' number, 'b' bool, 'z' null
	keys []string
	obj  map[string]*jsonNode
	arr  []*jsonNode
	str  string
	num  string // raw number text, parsed per target type to avoid float64 precision loss
	bl   bool
}

// Source is a JSON ValueSource.
type Source struct {
	root *jsonNode
	env  *types.Env
}

// New parses data and wires the type environment (for $type resolution).
func New(data []byte, env *types.Env) (*Source, error) {
	root, err := parseJSON(data)
	if err != nil {
		return nil, err
	}
	return &Source{root: root, env: env}, nil
}

// Provide implements domain.ValueSource.
func (s *Source) Provide(vars []domain.Variable) (eval.InputValues, error) {
	inputs := eval.InputValues{}
	for _, v := range vars {
		node, ok := s.root.obj[v.Name]
		if !ok {
			continue // omitted; the default applies
		}
		val, err := coerce(v.Type, node, s.env)
		if err != nil {
			return nil, fmt.Errorf("variable %q: %w", v.Name, err)
		}
		inputs[v.Name] = val
	}
	return inputs, nil
}

// parseJSON parses data into an order-preserving jsonNode.
//
// sonic has no streaming Token()/Delim API, so a strict full-document decode
// (sonic.Unmarshal) validates the input and rejects malformed JSON and trailing
// content, then the sonic ast tree is walked to rebuild the order-preserving
// jsonNode. The ast preserves object key order and raw number text.
func parseJSON(data []byte) (*jsonNode, error) {
	var anyVal any
	if err := sonic.Unmarshal(data, &anyVal); err != nil {
		return nil, err
	}
	parser := ast.NewParser(string(data))
	n, perr := parser.Parse()
	if perr != 0 {
		return nil, parser.ExportError(perr)
	}
	return buildNode(&n)
}

// buildNode walks a sonic ast.Node into the order-preserving jsonNode.
func buildNode(n *ast.Node) (*jsonNode, error) {
	switch n.TypeSafe() {
	case ast.V_OBJECT:
		node := &jsonNode{kind: 'o', obj: map[string]*jsonNode{}}
		props, err := n.Properties()
		if err != nil {
			return nil, err
		}
		var pair ast.Pair
		for props.Next(&pair) {
			child, err := buildNode(&pair.Value)
			if err != nil {
				return nil, err
			}
			node.keys = append(node.keys, pair.Key)
			node.obj[pair.Key] = child
		}
		return node, nil
	case ast.V_ARRAY:
		node := &jsonNode{kind: 'a'}
		vals, err := n.Values()
		if err != nil {
			return nil, err
		}
		var child ast.Node
		for vals.Next(&child) {
			sub, err := buildNode(&child)
			if err != nil {
				return nil, err
			}
			node.arr = append(node.arr, sub)
		}
		return node, nil
	case ast.V_STRING:
		s, err := n.String()
		if err != nil {
			return nil, err
		}
		return &jsonNode{kind: 's', str: s}, nil
	case ast.V_NUMBER:
		num, err := n.Number()
		if err != nil {
			return nil, err
		}
		return &jsonNode{kind: 'n', num: num.String()}, nil
	case ast.V_TRUE:
		return &jsonNode{kind: 'b', bl: true}, nil
	case ast.V_FALSE:
		return &jsonNode{kind: 'b', bl: false}, nil
	case ast.V_NULL:
		return &jsonNode{kind: 'z'}, nil
	}
	return nil, fmt.Errorf("unsupported JSON node type %d", n.TypeSafe())
}

func coerce(t types.Type, n *jsonNode, env *types.Env) (eval.Value, error) {
	switch tt := t.(type) {
	case types.Primitive:
		switch tt {
		case types.TypeString:
			if n.kind != 's' {
				return eval.Value{}, fmt.Errorf("expected string")
			}
			return eval.StringVal(n.str), nil
		case types.TypeInt:
			if n.kind != 'n' {
				return eval.Value{}, fmt.Errorf("expected int")
			}
			i, err := strconv.ParseInt(n.num, 10, 64)
			if err != nil {
				return eval.Value{}, fmt.Errorf("%q is not a valid int", n.num)
			}
			return eval.IntVal(i), nil
		case types.TypeFloat:
			if n.kind != 'n' {
				return eval.Value{}, fmt.Errorf("expected float")
			}
			f, err := strconv.ParseFloat(n.num, 64)
			if err != nil {
				return eval.Value{}, fmt.Errorf("%q is not a valid float", n.num)
			}
			return eval.FloatVal(f), nil
		case types.TypeBool:
			if n.kind != 'b' {
				return eval.Value{}, fmt.Errorf("expected bool")
			}
			return eval.BoolVal(n.bl), nil
		}
	case *types.Enum:
		if n.kind != 's' {
			return eval.Value{}, fmt.Errorf("expected enum member")
		}
		if !slices.Contains(tt.Members, n.str) {
			return eval.Value{}, fmt.Errorf("%q is not a member of %s", n.str, tt.Name)
		}
		return eval.EnumVal(tt.Name, n.str), nil
	case *types.Array:
		if n.kind != 'a' {
			return eval.Value{}, fmt.Errorf("expected array")
		}
		elems := make([]eval.Value, len(n.arr))
		for i, el := range n.arr {
			v, err := coerce(tt.Elem, el, env)
			if err != nil {
				return eval.Value{}, err
			}
			elems[i] = v
		}
		return eval.ArrayVal(tt.Elem, elems), nil
	case *types.Map:
		if n.kind != 'o' {
			return eval.Value{}, fmt.Errorf("expected object")
		}
		vals := map[string]eval.Value{}
		for _, k := range n.keys {
			v, err := coerce(tt.Value, n.obj[k], env)
			if err != nil {
				return eval.Value{}, err
			}
			vals[k] = v
		}
		return eval.MapVal(tt.Value, n.keys, vals), nil
	case *types.Optional:
		if n.kind == 'z' {
			return eval.NoneVal(tt.Elem), nil
		}
		v, err := coerce(tt.Elem, n, env)
		if err != nil {
			return eval.Value{}, err
		}
		return eval.SomeVal(v), nil
	case *types.Class:
		if n.kind != 'o' {
			return eval.Value{}, fmt.Errorf("expected object")
		}
		fields := map[string]eval.Value{}
		for _, k := range n.keys {
			ft, ok := tt.FieldType(k)
			if !ok {
				return eval.Value{}, fmt.Errorf("class %s has no field %q", tt.Name, k)
			}
			v, err := coerce(ft, n.obj[k], env)
			if err != nil {
				return eval.Value{}, err
			}
			fields[k] = v
		}
		return eval.ObjectVal(tt, fields), nil
	case *types.Interface:
		if n.kind != 'o' {
			return eval.Value{}, fmt.Errorf("expected object")
		}
		typeNode, ok := n.obj["$type"]
		if !ok || typeNode.kind != 's' {
			return eval.Value{}, fmt.Errorf("interface value requires a \"$type\" discriminator")
		}
		cls, ok := env.Types[typeNode.str].(*types.Class)
		if !ok {
			return eval.Value{}, fmt.Errorf("unknown class %q", typeNode.str)
		}
		fields := map[string]eval.Value{}
		for _, k := range n.keys {
			if k == "$type" {
				continue
			}
			ft, ok := cls.FieldType(k)
			if !ok {
				return eval.Value{}, fmt.Errorf("class %s has no field %q", cls.Name, k)
			}
			v, err := coerce(ft, n.obj[k], env)
			if err != nil {
				return eval.Value{}, err
			}
			fields[k] = v
		}
		return eval.ObjectVal(cls, fields), nil
	}
	return eval.Value{}, fmt.Errorf("cannot coerce %s", t)
}
