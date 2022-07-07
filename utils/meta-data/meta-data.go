package meta_data

import "sync"

type MetaData struct {
	data     sync.Map
	initLock sync.Map
}

type initItem struct {
	initOnce sync.Once
	makeFn   func() any
	value    interface{}
}

func (m *MetaData) LoadMeta(k any) (any, bool) {
	v, ok := m.data.Load(k)
	if ok {
		return m.unwrapInit(v), ok
	}
	return v, ok
}

func (m *MetaData) unwrapInit(v any) any {
	if init, ok := v.(*initItem); ok {
		init.initOnce.Do(func() {
			init.value = init.makeFn()
		})
		return init.value
	}
	return v
}

func (m *MetaData) StoreMeta(k, v any) {
	m.data.Store(k, v)
}

func (m *MetaData) LoadOrMakeAndStoreMeta(k any, makeFn func() any) (any, bool) {
	v, ok := m.data.LoadOrStore(k, &initItem{
		initOnce: sync.Once{},
		makeFn:   makeFn,
		value:    nil,
	})
	return m.unwrapInit(v), ok
}
