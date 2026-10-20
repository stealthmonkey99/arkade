// SPDX-License-Identifier: GPL-2.0-or-later

package wasm

import (
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/internal/controller"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/flight"
	"github.com/stealthmonkey99/arkade/internal/input"
)

// stubChannelHost exists because carttest imports this package.
type stubChannelHost struct {
	offered map[string]ChannelKind
	blobLen int
	values  map[string]int32
	logs    []string
	dropped bool
}

func newStubChannelHost() *stubChannelHost {
	return &stubChannelHost{offered: map[string]ChannelKind{}, values: map[string]int32{}}
}

func (h *stubChannelHost) OfferedChannel(name string) (ChannelKind, int, bool) {
	k, ok := h.offered[name]
	if !ok {
		return 0, 0, false
	}
	if k == ChannelBlob {
		return k, h.blobLen, true
	}
	return k, 0, true
}
func (h *stubChannelHost) SampleChannel(name string) int32 { return h.values[name] }
func (h *stubChannelHost) SampleChannelBlob(name string, dst []byte) int {
	for i := range dst {
		dst[i] = byte(h.values[name])
	}
	return len(dst)
}
func (h *stubChannelHost) SetSessionProfiles(profiles []*controller.Spec) {
	h.dropped = profiles == nil
}

func (h *stubChannelHost) Log(level LogLevel, msg string) { h.logs = append(h.logs, msg) }

func (h *stubChannelHost) Blit([]display.Pixel)                               {}
func (h *stubChannelHost) DrawPixel(int, int, display.Pixel)                  {}
func (h *stubChannelHost) FillRect(int, int, int, int, display.Pixel)         {}
func (h *stubChannelHost) LoadSprite([]byte) (uint32, error)                  { return 0, nil }
func (h *stubChannelHost) DrawSprite(uint32, int, int)                        {}
func (h *stubChannelHost) DrawCel([]byte, int, int, int, int)                 {}
func (h *stubChannelHost) BundleAsset(string) ([]byte, bool)                  { return nil, false }
func (h *stubChannelHost) LoadSound(string) (uint32, error)                   { return 0, nil }
func (h *stubChannelHost) PlaySound(uint32, uint32)                           {}
func (h *stubChannelHost) StopSound(uint32)                                   {}
func (h *stubChannelHost) StopAllSounds()                                     {}
func (h *stubChannelHost) StorageRead(string) ([]byte, bool)                  { return nil, false }
func (h *stubChannelHost) StorageWrite(string, []byte)                        {}
func (h *stubChannelHost) NowMicros() uint64                                  { return 0 }
func (h *stubChannelHost) RandomU64() uint64                                  { return 0 }
func (h *stubChannelHost) ScoreSubmit(int, string) int                        { return 0 }
func (h *stubChannelHost) ScoreRead(int) (int, string, bool)                  { return 0, "", false }
func (h *stubChannelHost) ScoreCount() int                                    { return 0 }
func (h *stubChannelHost) ScoreQualifies(int) bool                            { return false }
func (h *stubChannelHost) ScoreRank(int) int                                  { return 0 }
func (h *stubChannelHost) RecentName(int) (string, bool)                      { return "", false }
func (h *stubChannelHost) RecentNameAt(int, int) (string, bool)               { return "", false }
func (h *stubChannelHost) NoteRecentName(int, string)                         {}
func (h *stubChannelHost) PushPlayerStrategy(StrategyKind)                    {}
func (h *stubChannelHost) PopPlayerStrategy()                                 {}
func (h *stubChannelHost) LockPlayerStrategy()                                {}
func (h *stubChannelHost) UnlockPlayerStrategy()                              {}
func (h *stubChannelHost) SwitchPlayer(SwitchMode)                            {}
func (h *stubChannelHost) LockPlayerToSeat(int)                               {}
func (h *stubChannelHost) SetRawDirections(bool)                              {}
func (h *stubChannelHost) RejectKey()                                         {}
func (h *stubChannelHost) SetText(input.Seats, string, string)                {}
func (h *stubChannelHost) SetButtonLights(input.Seats, map[string]bool, bool) {}
func (h *stubChannelHost) SetLampLevels(input.Seats, [6]uint8, uint8, int)    {}
func (h *stubChannelHost) SetLights(input.Seats, bool)                        {}
func (h *stubChannelHost) Exit()                                              {}

func (h *stubChannelHost) logged(want string) bool {
	for _, l := range h.logs {
		if strings.Contains(l, want) {
			return true
		}
	}
	return false
}

func TestChannelBindCapIsSixtyFour(t *testing.T) {
	host := newStubChannelHost()
	for i := 0; i < 200; i++ {
		host.offered["p1.ch"+string(rune('a'+i%26))+string(rune('a'+i/26))] = ChannelAxis
	}
	reg := newChannelRegistry(host, nil, nil)
	bound := 0
	for name := range host.offered {
		if reg.bind(name, ChannelAxis) >= 0 {
			bound++
		}
	}
	if bound != maxBoundChannels {
		t.Fatalf("bound %d channels, want the cap to stop at %d", bound, maxBoundChannels)
	}
	if !host.logged("already holding 64 channels") {
		t.Errorf("the cap bit without a warning; logs: %v", host.logs)
	}
}

func TestChannelBindIsInitOnly(t *testing.T) {
	host := newStubChannelHost()
	host.offered["p1.spinner"] = ChannelDelta
	host.offered["p1.wheel"] = ChannelDelta
	reg := newChannelRegistry(host, nil, nil)

	first := reg.bind("p1.spinner", ChannelDelta)
	if first < 0 {
		t.Fatalf("the first bind failed")
	}
	reg.sample()

	if got := reg.bind("p1.wheel", ChannelDelta); got != -1 {
		t.Errorf("a bind after the first tick returned %d, want -1", got)
	}
	if !host.logged("channels bind during cart_init") {
		t.Errorf("the late bind was refused without saying why; logs: %v", host.logs)
	}
	// Rebinding an already bound name is a lookup, so it still works.
	if got := reg.bind("p1.spinner", ChannelDelta); got != first {
		t.Errorf("rebinding p1.spinner returned %d, want the original %d", got, first)
	}
}

func TestChannelBlobDeclaredLengthIsBounded(t *testing.T) {
	host := newStubChannelHost()
	host.offered["p1.mat"] = ChannelBlob
	host.blobLen = maxChannelBlobBytes + 1
	reg := newChannelRegistry(host, nil, nil)
	if got := reg.bind("p1.mat", ChannelBlob); got != -1 {
		t.Fatalf("a blob of %d bytes bound (handle %d); the cap is %d", host.blobLen, got, maxChannelBlobBytes)
	}
	if !host.logged("outside 1..256") {
		t.Errorf("no warning naming the blob cap; logs: %v", host.logs)
	}
}

// TestPanelChannelNamesAreTheDocumentedSet makes a contract change be typed twice.
func TestPanelChannelNamesAreTheDocumentedSet(t *testing.T) {
	names := PanelChannelNames()
	want := []string{
		"p1.joy.x", "p1.joy.y",
		"p1.joy.north", "p1.joy.south", "p1.joy.east", "p1.joy.west",
		"p1.a", "p1.b", "p1.c", "p1.d", "p1.aux1", "p1.aux2",
		"p1.l1", "p1.r1", "p1.l2", "p1.r2",
		"p1.select", "p1.start", "p1.turbo", "p1.clear", "p1.home", "p1.enter",
	}
	if len(names) != 2*len(want) {
		t.Fatalf("the panel offers %d channels, want %d (%d per seat, two seats)", len(names), 2*len(want), len(want))
	}
	for i, w := range want {
		if names[i] != w {
			t.Fatalf("panel channel %d is %q, want %q", i, names[i], w)
		}
		p2 := "p2." + strings.TrimPrefix(w, "p1.")
		if names[len(want)+i] != p2 {
			t.Fatalf("panel channel %d is %q, want %q", len(want)+i, names[len(want)+i], p2)
		}
	}
}

// TestRecordedChannelsReplay records against a live source and replays into
// a registry with no source, since the failure is a mismatch between ends.
func TestRecordedChannelsReplay(t *testing.T) {
	live := newStubChannelHost()
	live.offered["p1.spinner"] = ChannelDelta
	live.offered["p1.dial"] = ChannelAxis
	live.offered["p1.mat"] = ChannelBlob
	live.blobLen = 8

	tr := &tapeRecorder{}
	rec := newChannelRegistry(live, tr, nil)

	spinner := rec.bind("p1.spinner", ChannelDelta)
	dial := rec.bind("p1.dial", ChannelAxis)
	mat := rec.bind("p1.mat", ChannelBlob)
	if spinner < 0 || dial < 0 || mat < 0 {
		t.Fatalf("binds: spinner=%d dial=%d mat=%d", spinner, dial, mat)
	}

	// The dial holds through the middle frame, so nothing is recorded for it
	// there: the case a naive replay gets wrong.
	script := []struct{ spin, dial, mat int32 }{
		{spin: 3, dial: -400, mat: 1},
		{spin: 0, dial: -400, mat: 1},
		{spin: -2, dial: 900, mat: 7},
	}
	var want []struct{ spin, dial, matByte int32 }
	for i, step := range script {
		tr.frame = uint32(i + 1)
		live.values["p1.spinner"] = step.spin
		live.values["p1.dial"] = step.dial
		live.values["p1.mat"] = step.mat
		rec.sample()
		want = append(want, struct{ spin, dial, matByte int32 }{
			rec.bound[spinner].value, rec.bound[dial].value, int32(rec.bound[mat].blob[0]),
		})
	}

	dead := newStubChannelHost()
	var binds []ChannelBind
	byFrame := map[uint32][]tapeEvent{}
	for _, e := range tr.tape {
		switch e.kind {
		case flight.KindChannelBind:
			h, k, blobLen := flight.UnpackChannelBind(e.data)
			for int(h) >= len(binds) {
				binds = append(binds, ChannelBind{})
			}
			binds[h] = ChannelBind{Kind: ChannelKind(k), BlobLen: blobLen}
		default:
			byFrame[e.frame] = append(byFrame[e.frame], e)
		}
	}
	rp := newChannelRegistry(dead, nil, NewChannelReplay(binds))
	if got := rp.bind("p1.spinner", ChannelDelta); got != spinner {
		t.Fatalf("replayed spinner bound to %d, want %d — the bind table is what makes a replay possible at all", got, spinner)
	}
	if rp.bind("p1.dial", ChannelAxis) != dial || rp.bind("p1.mat", ChannelBlob) != mat {
		t.Fatalf("the replayed bind order diverged from the recorded one")
	}

	for i := range script {
		for _, e := range byFrame[uint32(i+1)] {
			switch e.kind {
			case flight.KindChannel:
				h, _, v := flight.UnpackChannel(e.data)
				rp.replay.SetValue(h, v)
			case flight.KindChannelBlob:
				h, chunk, payload := flight.UnpackChannelBlob(e.data)
				rp.replay.SetBlobChunk(h, chunk, payload[:])
			}
		}
		rp.sample()
		got := struct{ spin, dial, matByte int32 }{
			rp.bound[spinner].value, rp.bound[dial].value, int32(rp.bound[mat].blob[0]),
		}
		if got != want[i] {
			t.Fatalf("frame %d replayed %+v, want %+v", i+1, got, want[i])
		}
	}
}

type tapeEvent struct {
	frame uint32
	kind  uint8
	data  uint64
}

type tapeRecorder struct {
	frame uint32
	tape  []tapeEvent
}

func (r *tapeRecorder) RecordChannelBind(handle int32, kind ChannelKind, blobLen int) {
	r.tape = append(r.tape, tapeEvent{r.frame, flight.KindChannelBind, flight.PackChannelBind(handle, uint8(kind), blobLen)})
}

func (r *tapeRecorder) RecordChannel(handle int32, kind ChannelKind, value int32) {
	r.tape = append(r.tape, tapeEvent{r.frame, flight.KindChannel, flight.PackChannel(handle, uint8(kind), value)})
}

func (r *tapeRecorder) RecordChannelBlob(handle int32, blob []byte) {
	for chunk := 0; chunk*flight.ChannelBlobChunkBytes < len(blob); chunk++ {
		off := chunk * flight.ChannelBlobChunkBytes
		end := min(off+flight.ChannelBlobChunkBytes, len(blob))
		r.tape = append(r.tape, tapeEvent{r.frame, flight.KindChannelBlob, flight.PackChannelBlob(handle, chunk, blob[off:end])})
	}
}
