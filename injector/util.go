package injector

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

const (
	blank              = ""
	namespaceSeparator = "."
	leftBracket        = "["
	rightBracket       = "]"
	restrictedTagChars = ".[],|=+()`~!@#$%^&*\\\"/?<>{}"
	restrictedAliasErr = "Alias '%s' either contains restricted characters or is the same as a restricted tag needed for normal operation"
	restrictedTagErr   = "Tag '%s' either contains restricted characters or is the same as a restricted tag needed for normal operation"
)

var (
	restrictedTags = map[string]struct{}{
		diveTag:          {},
		existsTag:        {},
		structOnlyTag:    {},
		omitempty:        {},
		skipInjectionTag: {},
		utf8HexComma:     {},
		utf8Pipe:         {},
		noStructLevelTag: {},
	}
)

func (inj *Inject) ParseTagValue(v string) (reflect.Kind, interface{}) {
	if strings.HasPrefix(v, "fn@") {
		parts := strings.Split(v, "@")
		if len(parts) != 2 {
			panic(fmt.Sprintf("tag value %s not valid", v))
		}

		if fn, ok := inj.funcs[parts[1]]; ok {
			return reflect.Func, fn
		}

		panic(fmt.Sprintf("fn %s not found", parts[1]))
	}

	if strings.HasPrefix(v, "var@") {
		parts := strings.Split(v, "@")
		if len(parts) != 2 {
			panic(fmt.Sprintf("tag value %s not valid", v))
		}

		if v, ok := inj.variables[parts[1]]; ok {
			return reflect.ValueOf(v).Kind(), v
		}

		panic(fmt.Sprintf("fn %s not found", parts[1]))
	}
	return reflect.String, v
}

func (inj *Inject) AddFunc(tag string, fn interface{}) {
	if reflect.TypeOf(fn).Kind() != reflect.Func {
		panic("fn is not a func")
	}
	inj.funcs[tag] = fn
}

func (inj *Inject) AddVar(tag string, val interface{}) {
	inj.variables[tag] = val
}

func (inj *Inject) ExtractType(current reflect.Value) (reflect.Value, reflect.Kind) {

	val, k, _ := inj.extractTypeInternal(current, false)
	return val, k
}

// 指针转非指针, interface转实际类型
func (inj *Inject) extractTypeInternal(current reflect.Value, nullable bool) (reflect.Value, reflect.Kind, bool) {

	switch current.Kind() {
	case reflect.Ptr:

		nullable = true

		if current.IsNil() {
			return current, reflect.Ptr, nullable
		}

		return inj.extractTypeInternal(current.Elem(), nullable)

	case reflect.Interface:

		nullable = true

		if current.IsNil() {
			return current, reflect.Interface, nullable
		}

		return inj.extractTypeInternal(current.Elem(), nullable)

	case reflect.Invalid:
		return current, reflect.Invalid, nullable

	default:
		return current, current.Kind(), nullable
	}
}

// 获取结构体的字段值
func (inj *Inject) GetStructFieldOK(current reflect.Value, namespace string) (reflect.Value, reflect.Kind, bool) {

	current, kind := inj.ExtractType(current)

	if kind == reflect.Invalid {
		return current, kind, false
	}

	if namespace == blank {
		return current, kind, true
	}

	switch kind {

	case reflect.Ptr, reflect.Interface:

		return current, kind, false

	case reflect.Struct:

		typ := current.Type()
		fld := namespace
		ns := namespace

		if typ != timeType && typ != timePtrType {

			idx := strings.Index(namespace, namespaceSeparator)

			if idx != -1 {
				fld = namespace[:idx]
				ns = namespace[idx+1:]
			} else {
				ns = blank
			}

			bracketIdx := strings.Index(fld, leftBracket)
			if bracketIdx != -1 {
				fld = fld[:bracketIdx]

				ns = namespace[bracketIdx:]
			}

			current = current.FieldByName(fld)

			return inj.GetStructFieldOK(current, ns)
		}

	case reflect.Array, reflect.Slice:
		idx := strings.Index(namespace, leftBracket)
		idx2 := strings.Index(namespace, rightBracket)

		arrIdx, _ := strconv.Atoi(namespace[idx+1 : idx2])

		if arrIdx >= current.Len() {
			return current, kind, false
		}

		startIdx := idx2 + 1

		if startIdx < len(namespace) {
			if namespace[startIdx:startIdx+1] == namespaceSeparator {
				startIdx++
			}
		}

		return inj.GetStructFieldOK(current.Index(arrIdx), namespace[startIdx:])

	case reflect.Map:
		idx := strings.Index(namespace, leftBracket) + 1
		idx2 := strings.Index(namespace, rightBracket)

		endIdx := idx2

		if endIdx+1 < len(namespace) {
			if namespace[endIdx+1:endIdx+2] == namespaceSeparator {
				endIdx++
			}
		}

		key := namespace[idx:idx2]

		switch current.Type().Key().Kind() {
		case reflect.Int:
			i, _ := strconv.Atoi(key)
			return inj.GetStructFieldOK(current.MapIndex(reflect.ValueOf(i)), namespace[endIdx+1:])
		case reflect.Int8:
			i, _ := strconv.ParseInt(key, 10, 8)
			return inj.GetStructFieldOK(current.MapIndex(reflect.ValueOf(int8(i))), namespace[endIdx+1:])
		case reflect.Int16:
			i, _ := strconv.ParseInt(key, 10, 16)
			return inj.GetStructFieldOK(current.MapIndex(reflect.ValueOf(int16(i))), namespace[endIdx+1:])
		case reflect.Int32:
			i, _ := strconv.ParseInt(key, 10, 32)
			return inj.GetStructFieldOK(current.MapIndex(reflect.ValueOf(int32(i))), namespace[endIdx+1:])
		case reflect.Int64:
			i, _ := strconv.ParseInt(key, 10, 64)
			return inj.GetStructFieldOK(current.MapIndex(reflect.ValueOf(i)), namespace[endIdx+1:])
		case reflect.Uint:
			i, _ := strconv.ParseUint(key, 10, 0)
			return inj.GetStructFieldOK(current.MapIndex(reflect.ValueOf(uint(i))), namespace[endIdx+1:])
		case reflect.Uint8:
			i, _ := strconv.ParseUint(key, 10, 8)
			return inj.GetStructFieldOK(current.MapIndex(reflect.ValueOf(uint8(i))), namespace[endIdx+1:])
		case reflect.Uint16:
			i, _ := strconv.ParseUint(key, 10, 16)
			return inj.GetStructFieldOK(current.MapIndex(reflect.ValueOf(uint16(i))), namespace[endIdx+1:])
		case reflect.Uint32:
			i, _ := strconv.ParseUint(key, 10, 32)
			return inj.GetStructFieldOK(current.MapIndex(reflect.ValueOf(uint32(i))), namespace[endIdx+1:])
		case reflect.Uint64:
			i, _ := strconv.ParseUint(key, 10, 64)
			return inj.GetStructFieldOK(current.MapIndex(reflect.ValueOf(i)), namespace[endIdx+1:])
		case reflect.Float32:
			f, _ := strconv.ParseFloat(key, 32)
			return inj.GetStructFieldOK(current.MapIndex(reflect.ValueOf(float32(f))), namespace[endIdx+1:])
		case reflect.Float64:
			f, _ := strconv.ParseFloat(key, 64)
			return inj.GetStructFieldOK(current.MapIndex(reflect.ValueOf(f)), namespace[endIdx+1:])
		case reflect.Bool:
			b, _ := strconv.ParseBool(key)
			return inj.GetStructFieldOK(current.MapIndex(reflect.ValueOf(b)), namespace[endIdx+1:])

		// reflect.Type = string
		default:
			return inj.GetStructFieldOK(current.MapIndex(reflect.ValueOf(key)), namespace[endIdx+1:])
		}
	}

	// if got here there was more namespace, cannot go any deeper
	panic("Invalid field namespace")
}

func asInt(param string) int64 {

	i, err := strconv.ParseInt(param, 0, 64)
	panicIf(err)

	return i
}

func asUint(param string) uint64 {

	i, err := strconv.ParseUint(param, 0, 64)
	panicIf(err)

	return i
}

func asFloat(param string) float64 {

	i, err := strconv.ParseFloat(param, 64)
	panicIf(err)

	return i
}

func panicIf(err error) {
	if err != nil {
		panic(err.Error())
	}
}

func GetFieldByNestedName(v reflect.Value, nestedName string) reflect.Value {
	if nestedName == "" {
		return v
	}
	names := strings.Split(nestedName, ".")
	for _, name := range names {
		v = getFieldByName(v, name)
		if !v.IsValid() {
			// 如果某一级字段不存在，直接返回无效的 reflect.Value
			return v
		}
	}
	return v
}

// getFieldByName 通过字段名获取 reflect.Value
func getFieldByName(v reflect.Value, fieldName string) reflect.Value {
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		// 如果不是结构体，返回无效的 reflect.Value
		return reflect.Value{}
	}

	field := v.FieldByName(fieldName)
	return field
}
