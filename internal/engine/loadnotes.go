// SPDX-License-Identifier: GPL-2.0-or-later

package engine

// loadNotes are one-shot messages the engine leaves for the next load and
// the app that comes up in it. Each is consumed by its first read.
type loadNotes struct {
	// quiet suppresses the loading bar, for loads standby starts that nobody
	// asked for.
	quiet bool
	// bootSettle marks the attract load that follows the boot splash.
	bootSettle bool
	// attractFocus pins the next attract load to one showcase slot.
	attractFocus string
	// settingsOpen names the screensaver the green button opened for its
	// settings; only that app's load reads it as true.
	settingsOpen string
}

func (n *loadNotes) takeQuiet() bool {
	v := n.quiet
	n.quiet = false
	return v
}

func (n *loadNotes) takeBootSettle() bool {
	v := n.bootSettle
	n.bootSettle = false
	return v
}

func (n *loadNotes) takeAttractFocus() string {
	v := n.attractFocus
	n.attractFocus = ""
	return v
}

func (n *loadNotes) takeSettingsOpen(slug string) bool {
	opened := slug != "" && n.settingsOpen == slug
	n.settingsOpen = ""
	return opened
}
