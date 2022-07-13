package injector

import (
	"fmt"
	"github.com/pkg/errors"
	"reflect"
	"regexp"
	"runtime/debug"
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
		kind, v := state.Inj.ParseTagValue(state.Param)
		var memberSet = map[string]bool{}
		var memberArr []string
		if kind == reflect.String {
			memberArr = strings.Split(state.Param, enumSep)
		} else if arr, ok := v.([]string); ok {
			memberArr = arr
		} else {
			panic(fmt.Sprintf("enum members type need comma separated string or a variable refers to string slice, got %T", v))
		}
		for _, name := range memberArr {
			memberSet[name] = true
		}
		return func(state *TagFnState) (value reflect.Value, err error) {
			p := state.Field.Interface().(string)
			if memberSet[p] {
				return state.Field, nil
			}
			if p == "" && len(memberArr) > 0 {
				return reflect.ValueOf(memberArr[0]), nil
			}
			return state.Field, errors.Errorf("[%s] is not valid option", p)
		}
	default:
		panic(fmt.Sprintf("enum not supported for %s", state.Field.Kind()))
	}
}

type byResolver struct {
	matchTagBinding func(useResolver string, byFieldNames []string, by []reflect.Type, to reflect.Type) func(state *TagFnState) (value reflect.Value, err error)
}

var (
	typTagFnStatePtr = reflect.TypeOf((*TagFnState)(nil))
)

func (inj *Inject) AddResolver(fn interface{}) (err error) {
	return inj.AddResolverWithName("", fn)
}

func (inj *Inject) AddResolverWithName(resolverName string, fn interface{}) (err error) {
	rfn := reflect.ValueOf(fn)
	tfn := reflect.TypeOf(fn)
	if rfn.Kind() != reflect.Func || rfn.IsNil() {
		return errors.New("by resolver must be func")
	}

	resolverInParams := make([]reflect.Type, tfn.NumIn())
	resolverOutParams := make([]reflect.Type, tfn.NumOut())
	for i := 0; i < tfn.NumIn(); i++ {
		resolverInParams[i] = tfn.In(i)
	}
	for i := 0; i < tfn.NumOut(); i++ {
		resolverOutParams[i] = tfn.Out(i)
	}
	if tfn.NumOut() > 2 || tfn.NumOut() < 1 {
		return errors.New("by resolver must have 1 or 2 out params")
	}
	if tfn.NumOut() == 2 && !tfn.Out(1).Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		return errors.New("by resolver second out param must be error")
	}
	outTyp := tfn.Out(0)
	inj.byResolver = append(inj.byResolver, byResolver{
		matchTagBinding: func(useResolver string, byFieldNames []string, byTypes []reflect.Type, to reflect.Type) func(state *TagFnState) (value reflect.Value, err error) {
			if useResolver != "" && useResolver != resolverName {
				return nil
			}
			if !outTyp.AssignableTo(to) {
				return nil
			}

			resolverInParamsMaker := make([]func(state *TagFnState) (value reflect.Value, err error), len(resolverInParams))
			resolverInParamsIdx, byIdx := 0, 0
			for resolverInParamsIdx < len(resolverInParams) && byIdx < len(byTypes) {
				if byTypes[byIdx].AssignableTo(resolverInParams[resolverInParamsIdx]) {
					thisByIdx := byIdx
					resolverInParamsMaker[resolverInParamsIdx] = func(state *TagFnState) (value reflect.Value, err error) {
						v, _, ok := state.Inj.GetStructFieldOK(state.CurrentStruct, byFieldNames[thisByIdx])
						if !ok {
							return value, errors.New(fmt.Sprintf("get filed %s error", byFieldNames[thisByIdx]))
						}
						return v, nil
					}
					resolverInParamsIdx++
					byIdx++
				} else if typTagFnStatePtr.AssignableTo(resolverInParams[resolverInParamsIdx]) {
					resolverInParamsMaker[resolverInParamsIdx] = func(state *TagFnState) (value reflect.Value, err error) {
						return reflect.ValueOf(state), nil
					}
					resolverInParamsIdx++
				} else {
					return nil
				}
			}
			if resolverInParamsIdx < len(resolverInParams) || byIdx < len(byTypes) {
				return nil
			}

			return func(state *TagFnState) (value reflect.Value, err error) {
				defer func() {
					e := recover()
					if e == nil {
						return
					}
					println(e)
					debug.PrintStack()
					var ok bool
					err, ok = e.(error)
					if !ok {
						err = errors.New(fmt.Sprintf("paniked: %v", e))
					}
				}()

				in := make([]reflect.Value, len(resolverInParamsMaker))
				for i, maker := range resolverInParamsMaker {
					var param reflect.Value
					param, err = maker(state)
					if err != nil {
						return value, errors.Wrap(err, fmt.Sprintf("make resolver param %d [%s] error", i, resolverInParams[i].String()))
					}
					in[i] = param
				}
				out := rfn.Call(in)
				if len(out) == 2 {
					if !out[1].IsNil() {
						return value, out[1].Interface().(error)
					}
				}
				return out[0], nil
			}
		},
	})
	return nil
}

var regUseResolver = regexp.MustCompile(`^@(\w+):`)

func byTag(state *TagFnState) TagFn {
	var in []reflect.Type
	var useResolver string
	var parts []string
	if use := regUseResolver.FindStringSubmatch(state.Param); len(use) == 2 {
		useResolver = use[1]
		parts = strings.Split(regUseResolver.ReplaceAllString(state.Param, ""), bySep)
	} else {
		parts = strings.Split(state.Param, bySep)
	}

	for _, part := range parts {
		v, _, ok := state.Inj.GetStructFieldOK(state.CurrentStruct, part)
		if !ok {
			panic(fmt.Sprintf("by tag init panic: struct: %s, param: %s, get field %s not ok",
				state.CurrentStruct.Type(), state.Param, part))
		}
		in = append(in, v.Type())
	}
	out := state.Field.Type()
	var exec func(state *TagFnState) (value reflect.Value, err error)
	for _, r := range state.Inj.byResolver {
		exec = r.matchTagBinding(useResolver, parts, in, out)
		if exec != nil {
			break
		}
	}
	if exec == nil {
		panic(fmt.Sprintf("by tag init panic: struct: %s, param: %s; cant find resolver, are you registered?",
			state.CurrentStruct.Type(), state.Param))
	}
	return exec
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
