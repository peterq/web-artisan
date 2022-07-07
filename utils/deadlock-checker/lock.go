package deadlock_checker

import (
	"os"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

func New() sync.Locker {
	l := new(sync.Mutex)
	return &mu{
		id:     atomic.AddInt64(&_gid, 1),
		locked: false,
		lock:   l,
		cond:   sync.NewCond(l),
	}
}

var _gid int64

type mu struct {
	lock   *sync.Mutex
	cond   *sync.Cond
	locked bool
	cnt    int
	id     int64
}

func (m *mu) Lock() {
	m.lock.Lock()
	defer m.lock.Unlock()
	for m.locked {
		m.cond.Wait()
	}

	m.cnt++
	term := m.cnt
	stack := debug.Stack()
	m.locked = true
	//log.Println("lock", m.id)
	go func() {
		time.Sleep(time.Second)
		m.lock.Lock()
		defer m.lock.Unlock()
		if term == m.cnt && m.locked {
			os.Stderr.Write(stack)
		}
	}()

}

func (m *mu) Unlock() {
	m.lock.Lock()
	defer m.lock.Unlock()
	if !m.locked {
		panic("unlock unlocked mutex")
	}
	m.locked = false
	//log.Println("unlock", m.id)
	m.cond.Signal()
}
