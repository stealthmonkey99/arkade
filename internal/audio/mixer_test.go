// SPDX-License-Identifier: GPL-2.0-or-later

package audio

import (
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/wav"
)

// tone streams a constant-amplitude signal for n frames.
type tone struct {
	amp float64
	n   int
}

func (t *tone) Stream(samples [][2]float64) (int, bool) {
	if t.n <= 0 {
		return 0, false
	}
	n := min(len(samples), t.n)
	for i := 0; i < n; i++ {
		samples[i][0] = t.amp
		samples[i][1] = t.amp
	}
	t.n -= n
	return n, true
}

func (t *tone) Err() error { return nil }

// encodeWav renders writeWav's fixture to bytes. wav.Encode needs an
// io.WriteSeeker, which *bytes.Buffer isn't, so it round-trips through a temp file.
func encodeWav(t *testing.T, rate beep.SampleRate, frames int, amp float64) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.wav")
	writeWav(t, path, rate, frames, amp)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// writeWav writes a stereo constant-tone fixture.
func writeWav(t *testing.T, path string, rate beep.SampleRate, frames int, amp float64) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	format := beep.Format{SampleRate: rate, NumChannels: 2, Precision: 2}
	if err := wav.Encode(f, &tone{amp: amp, n: frames}, format); err != nil {
		t.Fatal(err)
	}
}

// newTestMixer builds a mixer with one 1000-frame 44.1k tone at
// ./assets/beep.wav under owner games/test.
func newTestMixer(t *testing.T) (*Mixer, string, *[]string) {
	t.Helper()
	root := t.TempDir()
	writeWav(t, filepath.Join(root, "games/test/assets/beep.wav"), MixRate, 1000, 0.5)
	var logs []string
	m := newCore(DirResolver(root), func(level, msg string) { logs = append(logs, level+": "+msg) })
	return m, root, &logs
}

func rms(frames [][2]float64) float64 {
	var sum float64
	for _, f := range frames {
		sum += f[0]*f[0] + f[1]*f[1]
	}
	return math.Sqrt(sum / float64(len(frames)*2))
}

func TestMixerPlayProducesAudio(t *testing.T) {
	m, _, _ := newTestMixer(t)
	if got := rms(m.readPCM(64)); got != 0 {
		t.Fatalf("silence expected before Play, rms=%f", got)
	}
	m.Play("games/test", "", "./assets/beep.wav", 0)
	if got := rms(m.readPCM(64)); math.Abs(got-0.5) > 0.02 {
		t.Errorf("expected ~0.5 amplitude tone, rms=%f", got)
	}
	m.readPCM(2000)
	if got := rms(m.readPCM(64)); got != 0 {
		t.Errorf("expected silence after sound ends, rms=%f", got)
	}
	if m.Playing("games/test", "") {
		t.Error("finished sound should not report playing")
	}
}

func TestMixerStopSilencesOwner(t *testing.T) {
	m, _, _ := newTestMixer(t)
	m.Play("games/test", "sfx", "./assets/beep.wav", 0)
	if rms(m.readPCM(32)) == 0 {
		t.Fatal("expected audio while playing")
	}
	m.Stop("games/test", "")
	if got := rms(m.readPCM(32)); got != 0 {
		t.Errorf("expected silence after Stop, rms=%f", got)
	}
}

func TestMixerLoopForeverUntilStopped(t *testing.T) {
	m, _, _ := newTestMixer(t)
	m.Play("games/test", "music", "./assets/beep.wav", LoopForever)
	m.readPCM(5000)
	if got := rms(m.readPCM(64)); math.Abs(got-0.5) > 0.02 {
		t.Errorf("looping sound should continue, rms=%f", got)
	}
	if !m.Playing("games/test", "music") {
		t.Error("looping sound should report playing")
	}
	m.Stop("games/test", "music")
	if got := rms(m.readPCM(64)); got != 0 {
		t.Errorf("expected silence after stopping loop, rms=%f", got)
	}
}

func TestMixerVolumeAndMute(t *testing.T) {
	m, _, _ := newTestMixer(t)
	m.Play("games/test", "music", "./assets/beep.wav", LoopForever)
	full := rms(m.readPCM(256))

	m.SetVolumePercent(50)
	half := rms(m.readPCM(256))
	if math.Abs(half-full/2) > 0.02 {
		t.Errorf("50%% volume should halve gain: full=%f half=%f", full, half)
	}

	m.SetMuted(true)
	if got := rms(m.readPCM(64)); got != 0 {
		t.Errorf("muted output should be silent, rms=%f", got)
	}
	m.SetMuted(false)
	if got := rms(m.readPCM(64)); got == 0 {
		t.Error("unmute should restore audio")
	}
	m.SetVolumePercent(0)
	if got := rms(m.readPCM(64)); got != 0 {
		t.Errorf("0%% volume should be silent, rms=%f", got)
	}
}

func TestMixerMissingFileLogsOnceAndContinues(t *testing.T) {
	m, _, logs := newTestMixer(t)
	m.Play("games/test", "", "./assets/nope.wav", 0)
	m.Play("games/test", "", "./assets/nope.wav", 0)
	if len(*logs) != 1 {
		t.Errorf("missing file should log exactly once, got %d logs", len(*logs))
	}
	if m.Playing("games/test", "") {
		t.Error("missing sound must not report playing")
	}
	if got := rms(m.readPCM(32)); got != 0 {
		t.Errorf("missing sound must stay silent, rms=%f", got)
	}
}

func TestMixerResamplesOtherRates(t *testing.T) {
	m, root, _ := newTestMixer(t)
	writeWav(t, filepath.Join(root, "games/test/assets/low.wav"), 22050, 500, 0.5)
	m.Play("games/test", "", "./assets/low.wav", 0)
	if got := rms(m.readPCM(64)); math.Abs(got-0.5) > 0.05 {
		t.Errorf("resampled tone should keep amplitude, rms=%f", got)
	}
	m.readPCM(3000)
	if got := rms(m.readPCM(64)); got != 0 {
		t.Errorf("expected silence after resampled sound ends, rms=%f", got)
	}
}

func TestPCMReaderEncodesFloat32LE(t *testing.T) {
	var mu sync.Mutex
	r := &pcmReader{mu: &mu, src: &tone{amp: 1.0, n: 2}}
	buf := make([]byte, 2*bytesPerFrame)
	n, err := r.Read(buf)
	if err != nil || n != 2*bytesPerFrame {
		t.Fatalf("read: n=%d err=%v", n, err)
	}
	// 1.0 as float32 LE = 00 00 80 3f
	want := []byte{0x00, 0x00, 0x80, 0x3f}
	for frame := 0; frame < 2; frame++ {
		for ch := 0; ch < 2; ch++ {
			off := frame*bytesPerFrame + ch*4
			for i, b := range want {
				if buf[off+i] != b {
					t.Fatalf("frame %d ch %d byte %d: got %02x want %02x", frame, ch, i, buf[off+i], b)
				}
			}
		}
	}
}

func TestDirResolverLayout(t *testing.T) {
	root := t.TempDir()
	writeWav(t, filepath.Join(root, "games/snake/assets/lost.wav"), MixRate, 10, 0.1)
	res := DirResolver(root)
	rc, err := res("games/snake", "./assets/lost.wav")
	if err != nil {
		t.Fatalf("resolver should find the path: %v", err)
	}
	rc.Close()
	if _, err := res("games/snake", "./assets/ghost.wav"); err == nil {
		t.Error("resolver should error on missing files")
	}
}

func TestMixerPreloadOwnerCachesSounds(t *testing.T) {
	root := t.TempDir()
	writeWav(t, filepath.Join(root, "games/test/assets/beep.wav"), MixRate, 1000, 0.5)
	m := NewMixer(DirResolver(root), DirLister(root), nil)

	m.PreloadOwner("games/test")
	if _, ok := m.cache["games/test\x00./assets/beep.wav"]; !ok {
		t.Fatal("PreloadOwner should decode and cache the sound before first play")
	}
	m.Play("games/test", "", "./assets/beep.wav", 0)
	if rms(m.readPCM(64)) == 0 {
		t.Error("preloaded sound should produce audio")
	}
}

func TestMixerLoadBytesRegistersAndPlays(t *testing.T) {
	m := newCore(DirResolver(t.TempDir()), nil)
	data := encodeWav(t, MixRate, 1000, 0.5)

	if err := m.LoadBytes("apps/config", "boop.wav", data); err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	if _, ok := m.cache["apps/config\x00boop.wav"]; !ok {
		t.Fatal("LoadBytes should populate the mixer cache")
	}

	m.Play("apps/config", "", "boop.wav", 0)
	if got := rms(m.readPCM(64)); math.Abs(got-0.5) > 0.02 {
		t.Errorf("expected ~0.5 amplitude tone from the loaded bytes, rms=%f", got)
	}

	var _ Service = m
}

func TestMixerLoadBytesRejectsBadWAV(t *testing.T) {
	m := newCore(DirResolver(t.TempDir()), nil)
	if err := m.LoadBytes("apps/config", "boop.wav", []byte("not a wav")); err == nil {
		t.Fatal("expected an error decoding non-WAV bytes")
	}
	if _, ok := m.cache["apps/config\x00boop.wav"]; ok {
		t.Error("a failed LoadBytes must not populate the cache")
	}
}

func TestMixerPlayBytesAttenuatesAndNeverCaches(t *testing.T) {
	m, _, _ := newTestMixer(t)
	wav := encodeWav(t, MixRate, 1000, 0.8)

	if err := m.PlayBytes("preview/owner", "preview", wav, 0.5); err != nil {
		t.Fatalf("PlayBytes: %v", err)
	}
	if got := rms(m.readPCM(64)); math.Abs(got-0.4) > 0.02 {
		t.Errorf("rms = %f, want ~0.4 (0.8 source at 0.5 gain)", got)
	}
	if len(m.cache) != 0 {
		t.Errorf("PlayBytes cached %d sounds, want 0", len(m.cache))
	}
	if !m.Playing("preview/owner", "preview") {
		t.Error("preview should report playing")
	}
	m.Stop("preview/owner", "preview")
	if got := rms(m.readPCM(64)); got != 0 {
		t.Errorf("rms = %f after Stop, want silence", got)
	}
}

func TestMixerPlayBytesGainBoundsAndBadWav(t *testing.T) {
	m, _, _ := newTestMixer(t)
	wav := encodeWav(t, MixRate, 1000, 0.8)

	if err := m.PlayBytes("preview/owner", "silent", wav, 0); err != nil {
		t.Fatalf("PlayBytes: %v", err)
	}
	if got := rms(m.readPCM(64)); got != 0 {
		t.Errorf("rms = %f at gain 0, want silence", got)
	}
	m.Stop("preview/owner", "")

	if err := m.PlayBytes("preview/owner", "loud", wav, 11); err != nil {
		t.Fatalf("PlayBytes: %v", err)
	}
	if got := rms(m.readPCM(64)); math.Abs(got-0.8) > 0.02 {
		t.Errorf("rms = %f at gain 11, want the source's ~0.8 (clamped to 1)", got)
	}
	m.Stop("preview/owner", "")

	if err := m.PlayBytes("preview/owner", "junk", []byte("not a wav at all"), 1); err == nil {
		t.Error("PlayBytes should reject bytes that aren't a WAV")
	}
	if m.Playing("preview/owner", "junk") {
		t.Error("a rejected WAV must not register a sound")
	}
}

func TestRecorderPlayBytesRecordsTheCall(t *testing.T) {
	r := NewRecorder()
	r.PlayBytes("preview/owner", "preview", []byte{1, 2, 3, 4}, 0.25)
	events := r.Events()
	if len(events) != 1 {
		t.Fatalf("events = %+v, want one", events)
	}
	ev := events[0]
	if ev.Action != "play" || ev.Owner != "preview/owner" || ev.Bytes != 4 || ev.Gain != 0.25 {
		t.Errorf("event = %+v, want a play of 4 bytes at 0.25 gain", ev)
	}
}

func TestEmbeddedSoundRegistersAndPlaysOnEitherBackend(t *testing.T) {
	m, _, _ := newTestMixer(t)
	for name, svc := range map[string]Service{"mixer": m, "recorder": NewRecorder()} {
		svc.PreloadOwner("apps/config")
		if err := svc.LoadBytes("apps/config", "boop.wav", PreviewBoop()); err != nil {
			t.Fatalf("%s: LoadBytes: %v", name, err)
		}
		svc.Play("apps/config", "boop", "boop.wav", 0)
	}
	if _, ok := m.cache["apps/config\x00boop.wav"]; !ok {
		t.Error("the mixer did not cache the registered sound")
	}
}
