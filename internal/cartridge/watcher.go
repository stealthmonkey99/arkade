// SPDX-License-Identifier: GPL-2.0-or-later

package cartridge

import (
	"sync"
	"time"
)

// DefaultPollInterval is how often Watcher rescans when no interval is given.
const DefaultPollInterval = 3 * time.Second

// Watcher polls a Registry off the engine goroutine. It polls rather than
// using fsnotify because a not-yet-mounted USB path can only be discovered by
// re-listing.
type Watcher struct {
	reg      *Registry
	interval time.Duration

	// OnHotAdd is called from the watcher goroutine with the slugs each
	// background rescan discovered at fresh paths, even ones already
	// registered from the library. The initial Start scan never fires it.
	// Set before Start.
	OnHotAdd func(slugs []string)

	mu      sync.Mutex
	started bool
	quit    chan struct{}
	wg      sync.WaitGroup
}

// NewWatcher builds a Watcher over reg. interval <= 0 uses
// DefaultPollInterval.
func NewWatcher(reg *Registry, interval time.Duration) *Watcher {
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	return &Watcher{reg: reg, interval: interval}
}

// Start scans once synchronously, so boot-time cartridges are registered
// before it returns, then begins the poll loop. A second Start is a no-op.
func (w *Watcher) Start() {
	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		return
	}
	w.started = true
	w.quit = make(chan struct{})
	w.mu.Unlock()

	w.reg.Scan()

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			select {
			case <-w.quit:
				return
			case <-ticker.C:
				if res := w.reg.Scan(); len(res.Discovered) > 0 && w.OnHotAdd != nil {
					w.OnHotAdd(res.Discovered)
				}
			}
		}
	}()
}

// Stop ends the poll loop and waits for it. Safe without Start and when
// repeated.
func (w *Watcher) Stop() {
	w.mu.Lock()
	if !w.started {
		w.mu.Unlock()
		return
	}
	w.started = false
	quit := w.quit
	w.mu.Unlock()

	close(quit)
	w.wg.Wait()
}
