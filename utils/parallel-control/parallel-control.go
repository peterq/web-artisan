package parallel_control

import (
	"context"
	"github.com/peterq/web-artisan/utils/cond_chan"
	"sync"
	"time"
)

type Controller interface {
	FetchToken(ctx context.Context) (int64, error)
	ReleaseToken(seq int64)
}

// NewController create a Controller: every interval can fetch a token, but there only can be parallel number of token in the same time
func NewController(ctx context.Context, parallel int, interval time.Duration) Controller {
	var c *controller
	c = &controller{
		ctx:             ctx,
		parallel:        parallel,
		interval:        int64(interval / time.Millisecond),
		mu:              sync.Mutex{},
		cond:            cond_chan.NewCond(),
		lastFetchTime:   0,
		currentParallel: 0,
		waiting:         0,
		lastSeq:         0,
		pending:         make(map[int64]struct{}),
	}
	return c
}

type controller struct {
	ctx      context.Context
	parallel int
	interval int64 // milliseconds

	mu              sync.Mutex
	cond            cond_chan.Cond
	lastFetchTime   int64 // milliseconds
	currentParallel int
	waiting         int
	lastSeq         int64
	pending         map[int64]struct{}
}

func (c *controller) FetchToken(ctx context.Context) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var now int64
	// loop until it can fetch token (wait parallel decrease and interval pass)
	for {
		now = time.Now().UnixNano() / int64(time.Millisecond)
		sleep := c.interval - (now - c.lastFetchTime)
		if sleep > 0 {
			c.mu.Unlock() // unlock before sleep
			var err error
			select {
			case <-time.After(time.Duration(sleep) * time.Millisecond):
			case <-c.ctx.Done():
				err = c.ctx.Err()
			case <-ctx.Done():
				err = ctx.Err()
			}
			c.mu.Lock() // lock after sleep
			if err != nil {
				return 0, err
			}
			continue // sleep finish, check again
		}

		// it can fetch token
		if c.currentParallel < c.parallel {
			break
		}

		// it can't fetch token, wait another token release
		signal := c.cond.Wait()
		c.waiting++
		c.mu.Unlock() // unlock before wait
		var err error
		select {
		case <-signal:
		case <-c.ctx.Done():
			err = c.ctx.Err()
		case <-ctx.Done():
			err = ctx.Err()
		}
		c.mu.Lock() // lock after wait
		c.waiting--
		if err != nil {
			return 0, err
		}
	}

	// loop break, means it can fetch token
	c.currentParallel++
	c.lastFetchTime = now
	c.lastSeq++
	c.pending[c.lastSeq] = struct{}{}
	return c.lastSeq, nil

}

func (c *controller) ReleaseToken(seq int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.pending[seq]; !ok {
		panic("invalid parallel controller token seq, maybe release twice")
	}
	delete(c.pending, seq)
	c.currentParallel--
	if c.waiting > 0 {
		c.cond.Signal()
	}
}
