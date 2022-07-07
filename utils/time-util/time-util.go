package time_util

import (
	"log"
	"math"
	"sync"
	"time"
)

// TickerFunc call func periodically, the difference from the std impl is reset logic
type TickerFunc struct {
	mu              sync.Mutex
	lastTriggerTime time.Time
	interval        time.Duration
	timer           *time.Timer
}

func (t *TickerFunc) init(fn func()) {
	interval := t.interval
	if interval <= 0 {
		interval = math.MaxInt64
	}
	t.timer = time.AfterFunc(interval, func() {
		var exec bool
		t.mu.Lock()
		defer func() {
			t.mu.Unlock()
			if exec {
				fn()
			}
		}()
		if t.interval <= 0 {
			log.Println(t.interval)
			return
		}
		now := time.Now()
		realInterval := now.Sub(t.lastTriggerTime)
		if needWait := t.interval - realInterval; needWait > 0 {
			t.timer.Reset(needWait)
			return
		}
		t.lastTriggerTime = now
		t.timer.Reset(t.interval)
		exec = true
	})
}

// Reset the interval duration. The next tick time is calculated by lastTriggerTime, if duration is shorter
// than current sleeping time, the next tick time will arrive immediately
func (t *TickerFunc) Reset(interval time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.interval == interval {
		return
	}
	t.interval = interval
	t.timer.Stop()
	if interval > 0 {
		needWait := interval - time.Now().Sub(t.lastTriggerTime)
		if needWait <= 0 {
			needWait = 1
		}
		t.timer.Reset(needWait)
	}
}

func NewTickerFunc(interval time.Duration, fn func()) *TickerFunc {
	t := &TickerFunc{
		lastTriggerTime: time.Now(),
		interval:        interval,
	}
	t.init(fn)
	return t
}
