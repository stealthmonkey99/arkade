// SPDX-License-Identifier: GPL-2.0-or-later

package controller

import (
	"embed"
	"fmt"
	"path"
	"sort"

	"github.com/stealthmonkey99/arkade/assetfile"
)

//go:embed profiles/*.arkadehid.json
var builtinProfiles embed.FS

// Profiles are the supported devices, compiled from profiles/*.arkadehid.json.
var Profiles []Profile

// The built-in profiles by name; tests refer to them directly.
var (
	Speedlink   Profile
	DragonRise  Profile
	Buffalo     Profile
	Sonicon     Profile
	DualShock   Profile
	DualShockBT Profile
)

func init() {
	byFile := map[string]*Profile{
		"speedlink.arkadehid.json":    &Speedlink,
		"dragonrise.arkadehid.json":   &DragonRise,
		"buffalo.arkadehid.json":      &Buffalo,
		"sonicon.arkadehid.json":      &Sonicon,
		"dualshock.arkadehid.json":    &DualShock,
		"dualshock-bt.arkadehid.json": &DualShockBT,
	}
	entries, err := builtinProfiles.ReadDir("profiles")
	if err != nil {
		panic(fmt.Sprintf("controller: reading embedded profiles: %v", err))
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		data, err := builtinProfiles.ReadFile(path.Join("profiles", name))
		if err != nil {
			panic(fmt.Sprintf("controller: reading %s: %v", name, err))
		}
		env, err := assetfile.ReadBytes(name, data)
		if err != nil {
			panic(fmt.Sprintf("controller: %s: %v", name, err))
		}
		p, err := ProfileFromEnvelope(env)
		if err != nil {
			panic(fmt.Sprintf("controller: %s: %v", name, err))
		}
		slot, ok := byFile[name]
		if !ok {
			panic(fmt.Sprintf("controller: %s has no package var; add one beside the others", name))
		}
		*slot = p
	}
	// Order is match order, so it is explicit rather than the directory's.
	Profiles = []Profile{Speedlink, DragonRise, Buffalo, Sonicon, DualShock, DualShockBT}
	for _, p := range Profiles {
		if p.Spec == nil {
			panic("controller: a built-in profile failed to compile")
		}
	}
}
