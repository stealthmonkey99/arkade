// SPDX-License-Identifier: GPL-2.0-or-later

package display

import (
	"testing"

	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// TestBlendOverMatchesMerge: ui.BlendOver must be byte-identical to
// display.Merge across the alpha range, or widgets would render differently in
// a cartridge and on the cabinet.
func TestBlendOverMatchesMerge(t *testing.T) {
	samples := []Pixel{
		{0, 0, 0, 0}, {0, 0, 0, 255}, {255, 255, 255, 255},
		{255, 0, 0, 255}, {0, 255, 0, 128}, {0, 0, 255, 70},
		{40, 160, 80, 255}, {255, 255, 0, 100}, {123, 45, 200, 200},
		{10, 20, 30, 1}, {200, 100, 50, 254},
	}
	for _, dst := range samples {
		for _, src := range samples {
			want := Merge(dst, src)
			gotUI := ui.BlendOver(ToUI(dst), ToUI(src))
			got := FromUI(gotUI)
			if got != want {
				t.Errorf("Merge(%v, %v)=%v but BlendOver=%v", dst, src, want, got)
			}
		}
	}
}

func TestSpriteCanvasMatchesMerge(t *testing.T) {
	cv, sprite := NewSpriteCanvas(3, 3)
	// Start non-transparent so the blend actually composites.
	sprite[1][1] = Pixel{0, 0, 0, 255}
	src := ui.Color{R: 255, G: 255, B: 255, A: 128}
	cv.Blend(1, 1, src)
	want := Merge(Pixel{0, 0, 0, 255}, Pixel{255, 255, 255, 128})
	if sprite[1][1] != want {
		t.Errorf("SpriteCanvas.Blend = %v, want Merge result %v", sprite[1][1], want)
	}
	// Out-of-range is a silent no-op.
	cv.Blend(-1, 0, ui.White)
	cv.Blend(0, 99, ui.White)
}

// TestMergeOverTransparentIsExact: the general formula rounds a translucent
// colour down, so a transparent destination takes src unchanged instead.
func TestMergeOverTransparentIsExact(t *testing.T) {
	for _, c := range []Pixel{{3, 3, 3, 3}, {140, 115, 191, 128}, {255, 0, 7, 1}, {9, 9, 9, 254}} {
		for _, dst := range []Pixel{Transparent, {200, 10, 10, 0}} {
			if got := Merge(dst, c); got != c {
				t.Errorf("Merge(%v, %v) = %v, want src exactly", dst, c, got)
			}
		}
	}
}

// TestStampWritesWhereDrawComposites: a translucent stroke over a translucent
// fill composites once drawn but keeps the stroke colour once stamped.
func TestStampWritesWhereDrawComposites(t *testing.T) {
	fill, stroke := Pixel{3, 3, 3, 3}, Pixel{200, 100, 50, 128}
	rect := ui.Rect{W: 3, H: 3, Fill: ToUI(fill), Stroke: ToUI(stroke), StrokeW: 1}
	if got := Stamp(3, 3, rect)[0][0]; got != stroke {
		t.Errorf("Stamp wrote %v, want %v", got, stroke)
	}
	if got, want := Draw(3, 3, rect)[0][0], Merge(fill, stroke); got != want {
		t.Errorf("Draw wrote %v, want the blended %v", got, want)
	}
	if got := Stamp(3, 3, rect)[1][1]; got != fill {
		t.Errorf("Stamp interior %v, want %v", got, fill)
	}
	stamp := stampCanvas{NewMatrix(1, 1, Transparent)}
	stamp.Blend(5, 5, ui.White) // out of range is a silent no-op
}

func TestTextSpriteIsTheFontAtNaturalSize(t *testing.T) {
	lavender := Pixel{140, 115, 191, 128}
	s := TextSprite("HI", lavender)
	font := ui.FontLED{}
	if len(s) != font.Height(1) || len(s[0]) != font.TextWidth("HI", 1) {
		t.Fatalf("sprite is %dx%d, want %dx%d", len(s[0]), len(s), font.TextWidth("HI", 1), font.Height(1))
	}
	lit := 0
	for _, row := range s {
		for _, p := range row {
			switch p {
			case lavender:
				lit++
			case Transparent:
			default:
				t.Fatalf("pixel %v is neither the label colour nor transparent", p)
			}
		}
	}
	if lit == 0 {
		t.Fatal("HI lit no pixels")
	}
	if empty := TextSprite("", lavender); len(empty) != font.Height(1) || len(empty[0]) != 0 {
		t.Errorf("empty text should be %d zero-width rows, got %d rows of %d", font.Height(1), len(empty), len(empty[0]))
	}
}
