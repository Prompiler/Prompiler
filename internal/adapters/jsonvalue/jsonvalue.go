// Package jsonvalue is a ValueSource adapter that coerces untyped JSON (a
// template's variables.json) into typed input values, preserving map key order.
package jsonvalue

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"

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
func parseJSON(data []byte) (*jsonNode, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	n, err := parseValue(dec)
	if err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, fmt.Errorf("unexpected trailing JSON")
	}
	return n, nil
}

func parseValue(dec *json.Decoder) (*jsonNode, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, isDelim := tok.(json.Delim)
	if !isDelim {
		switch v := tok.(type) {
		case string:
			return &jsonNode{kind: 's', str: v}, nil
		case json.Number:
			return &jsonNode{kind: 'n', num: v.String()}, nil
		case bool:
			return &jsonNode{kind: 'b', bl: v}, nil
		case nil:
			return &jsonNode{kind: 'z'}, nil
		}
		return nil, fmt.Errorf("unexpected JSON token %v", tok)
	}
	switch delim {
	case '{':
		node := &jsonNode{kind: 'o', obj: map[string]*jsonNode{}}
		for dec.More() {
			keyTok, err := dec.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyTok.(string)
			if !ok {
				return nil, fmt.Errorf("object key is not a string")
			}
			val, err := parseValue(dec)
			if err != nil {
				return nil, err
			}
			node.keys = append(node.keys, key)
			node.obj[key] = val
		}
		dec.Token() // consume '}'
		return node, nil
	case '[':
		node := &jsonNode{kind: 'a'}
		for dec.More() {
			val, err := parseValue(dec)
			if err != nil {
				return nil, err
			}
			node.arr = append(node.arr, val)
		}
		dec.Token() // consume ']'
		return node, nil
	}
	return nil, fmt.Errorf("unexpected delimiter %v", delim)
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
