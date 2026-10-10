package singleflight

import "sync"

type call struct {
	wg  sync.WaitGroup
	val any
	err error
}

type Group struct {
	mu sync.Mutex
	m  map[string]*call
}

func (g *Group) Do(key string, fn func() (any, error)) (any, error, bool) {
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[string]*call)
	}
	if c, ok := g.m[key]; ok {
		g.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err, true
	}
	c := new(call)
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	defer func() {
		c.wg.Done()

		g.mu.Lock()
		delete(g.m, key)
		g.mu.Unlock()
	}()

	c.val, c.err = fn()

	return c.val, c.err, false
}
