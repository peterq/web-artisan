package injector

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
)

const (
	utf8HexComma           = "0x2C"
	utf8Pipe               = "0x7C"
	tagSeparator           = ";"
	orSeparator            = "|"
	tagKeySeparator        = "="
	structOnlyTag          = "structonly"
	noStructLevelTag       = "nostructlevel"
	omitempty              = "omitempty"
	skipInjectionTag       = "-"
	diveTag                = "dive"
	existsTag              = "exists"
	fieldErrMsg            = "Key: '%s' Error:Field injection for '%s' failed on the '%s' tag: %s"
	arrayIndexFieldName    = "%s" + leftBracket + "%d" + rightBracket
	mapIndexFieldName      = "%s" + leftBracket + "%v" + rightBracket
	invalidInjection       = "Invalid injection tag on field %s"
	undefinedInjection     = "Undefined injection function on field %s"
	injectorNotInitialized = "Injector instance not initialized"
	fieldNameRequired      = "Field Name Required"
	tagRequired            = "Tag Required"
)

var (
	timeType      = reflect.TypeOf(time.Time{})
	timePtrType   = reflect.TypeOf(&time.Time{})
	defaultCField = new(cField)
)

// 结构体回调时的参数
type StructLevel struct {
	TopStruct     reflect.Value
	CurrentStruct reflect.Value
	errPrefix     string
	nsPrefix      string
	v             *Inject
}

// tag 处理器
type TagFn func(state *TagFnState) (reflect.Value, error)

type TagFnState struct {
	Inj           *Inject
	CurrentStruct reflect.Value
	Field         reflect.Value
	Param         string
	CtxData       map[string]interface{}
}

func (s *TagFnState) Get(key string) interface{} {
	if s.CtxData != nil {
		return s.CtxData[key]
	}
	return nil
}

// 注入器结构体
type Inject struct {
	tagName             string
	fieldNameTag        string
	initFuncs           map[string]TagInitFunc
	structLevelFuncs    map[reflect.Type]StructLevelFunc
	aliasInjectors      map[string]string
	hasCustomFuncs      bool
	hasAliasInjectors   bool
	hasStructLevelFuncs bool
	structCache         *structCache
	funcs               map[string]interface{}
	variables           map[string]interface{}

	byResolver []byResolver
}

func (inj *Inject) initCheck() {
	if inj == nil {
		panic(injectorNotInitialized)
	}
}

// 注入器配置
type Config struct {
	TagName      string
	FieldNameTag string
}

// tag 初始化函数
type TagInitFunc func(state *TagFnState) TagFn

// 结构体回调函数
type StructLevelFunc func(v *Inject, structLevel *StructLevel)

// 字段注入错误
type FieldError struct {
	FieldNamespace string
	NameNamespace  string
	Field          string
	Name           string
	Tag            string
	ActualTag      string
	Kind           reflect.Kind
	Type           reflect.Type
	Param          string
	Value          interface{}
	InjectError    error
}

func (err FieldError) Error() string {
	return fmt.Sprintf(fieldErrMsg, err.FieldNamespace, err.Field, err.Tag, err.InjectError)
}

// 创建注入器实例
func New(config *Config) *Inject {

	tc := new(tagCache)
	tc.m.Store(make(map[string]*cTag))

	sc := new(structCache)
	sc.m.Store(make(map[reflect.Type]*cStruct))

	inj := &Inject{
		tagName:      config.TagName,
		fieldNameTag: config.FieldNameTag,
		structCache:  sc}

	inj.funcs = map[string]interface{}{}
	inj.variables = map[string]interface{}{}

	if len(inj.aliasInjectors) == 0 {
		// must copy alias injectors for separate injections to be used in each injector instance
		inj.aliasInjectors = map[string]string{}
		for k, val := range bakedInAliasInjectors {
			inj.RegisterAliasInjection(k, val)
		}
	}

	if len(inj.initFuncs) == 0 {
		// must copy injectors for separate injections to be used in each instance
		inj.initFuncs = map[string]TagInitFunc{}
		for k, val := range bakedInInjectorsInit {
			inj.RegisterInjection(k, val)
		}
	}

	return inj
}

// 注册结构体注入
func (inj *Inject) RegisterStructInjection(fn StructLevelFunc, types ...interface{}) {
	inj.initCheck()

	if inj.structLevelFuncs == nil {
		inj.structLevelFuncs = map[reflect.Type]StructLevelFunc{}
	}

	for _, t := range types {
		inj.structLevelFuncs[reflect.TypeOf(t)] = fn
	}

	inj.hasStructLevelFuncs = true
}

// 注册一个tag处理器
func (inj *Inject) RegisterInjection(key string, fn TagInitFunc) {
	inj.initCheck()

	if key == blank {
		panic("Function Key cannot be empty")
	}

	if fn == nil {
		panic("Function cannot be empty")
	}

	_, ok := restrictedTags[key]

	if ok || strings.ContainsAny(key, restrictedTagChars) {
		panic(fmt.Sprintf(restrictedTagErr, key))
	}

	inj.initFuncs[key] = fn
}

// tag 别名注册, 把较长的tag(组合)定义成一个别名, 以减少tag长度
func (inj *Inject) RegisterAliasInjection(alias, tags string) {
	inj.initCheck()

	_, ok := restrictedTags[alias]

	if ok || strings.ContainsAny(alias, restrictedTagChars) {
		panic(fmt.Sprintf(restrictedAliasErr, alias))
	}

	inj.aliasInjectors[alias] = tags
	inj.hasAliasInjectors = true
}

// 只注入指定部分字段
func (inj *Inject) StructPartial(current interface{}, fields ...string) error {
	return inj.StructPartialWithCtxData(current, fields, nil)
}
func (inj *Inject) StructPartialWithCtxData(current interface{}, fields []string, ctxData map[string]interface{}) error {
	inj.initCheck()

	sv, _ := inj.ExtractType(reflect.ValueOf(current))
	name := sv.Type().Name()
	m := map[string]struct{}{}

	if fields != nil {
		for _, k := range fields {

			flds := strings.Split(k, namespaceSeparator)
			if len(flds) > 0 {

				key := name + namespaceSeparator
				for _, s := range flds {

					idx := strings.Index(s, leftBracket)

					if idx != -1 {
						for idx != -1 {
							key += s[:idx]
							m[key] = struct{}{}

							idx2 := strings.Index(s, rightBracket)
							idx2++
							key += s[idx:idx2]
							m[key] = struct{}{}
							s = s[idx2:]
							idx = strings.Index(s, leftBracket)
						}
					} else {

						key += s
						m[key] = struct{}{}
					}

					key += namespaceSeparator
				}
			}
		}
	}

	return inj.injectStruct(sv, sv, sv, blank, blank, true, len(m) != 0, false, m, ctxData)
}

// 注入指定部分之外的字段
func (inj *Inject) StructExcept(current interface{}, fields ...string) error {
	return inj.StructExceptWithCtxData(current, fields, nil)
}

func (inj *Inject) StructExceptWithCtxData(current interface{}, fields []string, ctxData map[string]interface{}) error {
	inj.initCheck()

	sv, _ := inj.ExtractType(reflect.ValueOf(current))
	name := sv.Type().Name()
	m := map[string]struct{}{}

	for _, key := range fields {
		m[name+namespaceSeparator+key] = struct{}{}
	}

	err := inj.injectStruct(sv, sv, sv, blank, blank, true, len(m) != 0, true, m, ctxData)
	if err != nil {
		return err
	}
	return nil
}

// 结构体注入
func (inj *Inject) Struct(current interface{}) error {
	return inj.StructWithCtxData(current, nil)
}

func (inj *Inject) StructWithCtxData(current interface{}, ctxData map[string]interface{}) error {
	inj.initCheck()
	sv := reflect.ValueOf(current)
	err := inj.injectStruct(sv, sv, sv, blank, blank, true, false, false, nil, ctxData)
	if err != nil {
		return err
	}
	return nil
}

// 结构体注入入口
func (inj *Inject) injectStruct(topStruct reflect.Value, currentStruct reflect.Value, current reflect.Value, errPrefix string, nsPrefix string, useStructName bool, partial bool, exclude bool, includeExclude map[string]struct{}, ctxData map[string]interface{}) *FieldError {

	if current.Kind() == reflect.Ptr && !current.IsNil() {
		current = current.Elem()
	}

	if current.Kind() != reflect.Struct && current.Kind() != reflect.Interface {
		panic("value passed for injection is not a struct")
	}

	if !current.CanAddr() {
		panic("the value passed for injection must be able to be obtained with Addr")
	}

	return inj.traverseStruct(topStruct, currentStruct, current, errPrefix, nsPrefix, useStructName, partial, exclude, includeExclude, nil, nil, ctxData)
}

// 遍历结构体所有字段, 并传入traverseField
func (inj *Inject) traverseStruct(topStruct reflect.Value, currentStruct reflect.Value, current reflect.Value, errPrefix string, nsPrefix string, useStructName bool, partial bool, exclude bool, includeExclude map[string]struct{}, cs *cStruct, ct *cTag, ctxData map[string]interface{}) *FieldError {
	var ok bool
	first := len(nsPrefix) == 0
	typ := current.Type()

	cs, ok = inj.structCache.Get(typ)
	if !ok {
		cs = inj.extractStructCache(current, typ.Name(), map[string]interface{}{})
	}

	if useStructName {
		errPrefix += cs.Name + namespaceSeparator

		if len(inj.fieldNameTag) != 0 {
			nsPrefix += cs.Name + namespaceSeparator
		}
	}

	// structonly tag present don't tranverseFields
	// but must still check and run below struct level injection
	// if present
	if first || ct == nil || ct.typeof != typeStructOnly {
		for _, idx := range cs.order {
			f := cs.fields[idx]

			if partial {

				_, ok = includeExclude[errPrefix+f.Name]

				if (ok && exclude) || (!ok && !exclude) {
					continue
				}
			}
			e := inj.traverseField(topStruct, currentStruct, current.Field(f.Idx), errPrefix, nsPrefix, partial, exclude, includeExclude, cs, f, f.cTags, ctxData)
			if e != nil {
				return e
			}

		}
	}

	// 结构体回调
	if cs.fn != nil {
		cs.fn(inj, &StructLevel{v: inj, TopStruct: topStruct, CurrentStruct: current, errPrefix: errPrefix, nsPrefix: nsPrefix})
	}
	return nil
}

// 遍历某字段的tag, 执行相应的注入函数
func (inj *Inject) traverseField(topStruct reflect.Value, currentStruct reflect.Value, current reflect.Value, errPrefix string, nsPrefix string, partial bool, exclude bool, includeExclude map[string]struct{}, cs *cStruct, cf *cField, ct *cTag, ctxData map[string]interface{}) *FieldError {

	var newVal reflect.Value = current
	var tagFnErr error

	var set = ct != nil && ct.fn != nil
	defer func() {
		if set {
			if tagFnErr == nil {
				if !newVal.IsValid() {
					panic(fmt.Sprintf("tagFnErr is nil, but newVal is invalid, %#v", ct))
				}
				current.Set(newVal)
			}
		}
	}()

	current, kind, nullable := inj.extractTypeInternal(current, false)
	var typ reflect.Type

	switch kind {
	case reflect.Ptr, reflect.Invalid:

		if kind == reflect.Ptr && ct != nil {
			break
		}

		if ct == nil {
			return nil
		}

		if ct.typeof == typeOmitEmpty {
			return nil
		}

		if ct.hasTag {

			ns := errPrefix + cf.Name

			if kind == reflect.Invalid {
				return &FieldError{
					FieldNamespace: ns,
					NameNamespace:  nsPrefix + cf.AltName,
					Name:           cf.AltName,
					Field:          cf.Name,
					Tag:            ct.aliasTag,
					ActualTag:      ct.tag,
					Param:          ct.param,
					Kind:           kind,
					InjectError:    errors.New("kind is invalid"),
				}
			}

			return &FieldError{
				FieldNamespace: ns,
				NameNamespace:  nsPrefix + cf.AltName,
				Name:           cf.AltName,
				Field:          cf.Name,
				Tag:            ct.aliasTag,
				ActualTag:      ct.tag,
				Param:          ct.param,
				Value:          current.Interface(),
				Kind:           kind,
				Type:           current.Type(),
				InjectError:    errors.New("kind is invalid"),
			}
		}

	case reflect.Struct:
		typ := current.Type()
		ct := ct
		if typ != timeType {

			if ct != nil {
				ct = ct.next
			}

			if ct != nil && ct.typeof == typeNoStructLevel {
				return nil
			}

			nestedStructError := inj.traverseStruct(topStruct, current, current, errPrefix+cf.Name+namespaceSeparator, nsPrefix+cf.AltName+namespaceSeparator, false, partial, exclude, includeExclude, cs, ct, ctxData)
			if nestedStructError != nil {
				return nestedStructError
			}
		}
	}

	if ct == nil || !ct.hasTag {
		return nil
	}

	typ = current.Type()

OUTER:
	for {
		if ct == nil {
			return nil
		}

		switch ct.typeof {

		case typeExists:
			ct = ct.next
			continue

		case typeOmitEmpty:

			if !nullable && !HasValue(current, typ, kind) {
				return nil
			}

			ct = ct.next
			continue

		case typeDive:

			ct = ct.next

			// traverse slice or map here
			// or panic ;)
			switch kind {
			case reflect.Slice, reflect.Array:

				for i := 0; i < current.Len(); i++ {
					e := inj.traverseField(topStruct, currentStruct, current.Index(i), errPrefix, nsPrefix, partial, exclude, includeExclude, cs, &cField{Name: fmt.Sprintf(arrayIndexFieldName, cf.Name, i), AltName: fmt.Sprintf(arrayIndexFieldName, cf.AltName, i)}, ct, ctxData)
					if e != nil {
						return e
					}
				}

			case reflect.Map:
				for _, key := range current.MapKeys() {
					e := inj.traverseField(topStruct, currentStruct, current.MapIndex(key), errPrefix, nsPrefix, partial, exclude, includeExclude, cs, &cField{Name: fmt.Sprintf(mapIndexFieldName, cf.Name, key.Interface()), AltName: fmt.Sprintf(mapIndexFieldName, cf.AltName, key.Interface())}, ct, ctxData)
					if e != nil {
						return e
					}
				}

			default:
				// throw error, if not a slice or map then should not have gotten here
				// bad dive tag
				panic("dive error! can't dive on a non slice or map")
			}

			return nil

		case typeOr:

			errTag := blank

			for {
				newVal, tagFnErr = ct.fn(&TagFnState{
					Inj:           inj,
					CurrentStruct: currentStruct,
					Field:         newVal,
					Param:         ct.param,
					CtxData:       ctxData,
				})
				if tagFnErr == nil {

					// drain rest of the 'or' values, then continue or leave
					for {

						ct = ct.next

						if ct == nil {
							return nil
						}

						if ct.typeof != typeOr {
							continue OUTER
						}
					}
				}

				errTag += orSeparator + ct.tag

				if ct.next == nil {
					// if we get here, no valid 'or' value and no more tags

					ns := errPrefix + cf.Name

					if ct.hasAlias {
						return &FieldError{
							FieldNamespace: ns,
							NameNamespace:  nsPrefix + cf.AltName,
							Name:           cf.AltName,
							Field:          cf.Name,
							Tag:            ct.aliasTag,
							ActualTag:      ct.actualAliasTag,
							Value:          current.Interface(),
							Type:           typ,
							Kind:           kind,
							InjectError:    tagFnErr,
						}
					} else {
						return &FieldError{
							FieldNamespace: ns,
							NameNamespace:  nsPrefix + cf.AltName,
							Name:           cf.AltName,
							Field:          cf.Name,
							Tag:            errTag[1:],
							ActualTag:      errTag[1:],
							Value:          current.Interface(),
							Type:           typ,
							Kind:           kind,
							InjectError:    tagFnErr,
						}
					}

					return nil
				}

				ct = ct.next
			}

		default:
			newVal, tagFnErr = ct.fn(&TagFnState{
				Inj:           inj,
				CurrentStruct: currentStruct,
				Field:         newVal,
				Param:         ct.param,
				CtxData:       ctxData,
			})

			if tagFnErr != nil {

				ns := errPrefix + cf.Name

				return &FieldError{
					FieldNamespace: ns,
					NameNamespace:  nsPrefix + cf.AltName,
					Name:           cf.AltName,
					Field:          cf.Name,
					Tag:            ct.aliasTag,
					ActualTag:      ct.tag,
					Value:          current.Interface(),
					Param:          ct.param,
					Type:           typ,
					Kind:           kind,
					InjectError:    tagFnErr,
				}

				return nil

			}

			ct = ct.next
		}
	}
}
