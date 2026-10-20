// SPDX-License-Identifier: GPL-2.0-or-later

package assetfile

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// Fixtures are regenerated on every run so the Go and browser parity tests share one oracle.
const testdataDir = "testdata"

func TestMain(m *testing.M) {
	if err := os.MkdirAll(testdataDir, 0o755); err != nil {
		panic(err)
	}
	if err := generateFixtures(); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func generateFixtures() error {
	type fixture struct {
		name string
		data []byte
	}
	fixtures := []fixture{
		{"sheet.arkadesprite.png", sheetFixture()},
		{"tone.arkadesound.wav", soundFixture()},
		{"heart.arkadeglyph.json", glyphFixture()},
		{"dancepad.arkadehid.json", hidFixture()},
		{"pulse.arkadelights.json", lightsFixture()},
		{"walls.arkadelevels", levelsFixture()},
		{"plain.levels", plainLevelsFixture()},
	}
	for _, f := range fixtures {
		path := filepath.Join(testdataDir, f.name)
		// Go's PNG encoder output varies by release; compare payloads to avoid
		// churning committed bytes.
		if old, err := os.ReadFile(path); err == nil && SamePayload(f.name, old, f.data) == nil {
			f.data = old
		} else if err := os.WriteFile(path, f.data, 0o644); err != nil {
			return err
		}
		env, err := ReadBytes(f.name, f.data)
		if err != nil {
			return err
		}
		expected, err := WriteJSON(env)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path+".expected.json", expected, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func sheetPixels() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 24, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 24; x++ {
			i := x / 8
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(0x20 * (i + 1)), G: 0x40, B: 0x80, A: 0xFF})
		}
	}
	return img
}

func encodePNG(img image.Image) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func sheetFixture() []byte {
	base := encodePNG(sheetPixels())
	env := &Envelope{
		Format:      FormatVersion,
		Kind:        KindSprite,
		Name:        "hero",
		Description: "Fixture sheet: three cels, one animation.",
		Tags:        []string{"fixture"},
		Generator:   Generator{Tool: "assetfile-fixtures", Version: "1.0.0"},
		Recipe:      json.RawMessage(`{"note":"hand-authored fixture, not rendered by any tool"}`),
		SpritePayload: SpritePayload{
			Cels: []Cel{
				{Name: "idle", X: 0, Y: 0, W: 8, H: 16, Anchor: [2]int{4, 16}},
				{Name: "walk.0", X: 8, Y: 0, W: 8, H: 16, Anchor: [2]int{4, 16}},
				{Name: "walk.1", X: 16, Y: 0, W: 8, H: 16, Anchor: [2]int{4, 16}},
			},
			Animations: []Animation{
				{Name: "walk", Frames: []Frame{{Cel: "walk.0", HoldMs: 90}, {Cel: "walk.1", HoldMs: 90}}, Loop: LoopCycle},
			},
		},
	}
	out, err := WriteBytes("sheet.arkadesprite.png", env, base)
	if err != nil {
		panic(err)
	}
	return out
}

func soundFixture() []byte {
	samples := []int16{0, 4000, -4000, 8000, -8000, 0}
	dataBytes := make([]byte, len(samples)*2)
	for i, s := range samples {
		binary.LittleEndian.PutUint16(dataBytes[i*2:], uint16(s))
	}
	const sampleRate = 22050
	const channels = 1
	const bits = 16
	fmtChunk := make([]byte, 16)
	binary.LittleEndian.PutUint16(fmtChunk[0:2], 1) // PCM
	binary.LittleEndian.PutUint16(fmtChunk[2:4], channels)
	binary.LittleEndian.PutUint32(fmtChunk[4:8], sampleRate)
	blockAlign := channels * bits / 8
	byteRate := sampleRate * blockAlign
	binary.LittleEndian.PutUint32(fmtChunk[8:12], uint32(byteRate))
	binary.LittleEndian.PutUint16(fmtChunk[12:14], uint16(blockAlign))
	binary.LittleEndian.PutUint16(fmtChunk[14:16], bits)

	base := joinRIFF([4]byte{'W', 'A', 'V', 'E'}, []riffChunk{
		{id: [4]byte{'f', 'm', 't', ' '}, data: fmtChunk},
		{id: [4]byte{'d', 'a', 't', 'a'}, data: dataBytes},
	})

	env := &Envelope{
		Format:       FormatVersion,
		Kind:         KindSound,
		Name:         "tone",
		Description:  "Fixture tone: a single short blip.",
		Generator:    Generator{Tool: "assetfile-fixtures", Version: "1.0.0"},
		Recipe:       json.RawMessage(`{"compose":[{"sfx":"blip","params":{"hz":880,"ms":60}}]}`),
		SoundPayload: SoundPayload{Render: &Render{SampleRate: sampleRate, Channels: channels, Bits: bits}},
	}
	out, err := WriteBytes("tone.arkadesound.wav", env, base)
	if err != nil {
		panic(err)
	}
	return out
}

func glyphFixture() []byte {
	env := &Envelope{
		Format:      FormatVersion,
		Kind:        KindGlyph,
		Name:        "heart",
		Description: "Filled heart for the lives counter.",
		Generator:   Generator{Tool: "assetfile-fixtures", Version: "1.0.0"},
		GlyphPayload: GlyphPayload{
			Codepoint: 0x2665, // 9829, U+2665 BLACK HEART SUIT
			Rows:      []int{4, 10, 31, 31, 31, 14, 4, 0},
			Category:  "hud",
		},
	}
	out, err := WriteBytes("heart.arkadeglyph.json", env, nil)
	if err != nil {
		panic(err)
	}
	return out
}

const levelsPackText = "pack walls\n\nlegend\n  . floor\n  # wall\n\nlevel L01\n  |###|\n  |#.#|\n  |###|\n"

func levelsFixture() []byte {
	env := &Envelope{
		Format:        FormatVersion,
		Kind:          KindLevels,
		Name:          "walls",
		Description:   "Fixture pack: one tiny room.",
		Tags:          []string{"fixture"},
		Generator:     Generator{Tool: "assetfile-fixtures", Version: "1.0.0"},
		LevelsPayload: LevelsPayload{Body: []byte(levelsPackText)},
	}
	out, err := WriteBytes("walls.arkadelevels", env, nil)
	if err != nil {
		panic(err)
	}
	return out
}

func plainLevelsFixture() []byte {
	return []byte(levelsPackText)
}

func lightsFixture() []byte {
	all := LightMaskPtr(MaxLightMask)
	dark := LightMaskPtr(0)
	d := LightMaskPtr(1 << 3)
	env := &Envelope{
		Format:      FormatVersion,
		Kind:        KindLights,
		Name:        "pulse",
		Description: "Win pulse: two flashes of every lamp, then the D button alone.",
		Tags:        []string{"fixture"},
		Generator:   Generator{Tool: "assetfile-fixtures", Version: "1.0.0"},
		LightsPayload: LightsPayload{
			Frames: []LightFrame{
				{Lamps: NewLightLamps(all, nil), HoldMs: 90},
				{Lamps: NewLightLamps(dark, nil), HoldMs: 90},
				{Lamps: NewLightLamps(all, nil), HoldMs: 90},
				{Lamps: NewLightLamps(d, nil), HoldMs: 200},
			},
			Loop: LoopOnce,
		},
	}
	out, err := WriteBytes("pulse.arkadelights.json", env, nil)
	if err != nil {
		panic(err)
	}
	return out
}

func hidFixture() []byte {
	env := &Envelope{
		Format:      FormatVersion,
		Kind:        KindHID,
		Name:        "dancepad",
		Description: "A four-arrow dance pad plus a spinner and a pressure sensor.",
		Generator:   Generator{Tool: "assetfile-fixtures", Version: "1.0.0"},
		HIDPayload: HIDPayload{
			Device: &Device{Vendor: 1356, Product: 2508, Name: "Dance pad"},
			Channels: []Channel{
				{Name: "pad.up", Kind: ChannelButton, Seat: 0, Report: Report{Byte: 3, Bit: 4}},
				{Name: "pad.down", Kind: ChannelButton, Seat: 0, Report: Report{Byte: 3, Bit: 5}},
				{Name: "spinner", Kind: ChannelDelta, Seat: 0, Report: Report{Byte: 5, Size: 2, Signed: true}},
				{Name: "pressure", Kind: ChannelBlob, Seat: 0, Report: Report{Byte: 8, Size: 32}},
			},
			Standard: map[string]string{"pad.up": "joy.north", "pad.down": "joy.south"},
		},
	}
	out, err := WriteBytes("dancepad.arkadehid.json", env, nil)
	if err != nil {
		panic(err)
	}
	return out
}
