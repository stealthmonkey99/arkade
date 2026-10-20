// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"fmt"
	"time"
)

// RequestAttractPreview asks the engine to show slug's attract slot now.
// Safe from any goroutine; applied at the top of the next tick, latest request
// wins. The running app is suspended, so a button press resumes it.
//
// An empty slug is ignored. A slug with nothing to show is not rejected here;
// the attract app falls back to its normal rotation and logs.
func (rt *Runtime) RequestAttractPreview(slug string) { rt.requests.attractPreview.Set(slug) }

// enterAttractPreview is enterStandbyNow with a focus slug. With standby
// disabled the request is dropped. Re-previewing while attract is up keeps
// the app parked by the first takeover and only reloads attract.
func (rt *Runtime) enterAttractPreview(now time.Time, slug string) {
	if !rt.standby.enabled() {
		rt.Log("warn", fmt.Sprintf("attract: preview %q ignored: standby is disabled", slug))
		return
	}
	if _, err := Lookup(slug); err != nil {
		rt.Log("warn", fmt.Sprintf("attract: preview %q ignored: %v", slug, err))
		return
	}
	rt.notes.attractFocus = slug
	rt.savers.pin = "" // the operator wants a game's slot, not a forced screensaver
	rt.standby.forceAttract(now)
	rt.Log("info", fmt.Sprintf("attract: preview %s", slug))
	if rt.curCtx == nil || rt.curCtx.Manifest.Type != TypeAttract {
		rt.suspendCurrent(now)
	}
	rt.loadAttract()
}

// TakeAttractFocus returns the slug the attract app should pin its showcase to
// and clears it, so a preview is one-shot. It returns "" for a normal load.
// Engine goroutine only; called from the attract app's Init.
func (rt *Runtime) TakeAttractFocus() string { return rt.notes.takeAttractFocus() }
