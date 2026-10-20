// SPDX-License-Identifier: GPL-2.0-or-later

package assetfile

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/assetfile/pngchunk"
)

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(testdataDir, name))
	if err != nil {
		t.Fatalf("reading testdata/%s: %v", name, err)
	}
	return data
}

func TestKindFromFilename(t *testing.T) {
	cases := []struct {
		name     string
		wantKind Kind
		wantOK   bool
	}{
		{"hero.arkadesprite.png", KindSprite, true},
		{"jump.arkadesound.wav", KindSound, true},
		{"heart.arkadeglyph.json", KindGlyph, true},
		{"dancepad.arkadehid.json", KindHID, true},
		{"commands.arkademic.json", KindMic, true},
		{"commands.arkademic.wav", "", false}, // wrong container for the kind
		{"assets/hero.arkadesprite.png", KindSprite, true},
		{"hero.png", "", false},
		{"hero.arkadesprite.wav", "", false}, // wrong container for the kind
		{"hero.arkadefoo.png", "", false},    // unknown kind
		{"arkadesprite.png", "", false},      // no name before the kind
		{"caves.levels", KindLevels, true},
		{"caves.arkadelevels", KindLevels, true},
		{"assets/caves.levels", KindLevels, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind, ok := KindFromFilename(tc.name)
			if kind != tc.wantKind || ok != tc.wantOK {
				t.Errorf("KindFromFilename(%q) = (%q, %v), want (%q, %v)", tc.name, kind, ok, tc.wantKind, tc.wantOK)
			}
		})
	}
}

func TestRoundTripSprite(t *testing.T) {
	data := readTestdata(t, "sheet.arkadesprite.png")
	env, err := ReadBytes("sheet.arkadesprite.png", data)
	if err != nil {
		t.Fatalf("ReadBytes: %v", err)
	}
	if env.Kind != KindSprite || env.Name != "hero" {
		t.Fatalf("env = %+v, want kind sprite, name hero", env)
	}
	if len(env.Cels) != 3 || len(env.Animations) != 1 {
		t.Fatalf("env has %d cels and %d animations, want 3 and 1", len(env.Cels), len(env.Animations))
	}
	if env.Animations[0].Loop != LoopCycle {
		t.Errorf("loop = %q, want %q", env.Animations[0].Loop, LoopCycle)
	}

	out, err := WriteBytes("sheet.arkadesprite.png", env, data)
	if err != nil {
		t.Fatalf("WriteBytes: %v", err)
	}
	got, err := ReadBytes("sheet.arkadesprite.png", out)
	if err != nil {
		t.Fatalf("ReadBytes (round trip): %v", err)
	}
	if !reflect.DeepEqual(got, env) {
		t.Errorf("round trip mismatch\ngot:  %+v\nwant: %+v", got, env)
	}
}

func TestTheRetiredFramesChunkIsNoEnvelope(t *testing.T) {
	data, err := pngchunk.WriteText(encodePNG(sheetPixels()), "arkade:frames",
		`{"version":1,"layout":"strip","cels":3,"celW":8,"celH":16,"order":[0,1,2],"delays":[100,100,100],"loop":"loop"}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadBytes("strip.png", data); !errors.Is(err, ErrNoEnvelope) {
		t.Errorf("ReadBytes error = %v, want ErrNoEnvelope", err)
	}
}

func TestRoundTripSound(t *testing.T) {
	data := readTestdata(t, "tone.arkadesound.wav")
	env, err := ReadBytes("tone.arkadesound.wav", data)
	if err != nil {
		t.Fatalf("ReadBytes: %v", err)
	}
	if env.Kind != KindSound || env.Render == nil || env.Render.SampleRate != 22050 {
		t.Fatalf("env = %+v, want a sound envelope at 22050Hz", env)
	}
	out, err := WriteBytes("tone.arkadesound.wav", env, data)
	if err != nil {
		t.Fatalf("WriteBytes: %v", err)
	}
	got, err := ReadBytes("tone.arkadesound.wav", out)
	if err != nil {
		t.Fatalf("ReadBytes (round trip): %v", err)
	}
	if !reflect.DeepEqual(got, env) {
		t.Errorf("round trip mismatch\ngot:  %+v\nwant: %+v", got, env)
	}
}

func TestRoundTripGlyph(t *testing.T) {
	data := readTestdata(t, "heart.arkadeglyph.json")
	env, err := ReadBytes("heart.arkadeglyph.json", data)
	if err != nil {
		t.Fatalf("ReadBytes: %v", err)
	}
	if env.Codepoint != 0x2665 || len(env.Rows) != 8 {
		t.Fatalf("env = %+v, want codepoint U+2665 and 8 rows", env)
	}
	out, err := WriteBytes("heart.arkadeglyph.json", env, nil)
	if err != nil {
		t.Fatalf("WriteBytes: %v", err)
	}
	got, err := ReadBytes("heart.arkadeglyph.json", out)
	if err != nil {
		t.Fatalf("ReadBytes (round trip): %v", err)
	}
	if !reflect.DeepEqual(got, env) {
		t.Errorf("round trip mismatch\ngot:  %+v\nwant: %+v", got, env)
	}
}

func TestRoundTripHID(t *testing.T) {
	data := readTestdata(t, "dancepad.arkadehid.json")
	env, err := ReadBytes("dancepad.arkadehid.json", data)
	if err != nil {
		t.Fatalf("ReadBytes: %v", err)
	}
	if len(env.Channels) != 4 || env.Standard["pad.up"] != "joy.north" {
		t.Fatalf("env = %+v, want 4 channels and a standard alias for pad.up", env)
	}
	out, err := WriteBytes("dancepad.arkadehid.json", env, nil)
	if err != nil {
		t.Fatalf("WriteBytes: %v", err)
	}
	got, err := ReadBytes("dancepad.arkadehid.json", out)
	if err != nil {
		t.Fatalf("ReadBytes (round trip): %v", err)
	}
	if !reflect.DeepEqual(got, env) {
		t.Errorf("round trip mismatch\ngot:  %+v\nwant: %+v", got, env)
	}
}

func TestMarshalIsDeterministic(t *testing.T) {
	data := readTestdata(t, "dancepad.arkadehid.json")
	env, err := ReadBytes("dancepad.arkadehid.json", data)
	if err != nil {
		t.Fatalf("ReadBytes: %v", err)
	}
	a, err := Marshal(env)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	b, err := Marshal(env)
	if err != nil {
		t.Fatalf("Marshal (again): %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Error("two Marshal calls over the same envelope produced different bytes")
	}
}

func TestWriteSpriteIsDeterministic(t *testing.T) {
	base := readTestdata(t, "sheet.arkadesprite.png")
	env, err := ReadBytes("sheet.arkadesprite.png", base)
	if err != nil {
		t.Fatalf("ReadBytes: %v", err)
	}
	first, err := WriteBytes("sheet.arkadesprite.png", env, base)
	if err != nil {
		t.Fatalf("WriteBytes: %v", err)
	}
	second, err := WriteBytes("sheet.arkadesprite.png", env, base)
	if err != nil {
		t.Fatalf("WriteBytes (again): %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Error("writing the same sprite envelope twice produced different bytes")
	}
}

func TestWAVWriterPreservesDataChunk(t *testing.T) {
	base := readTestdata(t, "tone.arkadesound.wav")
	_, origChunks, err := splitRIFF(base)
	if err != nil {
		t.Fatalf("splitRIFF: %v", err)
	}
	var wantData []byte
	for _, c := range origChunks {
		if string(c.id[:]) == "data" {
			wantData = c.data
		}
	}
	if wantData == nil {
		t.Fatal("fixture has no data chunk")
	}

	env, err := ReadBytes("tone.arkadesound.wav", base)
	if err != nil {
		t.Fatalf("ReadBytes: %v", err)
	}
	env.Description = "a different description, to force a rewrite"
	out, err := WriteBytes("tone.arkadesound.wav", env, base)
	if err != nil {
		t.Fatalf("WriteBytes: %v", err)
	}

	_, gotChunks, err := splitRIFF(out)
	if err != nil {
		t.Fatalf("splitRIFF (rewritten): %v", err)
	}
	var gotData []byte
	for _, c := range gotChunks {
		if string(c.id[:]) == "data" {
			gotData = c.data
		}
	}
	if !bytes.Equal(gotData, wantData) {
		t.Error("the data chunk changed after writing a new envelope")
	}
}

func baseSpriteEnvelope() *Envelope {
	return &Envelope{
		Format:    FormatVersion,
		Kind:      KindSprite,
		Name:      "t",
		Generator: Generator{Tool: "t", Version: "1"},
		SpritePayload: SpritePayload{
			Cels: []Cel{
				{Name: "a", X: 0, Y: 0, W: 4, H: 4},
				{Name: "b", X: 4, Y: 0, W: 4, H: 4},
			},
		},
	}
}

func TestValidateSpriteStructural(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(*Envelope)
		wantW int
		wantH int
		want  string
	}{
		{"wrong format", func(e *Envelope) { e.Format = 2 }, 8, 4, "format 2"},
		{"unknown kind", func(e *Envelope) { e.Kind = "bogus" }, 8, 4, "unknown kind"},
		{"kind mismatch is checked by caller", func(e *Envelope) {}, 8, 4, ""},
		{"no cels", func(e *Envelope) { e.Cels = nil }, 8, 4, "no cels"},
		{"duplicate cel name", func(e *Envelope) { e.Cels[1].Name = "a" }, 8, 4, "used twice"},
		{"cel outside the image", func(e *Envelope) { e.Cels[1].X = 5 }, 8, 4, "outside the"},
		{"cel non-positive size", func(e *Envelope) { e.Cels[0].W = 0 }, 8, 4, "non-positive size"},
		{"animation references missing cel", func(e *Envelope) {
			e.Animations = []Animation{{Name: "x", Frames: []Frame{{Cel: "c", HoldMs: 100}}}}
		}, 8, 4, "not in the sheet"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := baseSpriteEnvelope()
			tc.edit(env)
			err := Validate(env, KindSprite, tc.wantW, tc.wantH)
			if tc.want == "" {
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Validate error = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestValidateSpriteKindMismatch(t *testing.T) {
	env := baseSpriteEnvelope()
	if err := Validate(env, KindSound, 8, 4); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Errorf("Validate error = %v, want a kind-mismatch complaint", err)
	}
}

// validateThenNormalize is what every reader does, and checks on the way
// that Validate itself leaves env untouched and that Normalize is idempotent.
func validateThenNormalize(t *testing.T, env *Envelope, kind Kind, imgW, imgH int) {
	t.Helper()
	before, err := Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(env, kind, imgW, imgH); err != nil {
		t.Fatalf("a repairable envelope must not fail validation: %v", err)
	}
	if after, _ := Marshal(env); !bytes.Equal(before, after) {
		t.Fatalf("Validate modified the envelope:\nbefore %s\nafter  %s", before, after)
	}
	Normalize(env)
	once, _ := Marshal(env)
	Normalize(env)
	if twice, _ := Marshal(env); !bytes.Equal(once, twice) {
		t.Fatalf("a second Normalize changed the envelope:\nonce  %s\ntwice %s", once, twice)
	}
}

func TestNormalizeSpriteRepairsAnimation(t *testing.T) {
	env := baseSpriteEnvelope()
	env.Animations = []Animation{{Name: "x", Frames: []Frame{{Cel: "a"}, {Cel: "b"}, {Cel: "a"}}, Loop: "nonsense"}}
	validateThenNormalize(t, env, KindSprite, 8, 4)
	a := env.Animations[0]
	want := []Frame{{Cel: "a", HoldMs: 120}, {Cel: "b", HoldMs: 120}, {Cel: "a", HoldMs: 120}}
	if !reflect.DeepEqual(a.Frames, want) {
		t.Errorf("frames = %v, want the default fill %v", a.Frames, want)
	}
	if a.Loop != LoopCycle {
		t.Errorf("loop = %q, want the default %q", a.Loop, LoopCycle)
	}
}

func TestNormalizeSpriteClampsHoldMs(t *testing.T) {
	env := baseSpriteEnvelope()
	env.Animations = []Animation{{Name: "x", Frames: []Frame{{Cel: "a", HoldMs: 0}, {Cel: "b", HoldMs: 999999}}, Loop: LoopOnce}}
	validateThenNormalize(t, env, KindSprite, 8, 4)
	if want := (Animation{Name: "x", Frames: []Frame{{Cel: "a", HoldMs: 120}, {Cel: "b", HoldMs: 10000}}, Loop: LoopOnce}); !reflect.DeepEqual(env.Animations[0], want) {
		t.Errorf("animation = %+v, want %+v", env.Animations[0], want)
	}
}

func TestValidateSound(t *testing.T) {
	cases := []struct {
		name   string
		render *Render
		want   string
	}{
		{"nil render", nil, "no render block"},
		{"bad sample rate", &Render{SampleRate: 0, Channels: 1, Bits: 16}, "sampleRate"},
		{"bad channels", &Render{SampleRate: 22050, Channels: 3, Bits: 16}, "channels 3"},
		{"bad bits", &Render{SampleRate: 22050, Channels: 1, Bits: 12}, "bits 12"},
		{"ok", &Render{SampleRate: 22050, Channels: 2, Bits: 24}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := &Envelope{Format: FormatVersion, Kind: KindSound, Generator: Generator{Tool: "t", Version: "1"}, SoundPayload: SoundPayload{Render: tc.render}}
			err := Validate(env, KindSound, 0, 0)
			if tc.want == "" {
				if err != nil {
					t.Errorf("Validate: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Validate error = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestValidateGlyph(t *testing.T) {
	base := func() *Envelope {
		return &Envelope{
			Format: FormatVersion, Kind: KindGlyph, Generator: Generator{Tool: "t", Version: "1"},
			GlyphPayload: GlyphPayload{
				Codepoint: 0x2665,
				Rows:      []int{0, 0, 0, 0, 0, 0, 0, 0},
			},
		}
	}
	cases := []struct {
		name string
		edit func(*Envelope)
		want string
	}{
		{"codepoint below 0x80", func(e *Envelope) { e.Codepoint = 0x41 }, "below U+0080"},
		{"surrogate codepoint", func(e *Envelope) { e.Codepoint = 0xD800 }, "surrogate"},
		{"emoji codepoint", func(e *Envelope) { e.Codepoint = 0x1F9FA }, "is emoji"},
		{"wide codepoint", func(e *Envelope) { e.Codepoint = 0x4E00 }, "is wide"},
		{"past U+10FFFF", func(e *Envelope) { e.Codepoint = 0x110000 }, "not a code point"},
		{"wrong row count", func(e *Envelope) { e.Rows = e.Rows[:7] }, "want 8"},
		{"row out of range", func(e *Envelope) { e.Rows[0] = 0x20 }, "0..0x1f"},
		{"negative row", func(e *Envelope) { e.Rows[0] = -1 }, "0..0x1f"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := base()
			tc.edit(env)
			if err := Validate(env, KindGlyph, 0, 0); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Validate error = %v, want it to mention %q", err, tc.want)
			}
		})
	}
	env := base()
	env.Codepoint = 0x058D
	if err := Validate(env, KindGlyph, 0, 0); err != nil {
		t.Errorf("Validate refused U+058D: %v", err)
	}
}

func TestValidateHID(t *testing.T) {
	base := func() *Envelope {
		return &Envelope{
			Format: FormatVersion, Kind: KindHID, Generator: Generator{Tool: "t", Version: "1"},
			HIDPayload: HIDPayload{
				Channels: []Channel{
					{Name: "a", Kind: ChannelButton, Report: Report{Byte: 0, Bit: 0}},
					{Name: "b", Kind: ChannelBlob, Report: Report{Byte: 1, Size: 4}},
				},
			},
		}
	}
	cases := []struct {
		name string
		edit func(*Envelope)
		want string
	}{
		{"no channels", func(e *Envelope) { e.Channels = nil }, "no channels"},
		{"duplicate name", func(e *Envelope) { e.Channels[1].Name = "a" }, "used twice"},
		{"unknown kind", func(e *Envelope) { e.Channels[0].Kind = "bogus" }, "unknown kind"},
		{"blob too big", func(e *Envelope) { e.Channels[1].Report.Size = 257 }, "1..256"},
		{"blob zero size", func(e *Envelope) { e.Channels[1].Report.Size = 0 }, "1..256"},
		{"bad seat", func(e *Envelope) { e.Channels[0].Seat = 2 }, "seat 2"},
		{"standard references missing channel", func(e *Envelope) {
			e.Standard = map[string]string{"c": "joy.north"}
		}, "not in channels"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := base()
			tc.edit(env)
			if err := Validate(env, KindHID, 0, 0); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Validate error = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func solidImage(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 0x10, G: 0x20, B: 0x30, A: 0xFF})
		}
	}
	return img
}
