// SPDX-License-Identifier: GPL-2.0-or-later

package wasm

import (
	"fmt"
	"math"

	"github.com/stealthmonkey99/arkade/assetfile"
	"github.com/stealthmonkey99/arkade/internal/controller"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
)

// cartLayerLabel is the layer holding the one full-canvas sprite the ABI's
// immediate-mode draw calls rasterize into.
const cartLayerLabel = "cart-fb"

// Identity names the two owners an EngineHost acts for. CartSlug owns what
// the cartridge persists and tags its log lines; LayerOwner owns its display
// layer, sounds and bundle overlay. An attract instance takes a separate
// LayerOwner so it never collides with the same game suspended under standby.
type Identity struct {
	CartSlug   string
	LayerOwner string
}

// PlayIdentity is an ordinary launch's identity: the app's slug owns everything.
func PlayIdentity(ctx *engine.AppContext) Identity {
	return Identity{CartSlug: ctx.Manifest.Slug, LayerOwner: ctx.Manifest.Slug}
}

// EngineHost implements Host on a real engine.AppContext. Its Mode picks the
// Control and Scoreboard it serves; every other role is the same in both.
type EngineHost struct {
	Control
	Scoreboard

	ctx *engine.AppContext
	id  Identity

	fb   display.Sprite
	fbEl *element.SpriteEl

	sprites map[uint32]display.Sprite
	sounds  map[uint32]string
	nextID  uint32

	assets map[string][]byte // kept for BundleAsset as well as AssetOverlay
}

// NewEngineHost adds a width x height framebuffer layer under id.LayerOwner
// and registers assets, nil for a bare .wasm, with the AssetOverlay so
// LoadSound resolves bundle paths. Close removes both.
func NewEngineHost(ctx *engine.AppContext, id Identity, mode Mode, width, height int, assets map[string][]byte) *EngineHost {
	fb := display.NewMatrix(width, height, display.Transparent)
	el := element.NewSprite(element.SpriteOptions{Data: fb})
	opacity := 1.0
	ctx.RT.Display.AddLayer(id.LayerOwner, cartLayerLabel, display.LayerOptions{
		Elements: []display.Element{el},
		Opacity:  &opacity,
	})
	ctx.RT.AssetOverlay.Register(id.LayerOwner, assets)
	h := &EngineHost{
		ctx:     ctx,
		id:      id,
		fb:      fb,
		fbEl:    el,
		sprites: map[uint32]display.Sprite{},
		sounds:  map[uint32]string{},
		assets:  assets,
	}
	board := engineScoreboard{ctx: ctx, slug: id.CartSlug, log: h}
	switch mode {
	case ModeAttract:
		h.Control, h.Scoreboard = noControl{}, readOnlyScoreboard{board}
	default:
		h.Control, h.Scoreboard = &engineControl{ctx: ctx}, board
	}
	return h
}

func (h *EngineHost) Close() {
	h.ctx.RT.AssetOverlay.Unregister(h.id.LayerOwner)
	h.ctx.RT.Display.RemoveLayers(h.id.LayerOwner, cartLayerLabel)
}

// goDark clears the framebuffer and silences the cartridge's sounds.
func (h *EngineHost) goDark() {
	for _, row := range h.fb {
		for x := range row {
			row[x] = display.Transparent
		}
	}
	h.fbEl.MarkDirty()
	h.StopAllSounds()
}

var _ Host = (*EngineHost)(nil)

func (h *EngineHost) inBounds(x, y int) bool {
	return y >= 0 && y < len(h.fb) && x >= 0 && x < len(h.fb[y])
}

func (h *EngineHost) setPixel(x, y int, c display.Pixel) {
	if h.inBounds(x, y) {
		h.fb[y][x] = c
	}
}

func (h *EngineHost) mergePixel(x, y int, c display.Pixel) {
	if h.inBounds(x, y) {
		h.fb[y][x] = display.Merge(h.fb[y][x], c)
	}
}

// Blit overwrites the whole framebuffer, or does nothing when pixels is not
// exactly rows*cols.
func (h *EngineHost) Blit(pixels []display.Pixel) {
	rows := len(h.fb)
	if rows == 0 {
		return
	}
	cols := len(h.fb[0])
	if len(pixels) != rows*cols {
		return
	}
	for y := 0; y < rows; y++ {
		copy(h.fb[y], pixels[y*cols:(y+1)*cols])
	}
	h.fbEl.MarkDirty()
}

func (h *EngineHost) DrawPixel(x, y int, c display.Pixel) {
	h.setPixel(x, y, c)
	h.fbEl.MarkDirty()
}

func (h *EngineHost) FillRect(x, y, w, hgt int, c display.Pixel) {
	for ry := y; ry < y+hgt; ry++ {
		for rx := x; rx < x+w; rx++ {
			h.setPixel(rx, ry, c)
		}
	}
	h.fbEl.MarkDirty()
}

func (h *EngineHost) LoadSprite(png []byte) (uint32, error) {
	data, err := element.DecodePNG(png)
	if err != nil {
		return 0, fmt.Errorf("decode sprite png: %w", err)
	}
	h.nextID++
	h.sprites[h.nextID] = data
	return h.nextID, nil
}

func (h *EngineHost) DrawSprite(id uint32, x, y int) {
	sprite, ok := h.sprites[id]
	if !ok {
		return
	}
	for ry, row := range sprite {
		for rx, p := range row {
			if p.A == 0 {
				continue
			}
			h.mergePixel(x+rx, y+ry, p)
		}
	}
	h.fbEl.MarkDirty()
}

// DrawCel shares DrawSprite's blend, so a host-drawn cel and a guest-drawn
// ui.Sprite land the same pixels.
func (h *EngineHost) DrawCel(pix []byte, w, hgt, x, y int) {
	if len(pix) < w*hgt*4 {
		return
	}
	i := 0
	for ry := 0; ry < hgt; ry++ {
		for rx := 0; rx < w; rx++ {
			if a := pix[i+3]; a != 0 {
				h.mergePixel(x+rx, y+ry, display.Pixel{R: pix[i], G: pix[i+1], B: pix[i+2], A: a})
			}
			i += 4
		}
	}
	h.fbEl.MarkDirty()
}

func (h *EngineHost) BundleAsset(path string) ([]byte, bool) {
	data, ok := h.assets[path]
	return data, ok
}

func (h *EngineHost) LoadSound(path string) (uint32, error) {
	h.nextID++
	h.sounds[h.nextID] = path
	return h.nextID, nil
}

// PlaySound treats loopCount 0xFFFFFFFF as loop forever and any other value
// as a repeat count.
func (h *EngineHost) PlaySound(id uint32, loopCount uint32) {
	path, ok := h.sounds[id]
	if !ok {
		return
	}
	loop := int(loopCount)
	if loopCount == math.MaxUint32 {
		loop = -1 // audio.LoopForever
	}
	h.ctx.RT.Audio.Play(h.id.LayerOwner, soundLabel(id), path, loop)
}

func (h *EngineHost) StopSound(id uint32) {
	if _, ok := h.sounds[id]; !ok {
		return
	}
	h.ctx.RT.Audio.Stop(h.id.LayerOwner, soundLabel(id))
}

func (h *EngineHost) StopAllSounds() {
	h.ctx.RT.Audio.Stop(h.id.LayerOwner, "")
}

func soundLabel(id uint32) string { return fmt.Sprintf("cart-%d", id) }

// storageBlob carries raw bytes through the JSON-only storage.Store as base64.
type storageBlob struct {
	Data []byte `json:"data"`
}

// StorageKeyOK reports whether key is safe as a storage label, which
// storage.Store joins into a filesystem path: [A-Za-z0-9_.-]+, never "." or "..".
func StorageKeyOK(key string) bool {
	if key == "" || key == "." || key == ".." {
		return false
	}
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-', r == '.':
		default:
			return false
		}
	}
	return true
}

func (h *EngineHost) StorageRead(key string) ([]byte, bool) {
	if !StorageKeyOK(key) {
		hostWarnf(h, "storage key %q rejected (must be [A-Za-z0-9_.-]+)", key)
		return nil, false
	}
	var blob storageBlob
	if err := h.ctx.RT.Store.Read(h.id.CartSlug, "cart-"+key, &blob); err != nil {
		return nil, false
	}
	if blob.Data == nil {
		return nil, false
	}
	return blob.Data, true
}

func (h *EngineHost) StorageWrite(key string, val []byte) {
	if !StorageKeyOK(key) {
		hostWarnf(h, "storage key %q rejected (must be [A-Za-z0-9_.-]+)", key)
		return
	}
	_ = h.ctx.RT.Store.Write(h.id.CartSlug, "cart-"+key, storageBlob{Data: val})
}

// NowMicros returns the engine clock, never wall time, in microseconds.
func (h *EngineHost) NowMicros() uint64 { return uint64(h.ctx.Now().UnixMicro()) }

// RandomU64 draws from the engine's seeded RNG through CartRandomU64, so the
// flight recorder can capture and replay each value. RT.Rand is shared with
// the rest of the engine, so the seed alone cannot restore the position.
func (h *EngineHost) RandomU64() uint64 { return h.ctx.RT.CartRandomU64() }

func (h *EngineHost) Log(level LogLevel, msg string) {
	h.ctx.RT.Log(logTag(level), fmt.Sprintf("[cart:%s] %s", h.id.CartSlug, msg))
}

func (h *EngineHost) SetText(seats input.Seats, line1, line2 string) {
	h.ctx.SetText(seats, line1, line2)
}

func (h *EngineHost) SetButtonLights(seats input.Seats, lights map[string]bool, only bool) {
	h.ctx.SetButtonLights(seats, hardware.ButtonLights(lights), only)
}

func (h *EngineHost) SetLights(seats input.Seats, on bool) {
	h.ctx.SetLights(seats, on)
}

// SetLampLevels widens the ABI's 4-bit levels to the hardware's 8-bit ones
// and hands them to the engine's linear fade, which owns the cabinet's
// brightness policy.
func (h *EngineHost) SetLampLevels(seats input.Seats, levels [6]uint8, touch uint8, fadeMs int) {
	var lv hardware.LampLevels
	for i, v := range levels {
		lv[i] = hardware.LampLevelFromABI(v)
	}
	h.ctx.FadeLampsEased(seats, lv, hardware.LightMask(touch), fadeMs, engine.Linear)
}

// SetCartGlyphs installs the cartridge's glyphs as the hardware overlay, or
// clears it with nil. A Suite that is not a hardware.GlyphPager shows nothing.
func (h *EngineHost) SetCartGlyphs(glyphs map[rune][8]byte) {
	g, ok := h.ctx.RT.Hardware.(hardware.GlyphPager)
	if !ok {
		return
	}
	if glyphs == nil {
		g.SetCartGlyphs(nil)
		return
	}
	out := make(map[rune]hardware.GlyphRows, len(glyphs))
	for r, rows := range glyphs {
		out[r] = rows
	}
	g.SetCartGlyphs(out)
}

var _ GlyphDefiner = (*EngineHost)(nil)

func logTag(level LogLevel) string {
	switch level {
	case LogDebug:
		return "debug"
	case LogWarn:
		return "warn"
	case LogError:
		return "error"
	default:
		return "info"
	}
}

var _ ChannelSource = (*EngineHost)(nil)

// channelBus is nil with no HID manager; its methods are nil-safe.
func (h *EngineHost) channelBus() *controller.ChannelBus {
	if h.ctx == nil || h.ctx.RT == nil || h.ctx.RT.Controllers == nil {
		return nil
	}
	return h.ctx.RT.Controllers.Channels()
}

func (h *EngineHost) OfferedChannel(name string) (ChannelKind, int, bool) {
	kind, size, ok := h.channelBus().Offered(name)
	if !ok {
		return 0, 0, false
	}
	wire, ok := kindFromAsset(kind)
	if !ok {
		return 0, 0, false
	}
	return wire, size, true
}

// SampleChannel reads one scalar channel, resetting a delta's accumulator.
func (h *EngineHost) SampleChannel(name string) int32 { return h.channelBus().Read(name) }

func (h *EngineHost) SampleChannelBlob(name string, dst []byte) int {
	return h.channelBus().ReadBlob(name, dst)
}

func (h *EngineHost) SetSessionProfiles(profiles []*controller.Spec) {
	if h.ctx == nil || h.ctx.RT == nil || h.ctx.RT.Controllers == nil {
		return
	}
	h.ctx.RT.Controllers.SetSessionProfiles(profiles)
}

var _ MicModelSink = (*EngineHost)(nil)

// SetSessionMicModels installs, or with nil drops, the cartridge's sound
// models. With no capture device it does nothing.
func (h *EngineHost) SetSessionMicModels(models []*assetfile.Envelope) {
	if h.ctx == nil || h.ctx.RT == nil || h.ctx.RT.Mic == nil {
		return
	}
	h.ctx.RT.Mic.SetSessionModels(models)
}
