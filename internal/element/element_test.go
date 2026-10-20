// SPDX-License-Identifier: GPL-2.0-or-later

package element

import (
	"testing"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

func stats() clock.FrameTime { return clock.FrameTime{} }

func TestLabelRendersGlyphs(t *testing.T) {
	l := NewLabel(LabelOptions{Text: "A"})
	sprite := l.Render(stats())
	if len(sprite) != 8 {
		t.Fatalf("labels are 8 rows tall, got %d", len(sprite))
	}
	lit := 0
	for _, row := range sprite {
		for _, p := range row {
			if p == display.White {
				lit++
			}
		}
	}
	if lit == 0 {
		t.Error("glyph 'A' should light pixels")
	}
}

func TestLabelUnknownGlyphSkipped(t *testing.T) {
	l := NewLabel(LabelOptions{Text: "☃"}) // snowman not in font
	sprite := l.Render(stats())
	for _, row := range sprite {
		if len(row) != 0 {
			t.Fatal("unknown glyphs should render no columns")
		}
	}
}

func TestLabelDirtyOnTextChange(t *testing.T) {
	l := NewLabel(LabelOptions{Text: "HI"})
	l.Render(stats())
	l.MarkClean()
	l.SetText("HI")
	if l.IsDirty() {
		t.Error("same text should not dirty")
	}
	l.SetText("YO")
	if !l.IsDirty() {
		t.Error("new text should dirty")
	}
}

func TestRectFillAndEdgeBorder(t *testing.T) {
	fill := display.Blue
	r := NewRect(RectOptions{W: 6, H: 6, Fill: &fill, Border: &EdgeBorder})
	sprite := r.Render(stats())
	if len(sprite) != 6 || len(sprite[0]) != 6 {
		t.Fatalf("expected 6x6, got %dx%d", len(sprite), len(sprite[0]))
	}
	if sprite[2][2] != display.Blue {
		t.Errorf("interior should be fill, got %+v", sprite[2][2])
	}
	// top row is highlight (fill + white@70): brighter than fill
	if sprite[0][0].R <= display.Blue.R {
		t.Errorf("top edge should be highlighted, got %+v", sprite[0][0])
	}
	// bottom row is shadow: darker than fill in blue channel
	if sprite[5][0].B >= display.Blue.B {
		t.Errorf("bottom edge should be shadowed, got %+v", sprite[5][0])
	}
}

func TestCircleDimensions(t *testing.T) {
	c := NewCircle(CircleOptions{Radius: 3})
	sprite := c.Render(stats())
	if len(sprite) != 7 {
		t.Fatalf("radius 3 circle should be 7 rows, got %d", len(sprite))
	}
}

func TestAlignmentCenterShiftsOffset(t *testing.T) {
	fill := display.Red
	r := NewRect(RectOptions{Options: Options{OffsetX: 10, OffsetY: 10, Align: Center}, W: 4, H: 4, Fill: &fill, Border: &NoBorder})
	r.Render(stats())
	if r.OffsetX() != 8 || r.OffsetY() != 8 {
		t.Errorf("center alignment should shift by half size, got (%d,%d)", r.OffsetX(), r.OffsetY())
	}
}

func TestElementRotationSwapsDims(t *testing.T) {
	fill := display.Red
	r := NewRect(RectOptions{W: 6, H: 2, Fill: &fill, Border: &NoBorder})
	r.Render(stats())
	if r.Width() != 6 || r.Height() != 2 {
		t.Fatalf("pre-rotation dims wrong: %dx%d", r.Width(), r.Height())
	}
	r.SetRotation(display.Clockwise)
	sprite := r.Render(stats())
	if len(sprite) != 6 || len(sprite[0]) != 2 {
		t.Errorf("rotated sprite should be 6 rows x 2 cols, got %dx%d", len(sprite), len(sprite[0]))
	}
	if r.Width() != 2 || r.Height() != 6 {
		t.Errorf("rotated dims should swap: got %dx%d", r.Width(), r.Height())
	}
}

func TestHiddenElementRendersEmpty(t *testing.T) {
	l := NewLabel(LabelOptions{Text: "X"})
	l.SetVisible(false)
	sprite := l.Render(stats())
	if len(sprite) != 1 || len(sprite[0]) != 0 {
		t.Errorf("hidden element should render empty sprite, got %d rows", len(sprite))
	}
}

func TestButtonFocusChangesFill(t *testing.T) {
	b := NewButton(ButtonOptions{Label: "GO"})
	unfocused := b.Render(stats())
	b.setFocused(true)
	focused := b.Render(stats())
	if display.MatrixEquals(unfocused, focused) {
		t.Error("focus should change the button visuals")
	}
	if len(focused) != 10 || len(focused[0]) != (ui.FontLED{}).TextWidth("GO", 1)+3 {
		t.Errorf("button is %dx%d, want the label's width plus 3 by 10", len(focused[0]), len(focused))
	}
}

func TestMenuFocusMoveAndSelect(t *testing.T) {
	var selected string
	mk := func(name string) *Button {
		return NewButton(ButtonOptions{Label: name, Action: func(b *Button, _ input.Event) { selected = b.Text() }})
	}
	m := NewMenu(MenuOptions{Buttons: []*Button{mk("ONE"), mk("TWO")}})
	if m.CurrentSelection().Text() != "ONE" {
		t.Fatal("first button should start focused")
	}
	m.HandleInput(input.Event{Kind: input.KindKeypress, Name: input.KeyDown})
	if m.CurrentSelection().Text() != "TWO" {
		t.Error("down should move focus")
	}
	m.HandleInput(input.Event{Kind: input.KindKeypress, Name: input.KeyEnter})
	if selected != "TWO" {
		t.Errorf("enter should select focused button, got %q", selected)
	}
}

func TestLoadingBarProgress(t *testing.T) {
	bar, err := NewLoadingBar(LoadingBarOptions{BarW: 10, BarH: 2, Start: 50})
	if err != nil {
		t.Fatal(err)
	}
	sprite := bar.Render(stats())
	if len(sprite) != 4 {
		t.Fatalf("bar should be h+2 rows, got %d", len(sprite))
	}
	if sprite[1][3] != display.DarkGreen {
		t.Errorf("progress area should be green at 50%%, got %+v", sprite[1][3])
	}
	if sprite[1][9] != display.Black {
		t.Errorf("unfilled area should be black, got %+v", sprite[1][9])
	}
}

func TestSpriteJSONDecode(t *testing.T) {
	raw := []byte(`[[{"red":255,"green":0,"blue":0,"alpha":255}],[{"red":0,"green":0,"blue":0,"alpha":0}]]`)
	data, err := DecodeJSONSprite(raw)
	if err != nil {
		t.Fatal(err)
	}
	if data[0][0] != display.Red {
		t.Errorf("expected red pixel, got %+v", data[0][0])
	}
	if data[1][0] != display.Transparent {
		t.Errorf("expected transparent pixel, got %+v", data[1][0])
	}
}

// TestEveryElementIsCleanAfterPaint pins the layer protocol: Render then
// MarkClean leaves an element clean, whatever it composes.
func TestEveryElementIsCleanAfterPaint(t *testing.T) {
	fill := display.Blue
	bar, err := NewLoadingBar(LoadingBarOptions{})
	if err != nil {
		t.Fatal(err)
	}
	qr, err := NewQRCode("ARKADE", QRCodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	els := map[string]display.Element{
		"label":  NewLabel(LabelOptions{Text: "HI"}),
		"rect":   NewRect(RectOptions{Fill: &fill}),
		"circle": NewCircle(CircleOptions{}),
		"line":   NewLine(LineOptions{}),
		"sprite": NewSprite(SpriteOptions{Data: display.Sprite{{display.Red}}}),
		"button": NewButton(ButtonOptions{Label: "GO"}),
		"menu":   NewMenu(MenuOptions{Buttons: []*Button{NewButton(ButtonOptions{Label: "A"}), NewButton(ButtonOptions{Label: "B"})}}),
		"bar":    bar,
		"qr":     qr,
	}
	for name, e := range els {
		if !e.IsDirty() {
			t.Errorf("%s: a new element must paint once", name)
		}
		e.Render(stats())
		e.MarkClean()
		if e.IsDirty() {
			t.Errorf("%s: still dirty after Render and MarkClean", name)
		}
	}
}
