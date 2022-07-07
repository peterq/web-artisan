package event

import (
	"context"
	"sync"
)

type Event interface {
}

type listener[T Event] struct {
	once bool
	ctx  context.Context
	fn   func(T)
}

type Emitter[T Event] struct {
	listeners []listener[T]
	mu        sync.Mutex // guards
}

func NewEmitter[T Event]() *Emitter[T] {
	return &Emitter[T]{
		listeners: nil,
		mu:        sync.Mutex{},
	}
}

func Listen[T Event](ctx context.Context, once bool, e *Emitter[T], fn func(T)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.listeners = append(e.listeners, listener[T]{
		once: once,
		ctx:  ctx,
		fn:   fn,
	})
}

func Emit[T Event](e *Emitter[T], event T) {
	e.mu.Lock()
	defer e.mu.Unlock()
	list := e.listeners
	for i := 0; i < len(list); {
		l := list[i]
		var expire = l.ctx != nil && l.ctx.Err() != nil
		if !expire {
			go l.fn(event)
		}
		if expire || l.once {
			list = append(list[:i], list[i+1:]...)
		} else {
			i++
		}
	}
	e.listeners = list
}
