package sync

import "sync"

type Guard struct {
	mu      sync.Mutex
	running bool
}

func (g *Guard) TryStart() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.running {
		return false
	}
	g.running = true
	return true
}

func (g *Guard) Finish() {
	g.mu.Lock()
	g.running = false
	g.mu.Unlock()
}
