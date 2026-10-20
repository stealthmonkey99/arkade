// SPDX-License-Identifier: GPL-2.0-or-later

package mic

import (
	"slices"
	"testing"
)

func TestUnalignedGateFollowsThePlaybackTap(t *testing.T) {
	c := NewCapture(Options{Device: "none"})
	if c.echo.playingRecently() {
		t.Fatal("gate closed with nothing ever played")
	}
	tap := c.OutputTap()
	quiet := make([][2]float64, 512)
	for i := 0; i < 2*44100/512; i++ {
		tap(quiet)
	}
	if c.echo.playingRecently() {
		t.Fatal("gate closed on two seconds of silence")
	}
	loud := make([][2]float64, 512)
	for i := range loud {
		loud[i] = [2]float64{0.3, 0.3}
	}
	for i := 0; i < 44100/512/10; i++ {
		tap(loud)
	}
	if !c.echo.playingRecently() {
		t.Fatal("gate open right after a tenth of a second of playback")
	}
	for i := 0; i < 44100/512/2; i++ {
		tap(quiet)
	}
	if !c.echo.playingRecently() {
		t.Fatal("gate reopened only half a second after playback; the tail is longer")
	}
	for i := 0; i < 44100/512; i++ {
		tap(quiet)
	}
	if c.echo.playingRecently() {
		t.Fatal("gate still closed a second and a half after playback")
	}
}

// judgeRoom feeds a judge the cabinet's way (see simulate): broadband
// playback from 1 s to 4 s heard back through the room simDelay later. It
// returns each frame's verdict and whether clean matched the raw frame.
func judgeRoom(bypass bool) (verdicts []echoVerdict, untouched []bool) {
	ref := make([]float64, 5*Rate)
	burst := noise(0.3, 3000, 0, 7)
	for i, s := range burst {
		ref[Rate+i] = float64(s) / 32768
	}
	mic := make([]float64, len(ref))
	for i := simDelay; i < len(mic); i++ {
		mic[i] = 0.5 * ref[i-simDelay]
	}

	refRing, micRing := newRing(ringSeconds*Rate), newRing(ringSeconds*Rate)
	cfg := DefaultConfig()
	j := newEchoJudge(cfg, DefaultCancellerConfig(), refRing, micRing, bypass)
	n := cfg.FrameLen
	clean := make([]float64, n)
	refAt, framed := 0, 0
	for m := 0; m+simBlock <= len(mic); m += simBlock {
		for refAt < m+simBlock+simLead && refAt+simChunk <= len(ref) {
			refRing.push(ref[refAt : refAt+simChunk])
			refAt += simChunk
		}
		micRing.push(mic[m : m+simBlock])
		if m == 0 {
			refRing.alignTo(micRing.len())
		}
		for framed+n <= m+simBlock {
			frame := mic[framed : framed+n]
			framed += n
			v := j.judge(frame, clean, framed)
			v.taint = slices.Clone(v.taint)
			verdicts = append(verdicts, v)
			untouched = append(untouched, slices.Equal(clean, frame))
		}
	}
	return verdicts, untouched
}

func TestEchoJudgeBypassedPassesTheRawFrame(t *testing.T) {
	verdicts, untouched := judgeRoom(true)
	for i, v := range verdicts {
		if v.echo || v.taint != nil || !untouched[i] {
			t.Fatalf("frame %d bypassed: %+v, clean untouched %v", i, v, untouched[i])
		}
	}
}

func TestEchoJudgeMovesFromPlaybackGateToBinMask(t *testing.T) {
	verdicts, _ := judgeRoom(false)
	frameMs := func(i int) int { return i * DefaultConfig().FrameLen * 1000 / Rate }
	firstUnaligned, firstMasked := -1, -1
	for i, v := range verdicts {
		switch {
		case frameMs(i) < 800 && v.echo:
			t.Fatalf("frame %d at %d ms judged echo long before any playback", i, frameMs(i))
		case v.taint == nil && v.echo && firstUnaligned < 0:
			firstUnaligned = i
		case v.taint != nil && firstMasked < 0:
			firstMasked = i
			if want := newBankLayout(DefaultConfig()).toneBins(); len(v.taint) != want {
				t.Fatalf("taint mask has %d bins, the tone bank %d", len(v.taint), want)
			}
		}
	}
	if firstUnaligned < 0 || firstMasked < 0 || firstUnaligned > firstMasked {
		t.Fatalf("first unaligned echo at frame %d, first bin mask at %d; want playback gated before alignment, then masked",
			firstUnaligned, firstMasked)
	}
}
