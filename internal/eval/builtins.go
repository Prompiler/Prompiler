package eval

import (
	"fmt"
	"strings"

	"github.com/Jh123x/prompiler/internal/token"
	"github.com/Jh123x/prompiler/internal/types"
)

func (e *Evaluator) callBuiltinFunc(name string, args []Value, span token.Span) (Value, *RuntimeError) {
	switch name {
	case "range":
		n := args[0].i
		if n < 0 {
			return Value{}, rterr(token.CatRangeNegative, span, "range(stop) requires a non-negative stop")
		}
		elems := make([]Value, n)
		for i := int64(0); i < n; i++ {
			elems[i] = IntVal(i)
		}
		return ArrayVal(types.TypeInt, elems), nil
	case "range_from":
		start, stop := args[0].i, args[1].i
		var elems []Value
		for i := start; i < stop; i++ {
			elems = append(elems, IntVal(i))
		}
		return ArrayVal(types.TypeInt, elems), nil
	case "range_step":
		start, stop, step := args[0].i, args[1].i, args[2].i
		if step == 0 {
			return Value{}, rterr(token.CatRangeStepZero, span, "range_step step cannot be zero")
		}
		var elems []Value
		if step > 0 {
			for i := start; i < stop; {
				elems = append(elems, IntVal(i))
				next := i + step
				if next < i {
					return Value{}, rterr(token.CatOverflow, span, "range_step overflow")
				}
				i = next
			}
		} else {
			for i := start; i > stop; {
				elems = append(elems, IntVal(i))
				next := i + step
				if next > i {
					return Value{}, rterr(token.CatOverflow, span, "range_step overflow")
				}
				i = next
			}
		}
		return ArrayVal(types.TypeInt, elems), nil
	case "join":
		arr := *args[0].arr
		sep := args[1].str
		parts := make([]string, len(arr))
		for i, el := range arr {
			parts[i] = el.str
		}
		return StringVal(strings.Join(parts, sep)), nil
	}
	return Value{}, rterr(token.CatUnknownName, span, fmt.Sprintf("unknown function %q", name))
}

func (e *Evaluator) callBuiltinMethod(recv Value, name string, args []Value, span token.Span) (Value, *RuntimeError) {
	if recv.isString() {
		switch name {
		case "upper":
			return StringVal(strings.ToUpper(recv.str)), nil
		case "lower":
			return StringVal(strings.ToLower(recv.str)), nil
		case "trim":
			return StringVal(strings.TrimSpace(recv.str)), nil
		case "replace_all":
			return StringVal(strings.ReplaceAll(recv.str, args[0].str, args[1].str)), nil
		case "replace":
			return StringVal(strings.Replace(recv.str, args[0].str, args[1].str, int(args[2].i))), nil
		}
	}
	if recv.m != nil {
		switch name {
		case "keys":
			elems := make([]Value, len(recv.m.keys))
			for i, k := range recv.m.keys {
				elems[i] = StringVal(k)
			}
			return ArrayVal(types.TypeString, elems), nil
		case "values":
			vt := recv.typ.(*types.Map).Value
			elems := make([]Value, len(recv.m.keys))
			for i, k := range recv.m.keys {
				elems[i] = recv.m.vals[k]
			}
			return ArrayVal(vt, elems), nil
		}
	}
	if recv.opt != nil {
		switch name {
		case "is_empty":
			return BoolVal(!recv.opt.present), nil
		case "value":
			if !recv.opt.present {
				return Value{}, rterr(token.CatUnwrapEmpty, span, "unwrap of empty Optional")
			}
			return recv.opt.value, nil
		}
	}
	return Value{}, rterr(token.CatUnknownMethod, span, fmt.Sprintf("no method %q", name))
}
