// SPDX-License-Identifier: GPL-2.0-or-later

package element

import (
	"fmt"
	"math"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
)

// LoadingBar is the engine's two-tone bevelled progress bar; in infinite
// mode it cycles on the frame clock. ui.LoadingBar is a flat frame with an
// untouched track, so this one keeps its own raster.
type LoadingBar struct {
	Base
	infinite   bool
	minV, maxV float64
	value      float64
	barW, barH int
}

// LoadingBarOptions configures a LoadingBar. Zero values default:
// min 0, max 100, width 20, height 2.
type LoadingBarOptions struct {
	Options
	Infinite bool
	Min      float64
	Max      *float64
	Start    float64
	BarW     int
	BarH     int
}

// NewLoadingBar builds a loading bar.
func NewLoadingBar(opts LoadingBarOptions) (*LoadingBar, error) {
	l := &LoadingBar{infinite: opts.Infinite, minV: opts.Min, maxV: 100, barW: 20, barH: 2}
	if opts.Max != nil {
		l.maxV = *opts.Max
	}
	if opts.BarW != 0 {
		l.barW = opts.BarW
	}
	if opts.BarH != 0 {
		l.barH = opts.BarH
	}
	if l.maxV <= l.minV {
		return nil, fmt.Errorf("invalid LoadingBarOptions: min must be less than max")
	}
	l.value = math.Min(l.maxV, math.Max(l.minV, opts.Start))
	l.InitBase(opts.Options)
	l.Impl = l.renderImpl
	return l, nil
}

func (l *LoadingBar) renderImpl(stats clock.FrameTime) display.Sprite {
	currVal := l.value
	if l.infinite {
		currVal = math.Mod(stats.DurationMs, l.maxV)
	}
	pixelProg := int(math.Round((currVal - l.minV) * float64(l.barW) / (l.maxV - l.minV)))
	visual := make(display.Sprite, l.barH+2)
	for y := range visual {
		row := make([]display.Pixel, l.barW+2)
		for x := range row {
			switch {
			case y == 0:
				if x > 0 && x < l.barW+1 {
					row[x] = display.Gray
				} else {
					row[x] = display.Transparent
				}
			case y == l.barH+1:
				if x > 0 && x < l.barW+1 {
					row[x] = display.DarkGray
				} else {
					row[x] = display.Transparent
				}
			case x == 0:
				row[x] = display.Gray
			case x == l.barW+1:
				row[x] = display.DarkGray
			case x <= pixelProg:
				row[x] = display.DarkGreen
			default:
				row[x] = display.Black
			}
		}
		visual[y] = row
	}
	return visual
}

// Value returns the current progress value.
func (l *LoadingBar) Value() float64 { return l.value }

// SetValue updates the progress value.
func (l *LoadingBar) SetValue(v float64) {
	l.dirty = l.dirty || l.value != v
	l.value = v
}

// IsDirty always repaints in infinite mode.
func (l *LoadingBar) IsDirty() bool { return l.infinite || l.dirty }

// MarkClean keeps infinite bars dirty so they keep animating.
func (l *LoadingBar) MarkClean() {
	l.dirty = l.infinite && l.Visible()
}
