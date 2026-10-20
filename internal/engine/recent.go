// SPDX-License-Identifier: GPL-2.0-or-later

package engine

// ShellRecents is the part of the shell's settings the engine reads back. The
// shell embeds it in its own settings, so the two share one field.
type ShellRecents struct {
	// MRU is the most recently launched slugs, newest first.
	MRU []string `json:"mru"`
}

// RecentSlugs reports the shell's most-recently-launched game slugs, most
// recent first. It returns nil for no shell, no settings yet, or a read error.
func (rt *Runtime) RecentSlugs() []string {
	shell, ok := lookupType(TypeShell)
	if !ok {
		return nil
	}
	var s ShellRecents
	if err := rt.Store.Read(shell.Slug, "", &s); err != nil {
		return nil
	}
	return s.MRU
}
