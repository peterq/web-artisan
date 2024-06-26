package state_machine

import (
	"context"
	"encoding/json"
	"github.com/peterq/web-artisan/utils/cond_chan"
	"gopkg.in/yaml.v3"
	"sync"
)

func NewStateMachine[TState any](state *TState) *StateMachine[TState] {
	return &StateMachine[TState]{
		state:   state,
		term:    1,
		yamlBin: nil,
		jsonBin: nil,
		//mu:      deadlock_checker.New(),
		mu:         sync.Mutex{},
		changeCond: cond_chan.NewCond(),
		waitingCnt: 0,
	}
}

type StateMachine[T any] struct {
	state    *T
	term     int64
	yamlNode *yaml.Node
	yamlBin  []byte
	jsonBin  []byte
	//mu       sync.Locker
	mu         sync.Mutex // guards
	changeCond cond_chan.Cond
	waitingCnt int
}

func (m *StateMachine[T]) Update(fn func(*T) bool) {
	Update[T](m, fn)
}

func (m *StateMachine[T]) Read(fn func(*T)) T {
	return Read[T, T](m, func(state *T) T {
		if fn != nil {
			fn(state)
		}
		return *state
	})
}

func Update[T any](m *StateMachine[T], fn func(*T) bool) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if fn(m.state) {
		m.term++
		m.yamlNode = nil
		m.yamlBin = nil
		m.jsonBin = nil
		if m.waitingCnt == 1 {
			m.changeCond.Signal()
		} else if m.waitingCnt > 1 {
			m.changeCond.Broadcast()
		}
	}
	return m.term
}

func Read[T any, U any](m *StateMachine[T], fn func(state *T) U) U {
	m.mu.Lock()
	defer m.mu.Unlock()
	return fn(m.state)
}

func ReadTerm[T any, U any](m *StateMachine[T], fn func(state *T) U) (U, int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return fn(m.state), m.term
}

func ReadTermChange[T any](ctx context.Context, m *StateMachine[T], term int64) (*T, int64) {
	d, t := ReadUntilOkCtx1(m, ctx, func(state *T, t int64) (*T, bool) {
		return state, t > term
	})
	return d, t
}

func ReadUntilOk[T any, U any](m *StateMachine[T], fn func(state *T) (U, bool)) U {
	ret, _ := ReadUntilOkCtx(m, context.Background(), fn)
	return ret
}

func ReadUntilOkCtx1[T any, U any](m *StateMachine[T], ctx context.Context, fn func(state *T, term int64) (U, bool)) (U, int64) {
	if ctx == nil {
		ctx = context.Background()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.waitingCnt++
	defer func() { m.waitingCnt-- }()
	var ok bool
	var data U
	term := m.term - 1
	for {
		if m.term > term {
			data, ok = fn(m.state, m.term)
			term = m.term
		}
		if ok {
			return data, term
		}
		cond := m.changeCond.Wait()
		m.mu.Unlock()
		select {
		case <-cond:
			m.mu.Lock()
			continue
		case <-ctx.Done():
			m.mu.Lock()
			return data, -1
		}
	}
}
func ReadUntilOkCtx[T any, U any](m *StateMachine[T], ctx context.Context, fn func(state *T) (U, bool)) (U, bool) {
	d, t := ReadUntilOkCtx1(m, ctx, func(state *T, term int64) (U, bool) {
		return fn(state)
	})
	return d, t > -1
}

type contentWithTerm struct {
	term    int64
	content []byte
}

func WaitMarshaledContentChange[T any](m *StateMachine[T], ctx context.Context, term int64, isJson bool) ([]byte, int64) {
	if ctx == nil {
		ctx = context.Background()
	}
	var content *[]byte
	if isJson {
		content = &m.jsonBin
	} else {
		content = &m.yamlBin
	}
	var marshalFunc = json.Marshal
	if !isJson {
		marshalFunc = yaml.Marshal
	}
	var r, ok = ReadUntilOkCtx(m, ctx, func(state *T) (contentWithTerm, bool) {
		if m.term <= term {
			return contentWithTerm{}, false
		}
		if *content == nil {
			*content, _ = marshalFunc(m.state)
		}

		return contentWithTerm{
			term:    m.term,
			content: *content,
		}, true
	})
	if !ok {
		return nil, -1
	}
	return r.content, r.term
}

func WaitYamlNodeChange[T any](m *StateMachine[T], ctx context.Context, term int64) (*yaml.Node, int64) {
	var r, ok = ReadUntilOkCtx(m, ctx, func(state *T) (*yaml.Node, bool) {
		if m.term <= term {
			return m.yamlNode, false
		}
		if m.yamlNode == nil {
			var yamlNode yaml.Node
			_ = yamlNode.Encode(state)
			m.yamlNode = &yamlNode
		}
		term = m.term
		return m.yamlNode, true
	})
	if !ok {
		return nil, -1
	}
	return r, term
}
