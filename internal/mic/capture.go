// SPDX-License-Identifier: GPL-2.0-or-later

package mic

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/stealthmonkey99/arkade/assetfile"
	"github.com/stealthmonkey99/arkade/internal/dsp"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
)

// Capture runs at the mixer's rate so the playback tap is the echo reference
// without resampling; both streams decimate by the same integer to Rate.
const (
	CaptureRate = 44100
	Decimation  = 3
	Rate        = CaptureRate / Decimation
)

// decimatorTaps: 63 taps at 44.1 kHz is a 1.4 ms group delay and over 40 dB of
// stop-band rejection; both streams share it so the delay cancels out.
const decimatorTaps = 63

// ringSeconds must exceed the delay search range plus the correlation window.
const ringSeconds = 3

// Options configures a Capture. Zero values take the defaults.
type Options struct {
	// Device is the ALSA capture device arecord opens, e.g. "plughw:0,0".
	Device string
	// Detector configures the whistle/clap/blow gesture detector.
	Detector Config
	// Canceller configures the echo canceller.
	Canceller CancellerConfig
	// Events, when non-nil, receives every gesture and cue as a keypress
	// (input.KeyMic*). A full queue drops the event rather than block.
	Events chan<- input.Event
	// Log receives one-line diagnostics; nil discards them.
	Log func(string)
	// NoCancel bypasses the echo canceller, so the raw microphone reaches
	// the detector.
	NoCancel bool
}

// Block is what a listener sees once per analysis frame: the frame's raw
// and cancelled samples at Rate, and everything the pipeline decided about
// it. The slices are only valid during the callback.
type Block struct {
	Seq     uint64
	Raw     []float64
	Clean   []float64
	Echo    bool // the frame was judged to be the cabinet hearing itself
	Cancel  bool // the canceller is in the path (Options.NoCancel is off)
	ERLE    float64
	Delay   int
	Aligned bool

	RMS, Floor, PeakHz, Tonality float64
	Event                        Event   // gesture fired on this frame, or EventNone
	Cue                          int     // sound-model class fired on this frame, or -1
	CueName                      string  // that class's name
	CueScore                     float64 // its DTW distance
	Speech                       bool    // the classifier's activity gate is open
}

// Stats is a snapshot of the pipeline for status pages.
type Stats struct {
	Running  bool    `json:"running"`
	Device   string  `json:"device"`
	Frames   uint64  `json:"frames"`
	ERLE     float64 `json:"erleDb"`
	Delay    int     `json:"delaySamples"`
	Aligned  bool    `json:"aligned"`
	Score    float64 `json:"alignScore"`
	TryScore float64 `json:"tryScore"`
	TryShape float64 `json:"tryShape"`
	Ref      int     `json:"refSamples"`
	Cancel   bool    `json:"cancel"`
	Events   uint64  `json:"events"`
	Models   int     `json:"models"`
	Restarts int     `json:"restarts"`
	// Calibration is the outcome of the last stinger: whether the capture
	// aligned on it within calibrateWindowMs, and what it reached.
	Calibration Calibration `json:"calibration"`
}

// Capture owns the cabinet's microphone: an arecord subprocess, decimation to
// Rate, and the meter, echo judge, calibration watcher, gesture detector,
// sound models and listeners fed from it. It implements hardware.MicSource,
// hardware.MicBander and engine.MicModels. arecord rather than an ALSA binding
// keeps cgo out of the engine build.
type Capture struct {
	opts  Options
	open  opener
	retry time.Duration

	refDec  *dsp.Decimator
	refRing *ring
	refMono []float64
	refOut  []float64
	tapMu   sync.Mutex

	micDec  *dsp.Decimator
	micRing *ring
	echo    *echoJudge
	det     *Detector
	cls     atomic.Pointer[Classifier]
	models  atomic.Int32
	meter   *levelMeter
	calib   *calibrationWatcher

	mu        sync.Mutex
	listeners map[int]func(Block)
	nextID    int
	running   bool
	restarts  int
	frames    uint64
	aecStats  CancellerStats // published once per frame by the capture goroutine

	events      atomic.Uint64
	sink        atomic.Pointer[chan<- input.Event]
	recalibrate atomic.Pointer[func()]
}

// NewCapture builds a Capture. Nothing opens until Run.
func NewCapture(opts Options) *Capture {
	if opts.Log == nil {
		opts.Log = func(string) {}
	}
	if opts.Detector.SampleRate == 0 {
		opts.Detector = DefaultConfig()
	}
	if opts.Canceller.Taps == 0 {
		opts.Canceller = DefaultCancellerConfig()
	}
	c := &Capture{
		opts:      opts,
		retry:     restartDelay,
		refDec:    dsp.NewDecimator(Decimation, decimatorTaps),
		refRing:   newRing(ringSeconds * Rate),
		micDec:    dsp.NewDecimator(Decimation, decimatorTaps),
		micRing:   newRing(ringSeconds * Rate),
		det:       NewDetector(opts.Detector),
		meter:     newLevelMeter(),
		calib:     newCalibrationWatcher(opts.Detector.FrameLen, opts.Log),
		listeners: map[int]func(Block){},
	}
	c.open = c.openArecord
	c.echo = newEchoJudge(opts.Detector, opts.Canceller, c.refRing, c.micRing, opts.NoCancel)
	if opts.Events != nil {
		c.SetEvents(opts.Events)
	}
	return c
}

// SetEvents routes gestures and cues into ch, typically input.Handler.Events().
// It may be called after NewCapture.
func (c *Capture) SetEvents(ch chan<- input.Event) {
	c.sink.Store(&ch)
}

// SetRecalibrate installs fn to replay the stinger. Run calls it once a
// device it had been hearing comes back, never on the first start; it runs
// on the capture goroutine and must not block.
func (c *Capture) SetRecalibrate(fn func()) { c.recalibrate.Store(&fn) }

// OutputTap returns the function to hand to audio.Mixer.SetOutputTap. It
// runs on the device's pull goroutine: downmix, decimate, push.
func (c *Capture) OutputTap() func(frames [][2]float64) {
	return func(frames [][2]float64) {
		c.tapMu.Lock()
		if cap(c.refMono) < len(frames) {
			c.refMono = make([]float64, len(frames))
		}
		mono := c.refMono[:len(frames)]
		for i, f := range frames {
			mono[i] = (f[0] + f[1]) * 0.5
		}
		c.refOut = c.refDec.Process(mono, c.refOut[:0])
		c.refRing.push(c.refOut)
		c.tapMu.Unlock()
	}
}

// MicInfo describes the capture path (hardware.MicSource).
func (c *Capture) MicInfo() hardware.MicInfo {
	return hardware.MicInfo{
		Available:    true,
		Source:       c.opts.Device,
		SampleRateHz: Rate,
		FrameHz:      Rate / levelWindow,
		WindowMs:     1000 * levelWindow / Rate,
	}
}

// MicLevel reports the latest level frame (hardware.MicSource), measured on
// the raw capture.
func (c *Capture) MicLevel() (hardware.MicLevel, bool) {
	return c.meter.latestLevel()
}

// MicBands reports the latest band frame (hardware.MicBander): the peak each
// band reached over the same span as the matching MicLevel.
func (c *Capture) MicBands() ([hardware.MicBandCount]float64, bool) {
	return c.meter.latestBands()
}

// SetSessionModels installs a cartridge's bundled sound models for its
// session (engine.MicModels); nil at teardown removes them. An envelope
// that does not compile is logged and skipped.
func (c *Capture) SetSessionModels(models []*assetfile.Envelope) {
	if len(models) == 0 {
		c.cls.Store(nil)
		c.models.Store(0)
		return
	}
	cls, n, err := NewClassifier(models)
	if err != nil {
		c.opts.Log("mic: sound model: " + err.Error())
	}
	c.cls.Store(cls)
	c.models.Store(int32(n))
}

// Calibrate marks that the stinger is about to play: the pipeline watches
// for its alignment over the next few seconds and records the result in
// Stats.Calibration, logging a microphone that heard nothing.
func (c *Capture) Calibrate() { c.calib.arm() }

// Listen attaches fn to the pipeline, called once per analysis frame on the
// capture goroutine — keep it quick. The returned func detaches it.
func (c *Capture) Listen(fn func(Block)) (cancel func()) {
	c.mu.Lock()
	id := c.nextID
	c.nextID++
	c.listeners[id] = fn
	c.mu.Unlock()
	return func() {
		c.mu.Lock()
		delete(c.listeners, id)
		c.mu.Unlock()
	}
}

// Stats snapshots the pipeline.
func (c *Capture) Stats() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()
	a := c.aecStats
	return Stats{
		Running:     c.running,
		Device:      c.opts.Device,
		Frames:      c.frames,
		ERLE:        a.ERLE,
		Delay:       a.Delay,
		Aligned:     a.Aligned,
		Score:       a.Score,
		TryScore:    a.TryScore,
		TryShape:    a.TryShape,
		Ref:         c.refRing.len(),
		Cancel:      !c.opts.NoCancel,
		Events:      c.events.Load(),
		Models:      int(c.models.Load()),
		Restarts:    c.restarts,
		Calibration: c.calib.snapshot(),
	}
}

// restartDelay is the pause before reopening a capture that stopped.
const restartDelay = 2 * time.Second

// stream is one capture process's output, S16_LE mono at CaptureRate until
// EOF; Wait reaps the process.
type stream interface {
	io.Reader
	Wait() error
}

type opener func(ctx context.Context) (stream, error)

// Run captures until ctx is cancelled, reopening the device after a pause
// whenever the capture stops on its own (a device that went away and came
// back). It blocks; callers run it in a goroutine.
func (c *Capture) Run(ctx context.Context) error {
	heardBefore := false
	for {
		heard, err := c.runOnce(ctx, heardBefore)
		if ctx.Err() != nil {
			return nil
		}
		heardBefore = heardBefore || heard
		c.opts.Log(fmt.Sprintf("mic: capture stopped: %v; retrying in %v", err, c.retry))
		c.mu.Lock()
		c.restarts++
		c.running = false
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(c.retry):
		}
	}
}

// micSource stamps every microphone event; the player strategy passes keyboard
// sources unconditionally, and a mono microphone has no seat.
var micSource = input.PlayerSource{Type: input.SourceKeyboard, ID: "mic"}

func (c *Capture) openArecord(ctx context.Context) (stream, error) {
	args := []string{
		"-D", c.opts.Device,
		"-f", "S16_LE", "-c", "1",
		"-r", fmt.Sprint(CaptureRate),
		// the default buffer adds most of a second of delay jitter
		"--period-size=1024", "--buffer-size=4096",
		"-t", "raw", "-q", "-",
	}
	cmd := exec.CommandContext(ctx, "arecord", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("arecord: %w", err)
	}
	c.opts.Log(fmt.Sprintf("mic: listening on %s at %d Hz, analysing at %d Hz", c.opts.Device, CaptureRate, Rate))
	return arecordStream{stdout, cmd}, nil
}

type arecordStream struct {
	io.Reader
	cmd *exec.Cmd
}

func (s arecordStream) Wait() error { return s.cmd.Wait() }

// runOnce captures one stream to its end and reports whether it delivered
// any audio. regained says an earlier stream had, so this one's first audio
// is a device coming back and the stinger is replayed to realign.
func (c *Capture) runOnce(ctx context.Context, regained bool) (heard bool, err error) {
	s, err := c.open(ctx)
	if err != nil {
		return false, err
	}

	c.echo.aec.Reset()
	c.micDec.Reset()
	c.det.Reset()
	c.mu.Lock()
	c.aecStats = c.echo.aec.Stats()
	c.running = true
	c.mu.Unlock()

	frameLen := c.opts.Detector.FrameLen
	r := bufio.NewReaderSize(s, 8192)
	raw := make([]byte, 4096)
	pcm := make([]float64, 0, 2048)
	block := make([]float64, 0, 1024)
	pend := make([]float64, 0, frameLen)
	frame := make([]float64, frameLen)
	clean := make([]float64, frameLen)
	// The ring outlives a restart, so frame indices continue from its length.
	framed := c.micRing.len()

	for {
		n, rerr := r.Read(raw)
		if n > 0 {
			pcm = pcm[:0]
			for i := 0; i+1 < n; i += 2 {
				pcm = append(pcm, float64(int16(binary.LittleEndian.Uint16(raw[i:i+2])))/32768)
			}
			block = c.micDec.Process(pcm, block[:0])
			c.micRing.push(block)
			if !heard {
				heard = true
				// freezes the first read's jitter into a constant the delay estimate absorbs
				c.refRing.alignTo(c.micRing.len())
				if regained {
					c.replayStinger()
				}
			}
			c.meter.push(block)
			for _, s := range block {
				pend = append(pend, s)
				if len(pend) == frameLen {
					copy(frame, pend)
					pend = pend[:0]
					framed += frameLen
					c.analyse(frame, clean, framed)
				}
			}
		}
		if rerr != nil {
			if rerr == io.EOF || ctx.Err() != nil {
				break
			}
			_ = s.Wait()
			return heard, rerr
		}
	}
	return heard, s.Wait()
}

func (c *Capture) replayStinger() {
	if fn := c.recalibrate.Load(); fn != nil {
		(*fn)()
	}
}

// analyse runs one detector frame (ending at logical index end) through the
// echo judge, the gesture detector and the classifier, then tells the
// listeners.
func (c *Capture) analyse(frame, clean []float64, end int) {
	v := c.echo.judge(frame, clean, end)
	if err := c.det.SetTaint(v.taint); err != nil {
		c.opts.Log(err.Error())
	}

	aec := c.echo.aec.Stats()
	b := Block{
		Seq: c.frames, Raw: frame, Clean: clean, Echo: v.echo, Cancel: !c.opts.NoCancel, Cue: -1,
		ERLE: aec.ERLE, Delay: aec.Delay, Aligned: aec.Aligned,
	}

	c.det.Suppress(v.echo)
	c.det.WriteFloat(clean, func(f Frame) {
		b.RMS, b.Floor, b.PeakHz, b.Tonality, b.Event = f.RMS, f.Floor, f.PeakHz, f.Tonality, f.Event
		if key, ok := keyFor(f.Event); ok {
			c.opts.Log(fmt.Sprintf("mic: %s (%.0f Hz, %.2f tonality)", f.Event, f.PeakHz, f.Tonality))
			c.emit(key)
		}
	})
	if cls := c.cls.Load(); cls != nil {
		hit := cls.Write(clean, v.echo)
		b.Speech = cls.Active()
		if hit.Cue >= 0 {
			b.Cue, b.CueName, b.CueScore = hit.Cue, hit.Name, hit.Score
			c.opts.Log(fmt.Sprintf("mic: cue %d %q (%.3f)", hit.Cue, hit.Name, hit.Score))
			c.emit(input.MicCueKey(hit.Cue))
		}
	}
	c.calib.observe(b.Aligned, b.Delay, b.ERLE)

	c.mu.Lock()
	c.frames++
	c.aecStats = aec
	for _, fn := range c.listeners {
		fn(b)
	}
	c.mu.Unlock()
}

func (c *Capture) emit(key input.KeyName) {
	c.events.Add(1)
	sink := c.sink.Load()
	if sink == nil {
		return
	}
	select {
	case *sink <- input.Event{Kind: input.KindKeypress, Name: key, Source: micSource}:
	default:
	}
}

// keyFor maps a detected gesture to the KeyName it travels as.
func keyFor(e Event) (input.KeyName, bool) {
	switch e {
	case EventWhistle:
		return input.KeyMicWhistle, true
	case EventClap:
		return input.KeyMicClap, true
	case EventBlow:
		return input.KeyMicBlow, true
	}
	return "", false
}
