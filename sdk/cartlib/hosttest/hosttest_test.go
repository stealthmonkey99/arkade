// SPDX-License-Identifier: GPL-2.0-or-later

package hosttest

import (
	"testing"

	"github.com/stealthmonkey99/arkade/sdk/cartlib"
)

// probe is a Game that exercises one of everything Setup and the frame loop
// offer, so the harness is tested end to end rather than per method.
type probe struct {
	chime   uint32
	wheel   cartlib.Channel
	hero    *cartlib.Sheet
	sprite  bool
	glyph   bool
	setup   *cartlib.Setup
	updates int
	presses int
	exit    bool
}

const probeManifest = `{"abi":1,"slug":"probe","name":"Probe"}`

var probeBytes = []byte(probeManifest)

func (p *probe) Manifest() []byte { return probeBytes }

func (p *probe) Init(s *cartlib.Setup) {
	p.setup = s
	p.chime = s.LoadSound("assets/chime.wav")
	p.wheel = s.BindDelta(cartlib.P1 + "spinner")
	sheet := s.LoadSheet("assets/hero.arkadesprite.png")
	p.hero = &sheet
	p.sprite = sheet.Cel("idle") != nil
	p.glyph = s.DefineLCDGlyph('♥', cartlib.GlyphRows{0, 0, 0xa, 0x1f, 0x1f, 0xe, 0x4, 0})
	cartlib.SetText(cartlib.SeatBoth, "HELLO", "WORLD")
}

func (p *probe) HandleInput(records []cartlib.InputRecord) {
	for _, r := range records {
		if r.IsPress() {
			p.presses++
		}
	}
}

func (p *probe) Update(dtMicros uint64) {
	p.updates++
	if p.wheel.Delta() != 0 {
		cartlib.PlaySound(p.chime, 1)
	}
}

func (p *probe) Render(fb *cartlib.Framebuffer) {
	fb.Clear(0, 0, 0)
	fb.SetPixel(3, 4, 0xaa, 0xbb, 0xcc)
	cartlib.SetTextBytes(cartlib.SeatP1, tickText, nil)
}

var tickText = []byte("TICK")

func newProbe(t *testing.T) (*Host, *probe) {
	t.Helper()
	h := New(t)
	h.Channels = map[string]Channel{cartlib.P1 + "spinner": {Kind: ChannelDelta, Value: 3}}
	h.AddCel("assets/hero.arkadesprite.png", "idle", 2, 2, make([]byte, 16))
	g := &probe{}
	h.Init(g)
	return h, g
}

func TestInitRunsWithALiveSetupThatDiesWithIt(t *testing.T) {
	h, g := newProbe(t)
	if g.chime != 1 || h.SoundID("assets/chime.wav") != 1 {
		t.Errorf("LoadSound handle = %d, SoundID = %d; want 1", g.chime, h.SoundID("assets/chime.wav"))
	}
	if !g.wheel.Ok() {
		t.Error("BindDelta of an offered channel failed")
	}
	if !g.hero.Ok() || !g.sprite {
		t.Error("LoadSheet or Cel failed for a cel the test added")
	}
	if !g.glyph || len(h.Glyphs) != 1 {
		t.Error("DefineLCDGlyph was refused")
	}
	if l1, l2, ok := h.LCD(cartlib.SeatP2); !ok || l1 != "HELLO" || l2 != "WORLD" {
		t.Errorf("LCD(SeatP2) = %q %q %v after a SeatBoth write", l1, l2, ok)
	}

	defer func() {
		if recover() == nil {
			t.Error("Setup.LoadSound after Init did not panic on host Go")
		}
	}()
	g.setup.LoadSound("assets/late.wav")
}

func TestStaleSheetCelPanics(t *testing.T) {
	_, g := newProbe(t)
	defer func() {
		if recover() == nil {
			t.Error("Sheet.Cel after Init did not panic on host Go")
		}
	}()
	g.hero.Cel("idle")
}

func TestZeroSheetCelIsANilCelNotAPanic(t *testing.T) {
	newProbe(t)
	var none cartlib.Sheet
	if none.Cel("idle") != nil {
		t.Error("the zero Sheet lifted a cel")
	}
}

func TestLoadPhaseHoldsUpdatesUntilTheSplashIsOver(t *testing.T) {
	h, g := newProbe(t)
	h.Tick(16_000)
	if g.updates != 0 || h.Blits() != 1 {
		t.Fatalf("during the splash: updates=%d blits=%d, want 0 and 1", g.updates, h.Blits())
	}
	if h.Pixel(3, 4) == (cartlib.Color{R: 0xaa, G: 0xbb, B: 0xcc, A: 0xff}) {
		t.Fatal("the Game's Render ran during the splash")
	}
	h.AdvancePastSplash()
	h.Tick(16_000)
	if g.updates != 1 {
		t.Fatalf("after the splash: updates=%d, want 1", g.updates)
	}
	if got := h.Pixel(3, 4); got != (cartlib.Color{R: 0xaa, G: 0xbb, B: 0xcc, A: 0xff}) {
		t.Errorf("Pixel(3,4) = %+v after Render", got)
	}
	if l1, _, _ := h.LCD(cartlib.SeatP1); l1 != "TICK" {
		t.Errorf("SetTextBytes did not reach the seat-one LCD: %q", l1)
	}
}

func TestInputGoesThroughTheWireAndTheQuitDefault(t *testing.T) {
	h, g := newProbe(t)
	h.AdvancePastSplash()
	h.Input(Press(cartlib.SeatP1, cartlib.KeyJoyA)...)
	if g.presses != 1 {
		t.Fatalf("presses = %d after one Press, want 1 (keydown must not count)", g.presses)
	}
	h.Input(Press(cartlib.SeatP2, cartlib.KeyJoyB)...)
	if h.Exits != 1 {
		t.Errorf("Exits = %d after a B press on a Game that does not own quit keys, want 1", h.Exits)
	}
	h.Input(KeyboardPress(cartlib.KeyUp), Stick(cartlib.SeatP1, cartlib.JoyN))
	if g.presses != 3 {
		t.Errorf("presses = %d, want 3: the virtual keypress counts, the stick record does not", g.presses)
	}
}

func TestChannelsSoundsAndScoresKeepCabinetSemantics(t *testing.T) {
	h, g := newProbe(t)
	h.AdvancePastSplash()
	h.Tick(16_000)
	if !h.Playing(g.chime) {
		t.Error("a delta of 3 did not make Update play the chime")
	}
	h.Tick(16_000)
	if n := len(h.Sounds); n != 1 {
		t.Errorf("the delta was reported twice: %d plays", n)
	}

	if cartlib.ScoreSubmit(0, "ZERO") != 0 || cartlib.ScoreSubmit(10, "TEN") != 1 || cartlib.ScoreSubmit(10, "TIE") != 2 {
		t.Errorf("score policy: %+v", h.Scores)
	}
	for i := 0; i < 8; i++ {
		cartlib.ScoreSubmit(20+i, "FILL")
	}
	if cartlib.ScoreQualifies(10) || cartlib.ScoreRank(10) != 0 || !cartlib.ScoreQualifies(11) {
		t.Errorf("a full table accepted a tie with tenth place or refused a beat: %+v", h.Scores)
	}
	var buf [8 + cartlib.MaxNameLen]byte
	if score, name, ok := cartlib.ScoreRead(1, buf[:]); !ok || score != 27 || string(name) != "FILL" {
		t.Errorf("ScoreRead(1) = %d %q %v", score, name, ok)
	}
	cartlib.NoteRecentName(cartlib.SeatP1, "ADA")
	cartlib.NoteRecentName(cartlib.SeatP1, "BOB")
	if names := cartlib.RecentNames(cartlib.SeatP1); len(names) < 2 || names[0] != "BOB" || names[1] != "ADA" {
		t.Errorf("RecentNames = %q", names)
	}
}

func TestStorageRoundTrips(t *testing.T) {
	newProbe(t)
	if !cartlib.StorageWrite("k", []byte("value")) {
		t.Fatal("a legal write was refused")
	}
	var out [3]byte
	if n := cartlib.StorageRead("k", out[:]); n != 3 || string(out[:]) != "val" {
		t.Errorf("StorageRead into a short buffer = %d %q, want 3 \"val\"", n, out[:n])
	}
}

func TestLampStateFollowsEveryWrite(t *testing.T) {
	h, _ := newProbe(t)
	cartlib.SetButtonLights(cartlib.SeatBoth, cartlib.MaskJoyA|cartlib.MaskJoyC, true)
	want := cartlib.LampLevels{15, 0, 15, 0, 0, 0}
	if h.Lamps[0] != want || h.Lamps[1] != want {
		t.Errorf("Lamps = %v %v, want %v on both", h.Lamps[0], h.Lamps[1], want)
	}
	cartlib.PlayLights("assets/win.arkadelights.json", cartlib.SeatP2, cartlib.LightsLoopFile)
	cartlib.FadeLamps(cartlib.SeatP2, cartlib.MaskJoyB, 7, 300)
	if h.Lamps[1] != (cartlib.LampLevels{15, 7, 15, 0, 0, 0}) || h.LightsPlaying[1] != "" {
		t.Errorf("after a fade touching B: lamps %v, playing %q", h.Lamps[1], h.LightsPlaying[1])
	}
	cartlib.SetLampLevels(cartlib.SeatP1, cartlib.LampLevels{}, 0, 0)
	if h.Lamps[0] != want {
		t.Error("a SetLampLevels touching nothing changed the lamps")
	}
}

func TestATickAllocatesNothingOnHostGo(t *testing.T) {
	h, _ := newProbe(t)
	h.AdvancePastSplash()
	h.Tick(16_000)
	if n := h.AllocsPerRun(50, func() { h.Tick(16_000) }); n != 0 {
		t.Errorf("a probe tick allocated %v times per run through the fake", n)
	}
}

func TestEncodeIsTheWireFormat(t *testing.T) {
	r := cartlib.InputRecord{Kind: cartlib.KindJoystick, Key: 9, Player: cartlib.SlotP2, Joy: cartlib.JoySW,
		Source: cartlib.SourceController, Physical: cartlib.SeatP2, Index: 1, Flags: cartlib.FlagShift}
	got := cartlib.DecodeInputRecords(Encode([]cartlib.InputRecord{r}))
	if len(got) != 1 || got[0] != r {
		t.Errorf("Encode then decode = %+v, want %+v", got, r)
	}
}
