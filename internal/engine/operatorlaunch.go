// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"fmt"
	"time"
)

// RequestLaunchNow asks the engine to boot slug immediately, whatever is on
// screen, including a dark panel. Safe from any goroutine; applied at the top
// of the next tick, latest request wins. Anything parked behind the
// screensaver is torn down.
//
// An empty slug is ignored; an unknown one is logged when drained.
func (rt *Runtime) RequestLaunchNow(slug string) { rt.requests.operatorLaunch.Set(slug) }

// enterOperatorLaunch leaves standby the way a person would (which also
// clears a hard dark), discards anything parked, and loads.
func (rt *Runtime) enterOperatorLaunch(now time.Time, slug string) {
	m, err := Lookup(slug)
	if err != nil {
		rt.Log("warn", fmt.Sprintf("launch: operator request %q ignored: %v", slug, err))
		return
	}
	rt.leaveStandby(now, discardParked)
	rt.Log("info", fmt.Sprintf("launch: operator request %s", slug))
	rt.requestLoad(m)
}
