// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"fmt"
	"sync"
	"time"
)

// latest is a one-deep slot written from any goroutine and taken on the
// engine goroutine; a second Set before the Take replaces the first.
type latest[T any] struct {
	mu  sync.Mutex
	v   T
	set bool
}

func (l *latest[T]) Set(v T) {
	l.mu.Lock()
	l.v, l.set = v, true
	l.mu.Unlock()
}

func (l *latest[T]) Take() (v T, ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	v, ok = l.v, l.set
	var zero T
	l.v, l.set = zero, false
	return v, ok
}

// request is one kind of off-goroutine request, applied at the top of a tick.
type request[T any] struct {
	latest[T]
	// anyState requests are honored whatever is on screen. The rest are
	// honored only from the live shell menu and dropped otherwise.
	anyState bool
	apply    func(rt *Runtime, now time.Time, v T)
}

func (r *request[T]) drain(rt *Runtime, now time.Time) {
	v, ok := r.Take()
	if !ok || (!r.anyState && !rt.atShellMenu()) {
		return
	}
	r.apply(rt, now, v)
}

// requests holds every request kind. Drained in field order.
type requests struct {
	// A hot-mounted cartridge only launches from the menu, never mid-game.
	hotMountLaunch request[string]
	// Operator dashboard actions.
	attractPreview  request[string]
	screensaverShow request[string]
	operatorLaunch  request[string]
	// The asset and peripherals labs; nil cancels.
	labPreview request[*LabPreview]
	lightShow  request[*LightShow]
	powerOff   request[struct{}]
}

func (q *requests) init() {
	q.hotMountLaunch.apply = slugOnly((*Runtime).applyHotMountLaunch)
	q.attractPreview.anyState, q.attractPreview.apply = true, slugOnly((*Runtime).enterAttractPreview)
	q.screensaverShow.anyState, q.screensaverShow.apply = true, slugOnly((*Runtime).enterScreensaverShow)
	q.operatorLaunch.anyState, q.operatorLaunch.apply = true, slugOnly((*Runtime).enterOperatorLaunch)
	q.labPreview.anyState, q.labPreview.apply = true, (*Runtime).applyLabPreview
	q.lightShow.anyState, q.lightShow.apply = true, (*Runtime).applyLightShow
	q.powerOff.anyState, q.powerOff.apply = true, (*Runtime).applyPowerOff
}

// drainRequests applies every request raised off the engine goroutine since
// the last tick.
func (rt *Runtime) drainRequests(now time.Time) {
	q := &rt.requests
	q.hotMountLaunch.drain(rt, now)
	q.attractPreview.drain(rt, now)
	q.screensaverShow.drain(rt, now)
	q.operatorLaunch.drain(rt, now)
	q.labPreview.drain(rt, now)
	q.lightShow.drain(rt, now)
	q.powerOff.drain(rt, now)
}

// slugOnly drops an empty slug, which a caller may Set to withdraw a request.
func slugOnly(apply func(*Runtime, time.Time, string)) func(*Runtime, time.Time, string) {
	return func(rt *Runtime, now time.Time, slug string) {
		if slug != "" {
			apply(rt, now, slug)
		}
	}
}

func (rt *Runtime) atShellMenu() bool {
	return rt.curCtx != nil && rt.curCtx.Manifest.Type == TypeShell && rt.standby.state == StateActive
}

func (rt *Runtime) applyHotMountLaunch(_ time.Time, slug string) {
	if m, err := Lookup(slug); err == nil {
		rt.Log("info", fmt.Sprintf("cartridge hot-mounted, launching %s", slug))
		rt.requestLoad(m)
	}
}
