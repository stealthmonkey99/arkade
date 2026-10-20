// SPDX-License-Identifier: GPL-2.0-or-later

package assetfile

import (
	"errors"
	"fmt"
	"math"

	"github.com/stealthmonkey99/arkade/internal/lcdrune"
)

// Validate checks env against the per-kind rules in docs/ASSETS.md and
// returns an error for anything structural. It never modifies env:
// presentation values (holds, fades, loop modes) are repaired, not refused,
// and that is Normalize's job. wantKind is the kind the filename names; pass
// "" to skip that check. imgW and imgH are the carrying image's pixel
// bounds, used for sprites only.
func Validate(env *Envelope, wantKind Kind, imgW, imgH int) error {
	if env.Format != FormatVersion {
		return fmt.Errorf("assetfile: envelope format %d, this build reads %d", env.Format, FormatVersion)
	}
	if !env.Kind.valid() {
		return fmt.Errorf("assetfile: unknown kind %q", env.Kind)
	}
	if wantKind != "" && env.Kind != wantKind {
		return fmt.Errorf("assetfile: envelope kind %q does not match the filename's %q", env.Kind, wantKind)
	}
	switch env.Kind {
	case KindSprite:
		return validateSprite(&env.SpritePayload, imgW, imgH)
	case KindSound:
		return validateSound(env)
	case KindGlyph:
		return validateGlyph(&env.GlyphPayload)
	case KindHID:
		return validateHID(&env.HIDPayload)
	case KindLevels:
		return validateLevels(env)
	case KindLights:
		return validateLights(&env.LightsPayload)
	case KindMic:
		return validateMic(&env.MicPayload)
	default:
		return fmt.Errorf("assetfile: unknown kind %q", env.Kind)
	}
}

// validated is what every reader applies: Validate, then Normalize.
func validated(env *Envelope, wantKind Kind, imgW, imgH int) error {
	if err := Validate(env, wantKind, imgW, imgH); err != nil {
		return err
	}
	Normalize(env)
	return nil
}

// validateLights checks a lamp sequence against sdk/docs/reference-lights.md.
// Masks are structural; holds, fades and the loop mode are Normalize's.
func validateLights(p *LightsPayload) error {
	if len(p.Frames) == 0 {
		return errors.New("assetfile: lights envelope has no frames")
	}
	if len(p.Frames) > MaxLightFrames {
		return fmt.Errorf("assetfile: lights envelope has %d frames, over the %d cap", len(p.Frames), MaxLightFrames)
	}
	for i, f := range p.Frames {
		named := false
		for seat := 0; seat < 2; seat++ {
			mask, ok := f.Lamps.Mask(seat)
			if !ok {
				continue
			}
			named = true
			if mask < 0 || mask > MaxLightMask {
				return fmt.Errorf("assetfile: lights frame %d gives seat p%d the mask %d, want within 0..%d (%d lamps)",
					i, seat+1, mask, MaxLightMask, LightButtonCount)
			}
		}
		if !named {
			return fmt.Errorf("assetfile: lights frame %d names neither seat; a frame that changes nothing is a longer hold on the frame before it", i)
		}
	}
	return nil
}

// validateLevels requires only a pinned generator; the pack body is parsed
// by sdk/levels/packfile.
func validateLevels(env *Envelope) error {
	if env.Generator.Tool == "" || env.Generator.Version == "" {
		return errors.New("assetfile: a levels envelope needs a pinned generator (tool and version)")
	}
	return nil
}

// validateSprite checks cel rectangles against the image bounds and every
// animation's cels against the named cels.
func validateSprite(p *SpritePayload, imgW, imgH int) error {
	if len(p.Cels) == 0 {
		return errors.New("assetfile: sprite envelope has no cels")
	}
	byName := make(map[string]Cel, len(p.Cels))
	for _, c := range p.Cels {
		if c.Name == "" {
			return errors.New("assetfile: sprite cel has no name")
		}
		if _, dup := byName[c.Name]; dup {
			return fmt.Errorf("assetfile: sprite cel name %q is used twice", c.Name)
		}
		if c.W <= 0 || c.H <= 0 {
			return fmt.Errorf("assetfile: cel %q has non-positive size %dx%d", c.Name, c.W, c.H)
		}
		if c.X < 0 || c.Y < 0 || c.X+c.W > imgW || c.Y+c.H > imgH {
			return fmt.Errorf("assetfile: cel %q rectangle (%d,%d,%d,%d) lies outside the %dx%d image",
				c.Name, c.X, c.Y, c.W, c.H, imgW, imgH)
		}
		byName[c.Name] = c
	}
	for _, a := range p.Animations {
		if a.Name == "" {
			return errors.New("assetfile: animation has no name")
		}
		if len(a.Frames) == 0 {
			return fmt.Errorf("assetfile: animation %q has no frames", a.Name)
		}
		for _, f := range a.Frames {
			if _, ok := byName[f.Cel]; !ok {
				return fmt.Errorf("assetfile: animation %q references cel %q, which is not in the sheet", a.Name, f.Cel)
			}
		}
	}
	return nil
}

// validateSound checks the render block and the recipe's structure, leaving
// node params to the generator.
func validateSound(env *Envelope) error {
	if env.Render == nil {
		return errors.New("assetfile: sound envelope has no render block")
	}
	r := env.Render
	if r.SampleRate <= 0 {
		return fmt.Errorf("assetfile: sound sampleRate %d must be positive", r.SampleRate)
	}
	if r.Channels != 1 && r.Channels != 2 {
		return fmt.Errorf("assetfile: sound channels %d must be 1 or 2", r.Channels)
	}
	switch r.Bits {
	case 8, 16, 24, 32:
	default:
		return fmt.Errorf("assetfile: sound bits %d is not 8, 16, 24 or 32", r.Bits)
	}
	recipe, err := env.SoundRecipe()
	if err != nil {
		return err
	}
	return ValidateSoundRecipe(recipe)
}

// validateGlyph checks the code point and row bounds. The code point rule is
// internal/lcdrune's, shared with host_define_lcd_glyph, so a file that packs
// also mounts.
func validateGlyph(p *GlyphPayload) error {
	cp := p.Codepoint
	if cp < 0 || cp > 0x10FFFF {
		return fmt.Errorf("assetfile: glyph codepoint %d is not a code point", cp)
	}
	switch why := lcdrune.Reason(rune(cp)); why {
	case "":
	case lcdrune.ReasonASCII:
		return fmt.Errorf("assetfile: glyph codepoint U+%04X is below U+0080", cp)
	case lcdrune.ReasonNotACodePoint:
		return fmt.Errorf("assetfile: glyph codepoint U+%04X is a surrogate or noncharacter, not a code point", cp)
	default:
		return fmt.Errorf("assetfile: glyph codepoint U+%04X is %s — an LCD glyph rune must be a plain, single-width, monochrome character", cp, why)
	}
	if len(p.Rows) != 8 {
		return fmt.Errorf("assetfile: glyph has %d rows, want 8", len(p.Rows))
	}
	for i, v := range p.Rows {
		if v < 0 || v > 0x1f {
			return fmt.Errorf("assetfile: glyph row %d is %d, want within 0..0x1f (5 pixels wide)", i, v)
		}
	}
	return nil
}

// validateMic checks a sound model: a runnable front end, an endpointer, and
// classes whose templates match the front end's frame width. Nothing is
// repaired, because a nearly right model mounts and then never fires.
func validateMic(p *MicPayload) error {
	f := p.Features
	if f == nil {
		return errors.New("assetfile: mic envelope has no features block")
	}
	if f.SampleRateHz != MicRateHz {
		return fmt.Errorf("assetfile: mic sampleRateHz %d, want %d (the cabinet's analysis rate)", f.SampleRateHz, MicRateHz)
	}
	if f.FrameLen < 128 || f.FrameLen > 2048 || f.FrameLen&(f.FrameLen-1) != 0 {
		return fmt.Errorf("assetfile: mic frameLen %d, want a power of two within 128..2048", f.FrameLen)
	}
	if f.HopLen <= 0 || f.HopLen > f.FrameLen {
		return fmt.Errorf("assetfile: mic hopLen %d, want within 1..%d (the frame length)", f.HopLen, f.FrameLen)
	}
	if f.MelBands < 1 || f.MelBands > MaxMicBands {
		return fmt.Errorf("assetfile: mic melBands %d, want within 1..%d", f.MelBands, MaxMicBands)
	}
	if f.Coeffs < 1 || f.Coeffs > f.MelBands {
		return fmt.Errorf("assetfile: mic coeffs %d, want within 1..%d (the mel bands they come from)", f.Coeffs, f.MelBands)
	}
	if f.Coeffs > MaxMicCoeffs {
		return fmt.Errorf("assetfile: mic coeffs %d, over the %d cap", f.Coeffs, MaxMicCoeffs)
	}
	nyquist := float64(f.SampleRateHz) / 2
	if !finite(f.MinHz) || !finite(f.MaxHz) || f.MinHz < 0 || f.MaxHz <= f.MinHz || f.MaxHz > nyquist {
		return fmt.Errorf("assetfile: mic band %g..%g Hz, want 0 <= minHz < maxHz <= %g (half the sample rate)", f.MinHz, f.MaxHz, nyquist)
	}

	s := p.Segment
	if s == nil {
		return errors.New("assetfile: mic envelope has no segment block")
	}
	if s.MinMs <= 0 {
		return fmt.Errorf("assetfile: mic segment minMs %d, want a positive length", s.MinMs)
	}
	if s.MaxMs <= s.MinMs {
		return fmt.Errorf("assetfile: mic segment maxMs %d, want longer than minMs %d", s.MaxMs, s.MinMs)
	}
	if s.HangoverMs < 0 {
		return fmt.Errorf("assetfile: mic segment hangoverMs %d, want zero or more", s.HangoverMs)
	}
	if !finite(s.SNR) || s.SNR < 1 {
		return fmt.Errorf("assetfile: mic segment snr %g, want 1 or more (a sound at 1 is as loud as the room)", s.SNR)
	}
	if !finite(s.MinRMS) || s.MinRMS < 0 || s.MinRMS > 1 {
		return fmt.Errorf("assetfile: mic segment minRms %g, want within 0..1 (full scale)", s.MinRMS)
	}

	if len(p.Classes) == 0 {
		return errors.New("assetfile: mic envelope has no classes")
	}
	if len(p.Classes) > MaxMicClasses {
		return fmt.Errorf("assetfile: mic envelope has %d classes, over the %d cap (one per cue key)", len(p.Classes), MaxMicClasses)
	}
	seenName := make(map[string]bool, len(p.Classes))
	seenCue := make(map[int]string, len(p.Classes))
	for _, c := range p.Classes {
		if c.Name == "" {
			return errors.New("assetfile: mic class has no name")
		}
		if !validChannelName(c.Name) {
			return fmt.Errorf("assetfile: mic class name %q is not lowercase words joined by . _ or -", c.Name)
		}
		if seenName[c.Name] {
			return fmt.Errorf("assetfile: mic class name %q is used twice", c.Name)
		}
		seenName[c.Name] = true
		if c.Cue < 0 || c.Cue >= MaxMicClasses {
			return fmt.Errorf("assetfile: mic class %q has cue %d, want within 0..%d (mic.cue0..mic.cue%d)", c.Name, c.Cue, MaxMicClasses-1, MaxMicClasses-1)
		}
		if other, dup := seenCue[c.Cue]; dup {
			return fmt.Errorf("assetfile: mic classes %q and %q both fire cue %d", other, c.Name, c.Cue)
		}
		seenCue[c.Cue] = c.Name
		if !finite(c.Threshold) || c.Threshold <= 0 {
			return fmt.Errorf("assetfile: mic class %q has threshold %g, want a positive match distance", c.Name, c.Threshold)
		}
		if len(c.Templates) == 0 {
			return fmt.Errorf("assetfile: mic class %q has no templates", c.Name)
		}
		if len(c.Templates) > MaxMicTemplates {
			return fmt.Errorf("assetfile: mic class %q has %d templates, over the %d cap", c.Name, len(c.Templates), MaxMicTemplates)
		}
		for ti, tmpl := range c.Templates {
			if len(tmpl) < 2 || len(tmpl) > MaxMicTemplateFrames {
				return fmt.Errorf("assetfile: mic class %q template %d has %d frames, want within 2..%d", c.Name, ti, len(tmpl), MaxMicTemplateFrames)
			}
			for fi, frame := range tmpl {
				if len(frame) != f.Coeffs {
					return fmt.Errorf("assetfile: mic class %q template %d frame %d has %d coefficients, want the %d features declares", c.Name, ti, fi, len(frame), f.Coeffs)
				}
				for ci, v := range frame {
					if !finite(v) {
						return fmt.Errorf("assetfile: mic class %q template %d frame %d coefficient %d is %g, not a finite number", c.Name, ti, fi, ci, v)
					}
				}
			}
		}
	}
	return nil
}

// finite exists because a very large JSON literal can decode to infinity.
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// validateHID checks a profile against sdk/docs/reference-hid.md. Nothing is
// repaired, because a nearly right wiring silently reads the wrong bits.
func validateHID(p *HIDPayload) error {
	if len(p.Channels) == 0 {
		return errors.New("assetfile: hid envelope has no channels")
	}
	if p.Device != nil {
		switch p.Device.Bus {
		case "", BusUSB, BusBluetooth:
		default:
			return fmt.Errorf("assetfile: hid device bus %q, want %q, %q, or empty for any", p.Device.Bus, BusUSB, BusBluetooth)
		}
	}
	if len(p.Channels) > MaxProfileChannels {
		return fmt.Errorf("assetfile: hid envelope has %d channels, over the %d cap", len(p.Channels), MaxProfileChannels)
	}
	seen := make(map[string]ChannelKind, len(p.Channels))
	for _, c := range p.Channels {
		if c.Name == "" {
			return errors.New("assetfile: hid channel has no name")
		}
		if _, dup := seen[c.Name]; dup {
			return fmt.Errorf("assetfile: hid channel name %q is used twice", c.Name)
		}
		if !validChannelName(c.Name) {
			return fmt.Errorf("assetfile: hid channel name %q is not lowercase words joined by . _ or - (and may not start with a p1./p2. seat prefix, which the host adds)", c.Name)
		}
		switch c.Kind {
		case ChannelButton, ChannelAxis, ChannelDelta, ChannelBlob:
		default:
			return fmt.Errorf("assetfile: hid channel %q has unknown kind %q", c.Name, c.Kind)
		}
		if c.Seat != 0 && c.Seat != 1 {
			return fmt.Errorf("assetfile: hid channel %q has seat %d, want 0 or 1", c.Name, c.Seat)
		}
		if err := validateReport(c.Name, c.Kind, c.Report); err != nil {
			return err
		}
		seen[c.Name] = c.Kind
	}
	for from, to := range p.Standard {
		kind, ok := seen[from]
		if !ok {
			return fmt.Errorf("assetfile: standard alias %q references a channel that is not in channels", from)
		}
		isAxis, known := StandardName(to)
		if !known {
			return fmt.Errorf("assetfile: standard alias %q targets %q, which is not a standard panel name", from, to)
		}
		// An axis aliased onto a panel button would need a threshold the
		// profile never states.
		switch {
		case kind == ChannelBlob:
			return fmt.Errorf("assetfile: standard alias %q is a blob channel, which has no panel equivalent", from)
		case isAxis && kind == ChannelButton:
			return fmt.Errorf("assetfile: standard alias %q is a button but %q is an axis; alias a button onto a direction (%s) instead", from, to, StdJoyNorth)
		case !isAxis && kind != ChannelButton:
			return fmt.Errorf("assetfile: standard alias %q is a %s but %q is a panel button; a panel button needs a button channel", from, kind, to)
		}
	}
	return nil
}

func validateReport(name string, kind ChannelKind, r Report) error {
	if r.Byte < 0 || r.Byte > MaxReportByte {
		return fmt.Errorf("assetfile: hid channel %q reads byte %d, want within 0..%d", name, r.Byte, MaxReportByte)
	}
	if r.Bit < 0 || r.Bit > 7 {
		return fmt.Errorf("assetfile: hid channel %q reads bit %d, want within 0..7", name, r.Bit)
	}
	switch kind {
	case ChannelButton:
		if r.Bits > 1 {
			return fmt.Errorf("assetfile: hid channel %q is a button of %d bits; a button is one bit (use an axis for a wider field)", name, r.Bits)
		}
		if r.Size != 0 || r.Signed || len(r.Map) != 0 || r.Bias != 0 || r.Divide != 0 {
			return fmt.Errorf("assetfile: hid channel %q is a button, which reads one bit and takes no size, signed, map, bias or divide", name)
		}
	case ChannelBlob:
		if r.Size <= 0 || r.Size > MaxBlobBytes {
			return fmt.Errorf("assetfile: hid channel %q is a blob of %d bytes, want within 1..%d", name, r.Size, MaxBlobBytes)
		}
		if r.Bits != 0 || r.Signed || len(r.Map) != 0 || r.Bias != 0 || r.Divide != 0 {
			return fmt.Errorf("assetfile: hid channel %q is a blob, which copies bytes and takes no bits, signed, map, bias or divide", name)
		}
	case ChannelAxis, ChannelDelta:
		switch {
		case r.Bits != 0 && r.Size != 0:
			return fmt.Errorf("assetfile: hid channel %q sets both bits and size; a field is either a run of bits or a run of bytes", name)
		case r.Bits != 0:
			if r.Bits < 1 || r.Bits > 8 {
				return fmt.Errorf("assetfile: hid channel %q is %d bits wide, want within 1..8", name, r.Bits)
			}
			if r.Bit+r.Bits > 8 {
				return fmt.Errorf("assetfile: hid channel %q reads %d bits from bit %d, which runs past byte %d", name, r.Bits, r.Bit, r.Byte)
			}
			if r.Signed {
				return fmt.Errorf("assetfile: hid channel %q is a bit field, which is unsigned (use map or bias to recentre it)", name)
			}
		default:
			if r.Size < 0 || r.Size > MaxAxisBytes {
				return fmt.Errorf("assetfile: hid channel %q reads %d bytes, want within 1..%d", name, r.Size, MaxAxisBytes)
			}
		}
		if r.Divide < 0 {
			return fmt.Errorf("assetfile: hid channel %q divides by %d, want a positive divisor (or 0 for none)", name, r.Divide)
		}
		if len(r.Map) > 1<<16 {
			return fmt.Errorf("assetfile: hid channel %q has a %d-entry map, which is longer than any report field can index", name, len(r.Map))
		}
	}
	return nil
}
