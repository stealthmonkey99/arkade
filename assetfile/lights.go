// SPDX-License-Identifier: GPL-2.0-or-later

package assetfile

// Lights caps. A sequence is untrusted input, so every count a host acts on
// is bounded here and both hosts refuse the same file.
const (
	// MaxLightFrames caps one sequence's frame list.
	MaxLightFrames = 256

	// LightButtonCount is how many lamps one seat has.
	LightButtonCount = 6

	// MaxLightMask is every lamp on one seat lit.
	MaxLightMask = 1<<LightButtonCount - 1

	// MaxLightFadeMs caps a frame's cross-fade, the same ceiling as a hold.
	MaxLightFadeMs = maxHoldMs
)

// LightButtonNames is the lamp bit order: bit i names LightButtonNames[i].
// It copies internal/hardware.LightButtons because assetfile may not import
// internal/*; a test in internal/cartridge/wasm pins the two together.
var LightButtonNames = []string{"joya", "joyb", "joyc", "joyd", "joyaux1", "joyaux2"}

// LightLamps is one frame's per-seat masks. A nil field leaves that seat's
// lamps untouched, which differs from a mask of 0 (explicitly dark).
type LightLamps struct {
	P1 *int `json:"p1,omitempty"`
	P2 *int `json:"p2,omitempty"`
}

// Mask returns the mask this frame states for a seat (0 = p1, 1 = p2) and
// whether it states one at all.
func (l LightLamps) Mask(seat int) (int, bool) {
	p := l.P1
	if seat == 1 {
		p = l.P2
	}
	if p == nil {
		return 0, false
	}
	return *p, true
}

// LightFrame is one step of a sequence: the lamps lit on each seat it names
// and how long they are held. A mask is that seat's whole lamp state while
// the frame shows. A frame naming no seat is refused.
type LightFrame struct {
	Lamps  LightLamps `json:"lamps"`
	HoldMs int        `json:"holdMs"`
	// FadeMs is the cross-fade into this frame's masks, counted inside
	// HoldMs; 0 is a hard cut.
	FadeMs int `json:"fadeMs,omitempty"`
}

// NewLightLamps builds a LightLamps; pass nil for a seat the frame does not
// name.
func NewLightLamps(p1, p2 *int) LightLamps { return LightLamps{P1: p1, P2: p2} }

// LightMaskPtr returns a pointer to mask, for literal frames.
func LightMaskPtr(mask int) *int { return &mask }
