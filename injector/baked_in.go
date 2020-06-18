package injector

import (
	"fmt"
	"github.com/pkg/errors"
	"reflect"
	"strings"
)

const (
	enumSep = ","
	bySep   = ","
)

var bakedInAliasInjectors = map[string]string{}

var bakedInInjectorsInit = map[string]TagInitFunc{
	"enum":    enumTag,
	"by":      byTag,
	"default": defaultTag,
}

func defaultTag(state *TagFnState) TagFn {
	if len(state.Param) == 0 {
		panic("default tag need a param")
	}

	kind, d := state.Inj.ParseTagValue(state.Param)
	var getDefault func() reflect.Value
	switch state.Field.Kind() {
	case reflect.String:
		if kind == reflect.String {
			defaultStr := reflect.ValueOf(d)
			getDefault = func() reflect.Value {
				return defaultStr
			}
		} else if kind == reflect.Func {
			fn := d.(func() string)
			getDefault = func() reflect.Value {
				return reflect.ValueOf(fn())
			}
		} else {
			panic(fmt.Sprintf("defualt not support for %s, by %s", kind, state.Param))
		}
		return func(state *TagFnState) (value reflect.Value, err error) {
			p := state.Field.Interface().(string)
			if p == "" {
				return getDefault(), nil
			}
			return state.Field, nil
		}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if kind == reflect.String {
			def := reflect.ValueOf(asInt(d.(string))).Convert(state.Field.Type())
			getDefault = func() reflect.Value {
				return def
			}
		} else if kind == reflect.Func {
			fn := d.(func() int64)
			getDefault = func() reflect.Value {
				return reflect.ValueOf(fn()).Convert(state.Field.Type())
			}
		} else {
			panic(fmt.Sprintf("defualt not support for %s, by %s", kind, state.Param))
		}
		return func(state *TagFnState) (value reflect.Value, err error) {
			p := state.Field.Int()
			if p == 0 {
				return getDefault(), nil
			}
			return state.Field, nil
		}
	case reflect.Uint, reflect.Uintptr, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if kind == reflect.String {
			def := reflect.ValueOf(asInt(d.(string))).Convert(state.Field.Type())
			getDefault = func() reflect.Value {
				return def
			}
		} else if kind == reflect.Func {
			fn := d.(func() int64)
			getDefault = func() reflect.Value {
				return reflect.ValueOf(fn()).Convert(state.Field.Type())
			}
		} else {
			panic(fmt.Sprintf("defualt not support for %s, by %s", kind, state.Param))
		}
		return func(state *TagFnState) (value reflect.Value, err error) {
			p := state.Field.Uint()
			if p == 0 {
				return getDefault(), nil
			}
			return state.Field, nil
		}
	default:
		panic(fmt.Sprintf("endefault tag not supported for %s", state.Field.Kind()))
	}
}

func enumTag(state *TagFnState) TagFn {
	if len(state.Param) == 0 {
		panic("enum tag need 1 param at least")
	}

	switch state.Field.Kind() {
	case reflect.String:
		members := strings.Split(state.Param, enumSep)
		return func(state *TagFnState) (value reflect.Value, err error) {
			p := state.Field.Interface().(string)
			for _, member := range members {
				if p == member {
					return state.Field, nil
				}
			}
			return reflect.ValueOf(members[0]), nil
		}
	default:
		panic(fmt.Sprintf("enum not supported for %s", state.Field.Kind()))
	}
}

type byResolver struct {
	in  []reflect.Type
	out []reflect.Type
	fn  reflect.Value
}

func (inj *Inject) AddResolver(fn interface{}) (err error) {
	rfn := reflect.ValueOf(fn)
	tfn := reflect.TypeOf(fn)
	if rfn.Kind() != reflect.Func || rfn.IsNil() {
		return errors.New("by resolver must be func")
	}
	if tfn.NumOut() != 2 || tfn.Out(1) != reflect.TypeOf((*error)(nil)).Elem() {
		return errors.New("by resolver must return 2 param, and the second must be error")
	}
	r := byResolver{
		in:  make([]reflect.Type, tfn.NumIn()),
		out: make([]reflect.Type, tfn.NumOut()),
		fn:  rfn,
	}
	for i := 0; i < tfn.NumIn(); i++ {
		r.in[i] = tfn.In(i)
	}
	for i := 0; i < tfn.NumOut(); i++ {
		r.out[i] = tfn.Out(i)
	}
	inj.byResolver = append(inj.byResolver, r)
	return
}

func byTag(state *TagFnState) TagFn {
	var in, out []reflect.Type
	parts := strings.Split(state.Param, bySep)
	for _, part := range parts {
		v, _, ok := state.Inj.GetStructFieldOK(state.CurrentStruct, part)
		if !ok {
			panic(fmt.Sprintf("by tag init panic: struct: %s, param: %s, get field %s not ok",
				state.CurrentStruct.Type(), state.Param, part))
		}
		in = append(in, v.Type())
	}
	out = append(out, state.Field.Type())
	out = append(out, reflect.TypeOf((*error)(nil)).Elem())
	var fn reflect.Value

OUTER:
	for _, resolver := range state.Inj.byResolver {
		if len(resolver.in) != len(in) || len(resolver.out) != len(out) {
			continue
		}
		for idx, typ := range resolver.in {
			if in[idx] != typ {
				continue OUTER
			}
		}
		for idx, typ := range resolver.out {
			if out[idx] != typ {
				continue OUTER
			}
		}
		fn = resolver.fn
	}

	if !fn.IsValid() {
		panic(fmt.Sprintf("by tag init panic: struct: %s, param: %s; cant find resolver, are you registered?",
			state.CurrentStruct.Type(), state.Param))
	}

	return func(state *TagFnState) (value reflect.Value, err error) {
		var inParams []reflect.Value
		for _, part := range parts {
			v, _, ok := state.Inj.GetStructFieldOK(state.CurrentStruct, part)
			if !ok {
				panic(fmt.Sprintf("by tag init panic: struct: %s, param: %s, get field %s not ok",
					state.CurrentStruct.Type(), state.Param, part))
			}
			inParams = append(inParams, v)
		}
		outParams := fn.Call(inParams)
		value = outParams[0]
		if !outParams[1].IsNil() {
			err = outParams[1].Interface().(error)
		}
		return
	}
}

// 0值判断
func HasValue(field reflect.Value, fieldType reflect.Type, fieldKind reflect.Kind) bool {

	switch fieldKind {
	case reflect.Slice, reflect.Map, reflect.Ptr, reflect.Interface, reflect.Chan, reflect.Func:
		return !field.IsNil()
	default:
		return field.IsValid() && field.Interface() != reflect.Zero(fieldType).Interface()
	}
}
