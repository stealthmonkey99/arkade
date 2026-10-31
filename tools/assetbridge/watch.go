// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const pollInterval = 300 * time.Millisecond

type change struct {
	Cart string `json:"cart"`
	Path string `json:"path"`
}

type stamp struct {
	size  int64
	mtime int64
}

// hub polls the carts' assets/ trees only while someone is subscribed.
type hub struct {
	carts []cart

	mu   sync.Mutex
	subs map[chan change]struct{}
	snap map[change]stamp
	stop chan struct{}
}

func newHub(carts []cart) *hub {
	return &hub{carts: carts, subs: map[chan change]struct{}{}}
}

func (h *hub) subscribe() chan change {
	ch := make(chan change, 64)
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.subs) == 0 {
		h.snap = h.scan()
		h.stop = make(chan struct{})
		go h.poll(h.stop)
	}
	h.subs[ch] = struct{}{}
	return ch
}

func (h *hub) unsubscribe(ch chan change) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.subs, ch)
	if len(h.subs) == 0 && h.stop != nil {
		close(h.stop)
		h.stop = nil
	}
}

func (h *hub) poll(stop chan struct{}) {
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
		}
		next := h.scan()
		h.mu.Lock()
		if h.stop != stop {
			h.mu.Unlock()
			return
		}
		for _, c := range diff(h.snap, next) {
			for ch := range h.subs {
				select {
				case ch <- c:
				default: // a stalled client misses events rather than stalling the rest
				}
			}
		}
		h.snap = next
		h.mu.Unlock()
	}
}

func (h *hub) scan() map[change]stamp {
	out := map[change]stamp{}
	for _, c := range h.carts {
		filepath.WalkDir(filepath.Join(c.dir, "assets"), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if strings.HasPrefix(d.Name(), ".") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if !d.Type().IsRegular() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			out[change{Cart: c.Slug, Path: relTo(c, p)}] = stamp{size: info.Size(), mtime: info.ModTime().UnixNano()}
			return nil
		})
	}
	return out
}

func diff(prev, next map[change]stamp) []change {
	var out []change
	for k, s := range next {
		if old, ok := prev[k]; !ok || old != s {
			out = append(out, k)
		}
	}
	for k := range prev {
		if _, ok := next[k]; !ok {
			out = append(out, k)
		}
	}
	return out
}
