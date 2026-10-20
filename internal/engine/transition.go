// SPDX-License-Identifier: GPL-2.0-or-later

package engine

// transitionKind is what the top of the next tick does to the loaded app,
// in ascending priority: a request replaces a pending transition of the same
// or lower priority and is ignored under a higher one.
type transitionKind int

const (
	transitionNone transitionKind = iota
	transitionDark
	transitionUnload
	transitionLoad
	transitionPowerOff // outranks everything, including a pending load
)

// transition is deferred to the top of a tick because the code asking for
// it is usually running inside the app it would unload.
type transition struct {
	kind     transitionKind
	manifest Manifest // transitionLoad
	fade     bool     // transitionDark: play the goodnight sequence
}

func (rt *Runtime) setTransition(t transition) {
	cur := rt.transition
	if t.kind < cur.kind {
		return
	}
	if t.kind == transitionDark && cur.kind == transitionDark {
		t.fade = t.fade || cur.fade
	}
	rt.transition = t
}

func (rt *Runtime) requestLoad(m Manifest) {
	rt.setTransition(transition{kind: transitionLoad, manifest: m})
}

// requestUnload leaves the current app: reset=true reboots, otherwise the
// shell loads next (or the engine quits when the app ExitsOnQuit).
func (rt *Runtime) requestUnload(reset bool) {
	rt.dropScreensaverPinOnExit()
	if reset {
		if boot, ok := lookupType(TypeBoot); ok {
			rt.requestLoad(boot)
			return
		}
	}
	rt.setTransition(transition{kind: transitionUnload})
}

// applyTransition reports stop when the tick has nothing further to do.
func (rt *Runtime) applyTransition() (stop bool, err error) {
	t := rt.transition
	rt.transition = transition{}
	switch t.kind {
	case transitionPowerOff:
		rt.startPowerOff()
	case transitionLoad:
		return false, rt.loadApp(t.manifest)
	case transitionUnload:
		return rt.unloadToNext()
	case transitionDark:
		rt.enterDark(t.fade)
	}
	return false, nil
}

// unloadToNext unloads the current app and loads its successor: nothing (quit)
// for ExitsOnQuit apps, attract when boot finishes untouched, else the shell.
func (rt *Runtime) unloadToNext() (stop bool, err error) {
	exits := rt.curCtx != nil && rt.curCtx.Manifest.ExitsOnQuit
	toAttract := rt.bootHandsOffToAttract()
	rt.unloadCurrent()
	switch {
	case exits:
		rt.halt.request()
		return true, nil
	case toAttract:
		rt.standby.forceAttract(rt.frames.tickStart)
		rt.Log("info", "standby: boot finished untouched, starting attract")
		rt.notes.bootSettle = true
		rt.notes.quiet = true
		rt.loadStandby(rt.frames.tickStart)
		// Load now, not next tick: boot's layers are gone, so a tick would
		// paint one black frame.
		if next := rt.transition; next.kind == transitionLoad {
			rt.transition = transition{}
			return false, rt.loadApp(next.manifest)
		}
		return false, nil
	default:
		shell, ok := lookupType(TypeShell)
		if !ok {
			rt.halt.request()
			return true, nil
		}
		return false, rt.loadApp(shell)
	}
}
