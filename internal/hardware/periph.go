// SPDX-License-Identifier: GPL-2.0-or-later

package hardware

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
)

// LightMask is one seat's lit buttons as a bitmask: bit i is LightButtons[i].
// ButtonLights is the partial write API; this is the whole-seat read and
// transport shape.
type LightMask uint8

// AllLights is every button in LightButtons lit.
var AllLights = LightMask(1<<uint(len(LightButtons))) - 1

// Has reports whether the button at LightButtons position i is lit. An index
// outside the roster is not lit.
func (m LightMask) Has(i int) bool {
	if i < 0 || i >= len(LightButtons) {
		return false
	}
	return m&(1<<uint(i)) != 0
}

func (m LightMask) Names() []string {
	var out []string
	for i, name := range LightButtons {
		if m.Has(i) {
			out = append(out, name)
		}
	}
	return out
}

func (m LightMask) String() string {
	names := m.Names()
	if len(names) == 0 {
		return "-"
	}
	return strings.Join(names, "+")
}

// ButtonLights expands the mask into a ButtonLights map naming every button,
// so writing it leaves no lamp unmentioned.
func (m LightMask) ButtonLights() ButtonLights {
	out := make(ButtonLights, len(LightButtons))
	for i, name := range LightButtons {
		out[name] = m.Has(i)
	}
	return out
}

// LightMaskOf packs a per-position state slice into a mask. Extra entries are
// ignored; missing ones read as unlit.
func LightMaskOf(states []bool) LightMask {
	var m LightMask
	for i := range LightButtons {
		if i < len(states) && states[i] {
			m |= 1 << uint(i)
		}
	}
	return m
}

// LightMaskFromNames packs a list of button names; an unknown name is an
// error.
func LightMaskFromNames(names []string) (LightMask, error) {
	var m LightMask
	for _, n := range names {
		i := buttonIndex(n)
		if i < 0 {
			return 0, fmt.Errorf("unknown button %q (want one of %s)", n, strings.Join(LightButtons, ", "))
		}
		m |= 1 << uint(i)
	}
	return m, nil
}

// MotionSensor is optionally implemented by a Suite with a presence sensor
// the standby machine may poll. Virtual does not implement it: its presence
// is the input spy, and a second presence source would wake what the cabinet
// cannot.
type MotionSensor interface {
	MotionActive() bool
}

// MotionHistory is optionally implemented by a Suite that remembers when
// presence was last seen, for status displays rather than for waking.
type MotionHistory interface {
	LastMotion() (time.Time, bool)
}

// InputSpy is optionally implemented by a Suite that wants to hear about
// every input event, so a press counts as presence on hardware whose motion
// sensing is otherwise blind to it.
type InputSpy interface {
	InputSignal()
}

// ThermalSensor is optionally implemented by a Suite that samples the SoC
// temperature.
type ThermalSensor interface {
	CPUTempF() (degF float64, ok bool)
}

// MicInfo describes a cabinet's capture path, or says there is none.
type MicInfo struct {
	// Available is false when the suite has no microphone; other fields are
	// then meaningless.
	Available bool `json:"available"`
	// Source names the capture device: "virtual" or an ALSA name ("hw:1,0").
	Source string `json:"source"`
	// Synthetic marks a source that generates levels rather than measuring a
	// room.
	Synthetic bool `json:"synthetic"`
	// SampleRateHz is the capture rate the levels are computed from, or 0 for
	// a synthetic source.
	SampleRateHz int `json:"sampleRateHz"`
	FrameHz      int `json:"frameHz"`
	// WindowMs is the analysis window each frame's RMS covers.
	WindowMs int `json:"windowMs"`
}

// MicLevel is one coarse level frame. It carries levels, not samples: raw
// audio must never reach a running cartridge, since a sim fed a microphone
// cannot be replayed.
type MicLevel struct {
	// Seq counts frames from the source's start; a gap means the reader
	// polled slower than frames were produced.
	Seq uint64 `json:"seq"`
	// RMS is the window's root-mean-square level, 0..1 against full scale.
	RMS float64 `json:"rms"`
	// Peak is the largest absolute sample in the window, 0..1.
	Peak float64 `json:"peak"`
	// Clipped reports that the window contained full-scale samples.
	Clipped bool `json:"clipped"`
}

// MicSource is the optional microphone extension a Suite may implement. An
// implementation owes:
//
//   - Its own capture goroutine; MicLevel reads the latest frame and may not
//     block on a device.
//   - Levels computed over a fixed window (WindowMs).
//   - A fail-soft open: no capture device is MicInfo{Available:false}, not a
//     boot failure.
//   - Nothing that reaches a running cartridge.
type MicSource interface {
	MicInfo() MicInfo
	// MicLevel reports the most recent level frame, or ok=false when the
	// source has produced none. Safe from any goroutine and never blocking.
	MicLevel() (MicLevel, bool)
}

// MicBandCount is how many bands a MicBander reports.
const MicBandCount = 8

// MicBandCentersHz are the MicBander band centres, log-spaced from 80 Hz to
// 4 kHz. Each band runs between the geometric means of its neighbours, so the
// eight tile 60 Hz to 5.3 kHz without gaps.
var MicBandCentersHz = [MicBandCount]float64{80, 140, 240, 420, 740, 1300, 2280, 4000}

// MicBander is the optional spectrum extension a MicSource may implement, one
// band power per MicBandCentersHz entry over the same span as MicLevel.
type MicBander interface {
	// MicBands reports the latest band powers, normalised so a full-scale
	// sine inside a band reads about 1, or ok=false before the first
	// spectrum. These are powers, not amplitudes. A source that publishes
	// less often than it analyses reports the band's peak over the span.
	// Safe from any goroutine and never blocking.
	MicBands() ([MicBandCount]float64, bool)
}

const (
	virtualMicFrameHz  = 20
	virtualMicWindowMs = 1000 / virtualMicFrameHz
	virtualMicFloor    = 0.02
	// coprime periods so the transients drift against each other
	virtualMicClapEvery    = 7 * time.Second
	virtualMicWhistleEvery = 11 * time.Second
	virtualMicClapDecay    = 250 * time.Millisecond
	virtualMicWhistleHold  = 600 * time.Millisecond
)

// VirtualMic is the synthetic MicSource for desks without a cabinet: a quiet
// room with a clap every seven seconds and a whistle every eleven. It is a
// pure function of elapsed time, with no goroutine.
type VirtualMic struct {
	mu    sync.Mutex
	start time.Time
	now   func() time.Time
}

func NewVirtualMic() *VirtualMic { return &VirtualMic{start: time.Now(), now: time.Now} }

func (v *VirtualMic) setClock(start time.Time, now func() time.Time) {
	v.mu.Lock()
	v.start, v.now = start, now
	v.mu.Unlock()
}

func (v *VirtualMic) MicInfo() MicInfo {
	return MicInfo{
		Available: true,
		Source:    "virtual",
		Synthetic: true,
		FrameHz:   virtualMicFrameHz,
		WindowMs:  virtualMicWindowMs,
	}
}

// frameAt quantizes elapsed time to the frame instant MicLevel and MicBands
// share, so calls inside one frame agree.
func (v *VirtualMic) frameAt() (seq uint64, at time.Duration) {
	v.mu.Lock()
	elapsed := v.now().Sub(v.start)
	v.mu.Unlock()
	if elapsed < 0 {
		elapsed = 0
	}
	seq = uint64(elapsed / (time.Second / virtualMicFrameHz))
	at = time.Duration(seq) * (time.Second / virtualMicFrameHz)
	return seq, at
}

func (v *VirtualMic) MicLevel() (MicLevel, bool) {
	seq, at := v.frameAt()

	rms := virtualMicFloor + 0.008*math.Sin(float64(at)/float64(time.Second)*2*math.Pi/3)
	peak := rms * 2

	if d := at % virtualMicClapEvery; d < virtualMicClapDecay {
		fall := 1 - float64(d)/float64(virtualMicClapDecay)
		rms += 0.55 * fall * fall
		peak = math.Max(peak, 0.35+0.6*fall)
	}
	if d := at % virtualMicWhistleEvery; d < virtualMicWhistleHold {
		rms += 0.30
		peak = math.Max(peak, rms*1.42)
	}
	return MicLevel{
		Seq:     seq,
		RMS:     math.Min(rms, 1),
		Peak:    math.Min(peak, 1),
		Clipped: peak >= 1,
	}, true
}

// MicBands computes a synthetic spectrum: a hump sliding across the bands,
// the clap spreading energy into every band and the whistle concentrating it
// in one.
func (v *VirtualMic) MicBands() ([MicBandCount]float64, bool) {
	seq, at := v.frameAt()

	level := virtualMicFloor + 0.008*math.Sin(float64(at)/float64(time.Second)*2*math.Pi/3)
	center := float64(seq%MicBandCount) + 0.5*math.Sin(float64(at)/float64(time.Second)*2*math.Pi/5)

	var bands [MicBandCount]float64
	for i := range bands {
		d := float64(i) - center
		bands[i] = 4 * level * math.Exp(-0.5*d*d)
	}
	if d := at % virtualMicClapEvery; d < virtualMicClapDecay {
		fall := 1 - float64(d)/float64(virtualMicClapDecay)
		for i := range bands {
			bands[i] += 0.4 * fall * fall
		}
	}
	if d := at % virtualMicWhistleEvery; d < virtualMicWhistleHold {
		bands[4] += 0.9
	}
	return bands, true
}

var _ MicSource = (*VirtualMic)(nil)
var _ MicBander = (*VirtualMic)(nil)

// MicSlot holds a suite's optional microphone and answers MicSource and
// MicBander for it. Embed it in a suite; the zero value has no microphone.
type MicSlot struct {
	src MicSource // read without a lock, so attach before the suite reaches an engine
}

// AttachMic replaces the microphone; nil removes it. Call it before handing
// the suite to an engine.
func (m *MicSlot) AttachMic(src MicSource) { m.src = src }

func (m *MicSlot) MicInfo() MicInfo {
	if m.src == nil {
		return MicInfo{}
	}
	return m.src.MicInfo()
}

func (m *MicSlot) MicLevel() (MicLevel, bool) {
	if m.src == nil {
		return MicLevel{}, false
	}
	return m.src.MicLevel()
}

// MicBands reports the attached microphone's latest spectrum, or ok=false if
// there is none or it is not a MicBander.
func (m *MicSlot) MicBands() ([MicBandCount]float64, bool) {
	b, ok := m.src.(MicBander)
	if !ok {
		return [MicBandCount]float64{}, false
	}
	return b.MicBands()
}

var _ MicSource = (*MicSlot)(nil)
var _ MicBander = (*MicSlot)(nil)
