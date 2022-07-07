package injector

import (
	"fmt"
	"log"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
)

type tagType uint8

const (
	typeDefault tagType = iota
	typeOmitEmpty
	typeNoStructLevel
	typeStructOnly
	typeDive
	typeOr
	typeExists
)

type structCache struct {
	lock sync.Mutex
	m    atomic.Value // map[reflect.Type]*cStruct
}

func (sc *structCache) Get(key reflect.Type) (c *cStruct, found bool) {
	c, found = sc.m.Load().(map[reflect.Type]*cStruct)[key]
	return
}

func (sc *structCache) Set(key reflect.Type, value *cStruct) {

	m := sc.m.Load().(map[reflect.Type]*cStruct)

	nm := make(map[reflect.Type]*cStruct, len(m)+1)
	for k, v := range m {
		nm[k] = v
	}
	nm[key] = value
	sc.m.Store(nm)
}

type tagCache struct {
	lock sync.Mutex
	m    atomic.Value // map[string]*cTag
}

func (tc *tagCache) Get(key string) (c *cTag, found bool) {
	c, found = tc.m.Load().(map[string]*cTag)[key]
	return
}

func (tc *tagCache) Set(key string, value *cTag) {

	m := tc.m.Load().(map[string]*cTag)

	nm := make(map[string]*cTag, len(m)+1)
	for k, v := range m {
		nm[k] = v
	}
	nm[key] = value
	tc.m.Store(nm)
}

type cStruct struct {
	Name   string
	fields map[int]*cField
	order  []int
	fn     StructLevelFunc
}

type cField struct {
	Idx     int
	Name    string
	AltName string
	cTags   *cTag
}

type cTag struct {
	tag            string
	aliasTag       string
	actualAliasTag string
	param          string
	hasAlias       bool
	typeof         tagType
	hasTag         bool
	fn             TagFn
	next           *cTag
	Exported       *ExportedCTag
}

type ExportedCTag struct {
	cTag *cTag
	Info *TagInfo
}

type TagInfo struct {
	Data interface{}
}

func (inj *Inject) extractStructCache(current reflect.Value, sName string, ctxData map[string]interface{}) *cStruct {

	inj.structCache.lock.Lock()
	defer inj.structCache.lock.Unlock()

	typ := current.Type()

	// 避免重复遍历
	cs, ok := inj.structCache.Get(typ)
	if ok {
		return cs
	}

	cs = &cStruct{Name: sName, fields: make(map[int]*cField), fn: inj.structLevelFuncs[typ]}

	numFields := current.NumField()

	var ctag *cTag
	var fld reflect.StructField
	var tag string
	var customName string

	for i := 0; i < numFields; i++ {

		fld = typ.Field(i)

		if !fld.Anonymous && fld.PkgPath != blank {
			continue
		}

		tag = fld.Tag.Get(inj.tagName)

		if tag == skipInjectionTag {
			continue
		}

		customName = fld.Name

		if inj.fieldNameTag != blank {

			name := strings.SplitN(fld.Tag.Get(inj.fieldNameTag), ",", 2)[0]

			// dash check is for json "-" (aka skipInjectionTag) means don't output in json
			if name != "" && name != skipInjectionTag {
				customName = name
			}
		}

		// NOTE: cannot use shared tag cache, because tags may be equal, but things like alias may be different
		// and so only struct level caching can be used instead of combined with Field tag caching

		if len(tag) > 0 {
			ctag, _ = inj.parseFieldTagsRecursive(tag, fld.Name, blank, false, current, current.Field(i), ctxData)
		} else {
			// even if field doesn't have injections need cTag for traversing to potential inner/nested
			// elements of the field.
			ctag = new(cTag)
		}

		cs.fields[i] = &cField{Idx: i, Name: fld.Name, AltName: customName, cTags: ctag}
		cs.order = append(cs.order, i)
	}

	inj.structCache.Set(typ, cs)

	return cs
}

func (inj *Inject) parseFieldTagsRecursive(tag string, fieldName string, alias string, hasAlias bool, currentStruct reflect.Value, field reflect.Value, ctxData map[string]interface{}) (firstCtag *cTag, current *cTag) {

	var t string
	noAlias := len(alias) == 0
	tags := strings.Split(tag, tagSeparator)

	for i := 0; i < len(tags); i++ {

		t = tags[i]

		if noAlias {
			alias = t
		}

		if inj.hasAliasInjectors {
			// check map for alias and process new tags, otherwise process as usual
			if tagsVal, found := inj.aliasInjectors[t]; found {

				if i == 0 {
					firstCtag, current = inj.parseFieldTagsRecursive(tagsVal, fieldName, t, true, currentStruct, field, ctxData)
				} else {
					next, curr := inj.parseFieldTagsRecursive(tagsVal, fieldName, t, true, currentStruct, field, ctxData)
					current.next, current = next, curr

				}

				continue
			}
		}

		if i == 0 {
			current = &cTag{aliasTag: alias, hasAlias: hasAlias, hasTag: true}
			firstCtag = current
		} else {
			current.next = &cTag{aliasTag: alias, hasAlias: hasAlias, hasTag: true}
			current = current.next
		}

		switch t {

		case diveTag:
			current.typeof = typeDive
			continue

		case omitempty:
			current.typeof = typeOmitEmpty
			continue

		case structOnlyTag:
			current.typeof = typeStructOnly
			continue

		case noStructLevelTag:
			current.typeof = typeNoStructLevel
			continue

		case existsTag:
			current.typeof = typeExists
			continue

		default:

			// if a pipe character is needed within the param you must use the utf8Pipe representation "0x7C"
			orVals := strings.Split(t, orSeparator)

			for j := 0; j < len(orVals); j++ {

				vals := strings.SplitN(orVals[j], tagKeySeparator, 2)

				if noAlias {
					alias = vals[0]
					current.aliasTag = alias
				} else {
					current.actualAliasTag = t
				}

				if j > 0 {
					current.next = &cTag{aliasTag: alias, actualAliasTag: current.actualAliasTag, hasAlias: hasAlias, hasTag: true}
					current = current.next
				}

				current.tag = vals[0]
				if len(current.tag) == 0 {
					panic(strings.TrimSpace(fmt.Sprintf(invalidInjection, fieldName)))
				}

				var tagParams = blank
				if len(vals) > 1 {
					tagParams = vals[1]
				}

				if initFn, ok := inj.initFuncs[current.tag]; !ok {
					panic(strings.TrimSpace(fmt.Sprintf(undefinedInjection, fieldName)))
				} else {
					current.fn = initFn(&TagFnState{
						Inj:           inj,
						CurrentStruct: currentStruct,
						Field:         field,
						Param:         tagParams,
						CtxData:       ctxData,
					})
				}

				if len(orVals) > 1 {
					current.typeof = typeOr
				}

				if len(vals) > 1 {
					current.param = strings.Replace(strings.Replace(vals[1], utf8HexComma, ",", -1), utf8Pipe, "|", -1)
				}
			}
		}
	}

	return
}

func (inj *Inject) CacheForStruct(s interface{}) {
	inj.CacheForStructWithCtxData(s, nil)
}

func (inj *Inject) CacheForStructWithCtxData(s interface{}, ctxData map[string]interface{}) {
	current := reflect.ValueOf(s)
	if current.Kind() == reflect.Ptr && !current.IsNil() {
		current = current.Elem()
	}

	if current.Kind() != reflect.Struct && current.Kind() != reflect.Interface {
		log.Println(current.Type(), current.IsNil())
		panic("value passed for injection is not a struct")
	}

	if !current.CanAddr() {
		panic("the value passed for injection must be able to be obtained with Addr")
	}

	inj.extractStructCache(current, current.Type().Name(), ctxData)
}
