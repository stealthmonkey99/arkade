// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"encoding/json"
	"fmt"

	"github.com/stealthmonkey99/arkade/internal/display"
)

// tsManifest is the manifest.json shape that packages embed.
type tsManifest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        [][]struct {
		Red   uint8 `json:"red"`
		Green uint8 `json:"green"`
		Blue  uint8 `json:"blue"`
		Alpha uint8 `json:"alpha"`
	} `json:"icon"`
}

// IconFromManifestJSON extracts the icon sprite from a manifest.json. It
// panics on malformed data: manifests are compiled in, so it fails at init.
func IconFromManifestJSON(raw []byte) display.Sprite {
	var m tsManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		panic(fmt.Sprintf("bad embedded manifest.json: %v", err))
	}
	icon := make(display.Sprite, len(m.Icon))
	for y, row := range m.Icon {
		out := make([]display.Pixel, len(row))
		for x, p := range row {
			out[x] = display.Pixel{R: p.Red, G: p.Green, B: p.Blue, A: p.Alpha}
		}
		icon[y] = out
	}
	return icon
}

// DescriptionFromManifestJSON extracts the description field.
func DescriptionFromManifestJSON(raw []byte) string {
	var m tsManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		panic(fmt.Sprintf("bad embedded manifest.json: %v", err))
	}
	return m.Description
}
