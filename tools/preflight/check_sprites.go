// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const ruleAlpha = "docs/GAME_AUTHORING_AI.md 'The alpha rule': A==255 becomes the sprite's ui.Mask (its collidable body); anything below 255 draws but never collides"

// nearOpaqueLo is the bottom of the near-opaque band. Findings there warn
// because a deliberate choice looks the same as a slipped opacity slider.
const nearOpaqueLo = 240

// nearOpaqueShare is the fraction of visible pixels at one near-opaque alpha
// that triggers a finding; stray anti-aliased edge pixels stay below it.
const nearOpaqueShare = 0.20

// blessedDecorativeAlpha is the documented alpha for a solid-looking but
// non-collidable sprite. Exactly 254 passes with a note; other near-opaque
// values warn.
const blessedDecorativeAlpha = 254

// checkSpriteAlpha reports PNG alpha channels that will not mean what the
// artist meant. Collision-mask findings apply only under assets/sprites/,
// where ui.OpaqueMask turns A==255 into the hitbox; other PNGs are only decoded.
func checkSpriteAlpha(c *Cart, r *Result) {
	pngs, err := c.AssetFiles(".png")
	if err != nil {
		r.fail("", Loc{}, "walking assets/: %v", err)
		return
	}
	if len(pngs) == 0 {
		r.skip("no PNGs under %s/assets", c.Rel(c.Dir))
		return
	}

	sprites := 0
	for _, path := range pngs {
		where := c.At(path)
		img, err := decodePNG(path)
		if err != nil {
			r.fail("", where, "undecodable PNG: %v", err)
			continue
		}
		st := alphaStats(img)
		isSprite := isSpriteAsset(path)
		if isSprite {
			sprites++
		}

		if st.visible == 0 {
			r.fail(ruleAlpha, where, "every pixel is A=0 — this asset draws nothing")
			continue
		}

		// No A=255 pixel means an empty collision mask. Soft art peaking well
		// below opaque is intended; peaking at 240-254 is likely a slipped slider.
		if isSprite && st.opaque == 0 {
			switch {
			case st.maxAlpha == blessedDecorativeAlpha:
				r.info(where, "%dx%d peaks at A=%d, the documented decorative alpha (a near-opaque A=254 object is decorative on purpose) — no collision mask, by canon", st.w, st.h, st.maxAlpha)
			case st.maxAlpha >= nearOpaqueLo:
				r.warn(ruleAlpha, where, "%dx%d peaks at A=%d and never reaches 255 — its collision mask (ui.OpaqueMask) is EMPTY, so nothing will ever hit this sprite. Deliberate (A=%d is the canon value for that), or an opacity slider left below 100%%?",
					st.w, st.h, st.maxAlpha, blessedDecorativeAlpha)
			default:
				r.info(where, "%dx%d is fully decorative (peaks at A=%d, no collision mask) — decorative-overdraw canon", st.w, st.h, st.maxAlpha)
			}
			continue
		}

		for _, b := range st.nearOpaqueBands(nearOpaqueShare) {
			if isSprite && b.alpha == blessedDecorativeAlpha {
				r.info(where, "A=%d covers %d px (%.0f%% of the visible artwork) — the documented decorative alpha, so that area is non-collidable on purpose",
					b.alpha, b.count, 100*float64(b.count)/float64(st.visible))
				continue
			}
			if isSprite {
				r.warn(ruleAlpha, where, "A=%d covers %d px (%.0f%% of the visible artwork) — that whole area is NON-collidable; deliberate decorative overdraw (A=%d is the canon value for that), or an opacity slider left below 100%%?",
					b.alpha, b.count, 100*float64(b.count)/float64(st.visible), blessedDecorativeAlpha)
			} else {
				r.info(where, "A=%d covers %d px (%.0f%% of the visible artwork) — near-opaque but not opaque (not a sprite, so no mask is derived from it)",
					b.alpha, b.count, 100*float64(b.count)/float64(st.visible))
			}
		}
	}

	scope := fmt.Sprintf("%d PNG(s)", len(pngs))
	if sprites > 0 {
		scope += fmt.Sprintf(" (%d under assets/sprites, where alpha is the hitbox)", sprites)
	}
	if fails, warns := r.counts(); fails+warns > 0 {
		r.summarize("%s, %d violation(s), %d to look at", scope, fails, warns)
	} else {
		r.summarize("%s, alpha channels clean", scope)
	}
}

// isSpriteAsset reports whether a PNG is under assets/sprites/.
func isSpriteAsset(path string) bool {
	return strings.Contains(filepath.ToSlash(path), "/assets/sprites/")
}

func decodePNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

// alphaHistogram is the per-file alpha census.
type alphaHistogram struct {
	w, h     int
	visible  int // pixels with A > 0
	opaque   int // pixels with A == 255
	maxAlpha int
	counts   [256]int
}

type band struct {
	alpha int
	count int
}

// alphaStats builds the census from the image's 8-bit alpha.
func alphaStats(img image.Image) alphaHistogram {
	b := img.Bounds()
	st := alphaHistogram{w: b.Dx(), h: b.Dy()}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a16 := img.At(x, y).RGBA()
			a := int(a16 >> 8)
			st.counts[a]++
			if a > 0 {
				st.visible++
			}
			if a == 255 {
				st.opaque++
			}
			if a > st.maxAlpha {
				st.maxAlpha = a
			}
		}
	}
	return st
}

// nearOpaqueBands returns every alpha in [nearOpaqueLo,255) carrying at
// least share of the visible pixels, worst first.
func (st alphaHistogram) nearOpaqueBands(share float64) []band {
	if st.visible == 0 {
		return nil
	}
	var out []band
	for a := nearOpaqueLo; a < 255; a++ {
		if n := st.counts[a]; n > 0 && float64(n)/float64(st.visible) >= share {
			out = append(out, band{alpha: a, count: n})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].count > out[j].count })
	return out
}
