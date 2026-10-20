// SPDX-License-Identifier: GPL-2.0-or-later

package mic

import (
	"context"
	"errors"
	"io"
	"slices"
	"sync"
	"testing"
	"time"
)

// Run under -race: Stats is polled from an HTTP goroutine during analysis.
func TestStatsIsSafeAlongsideAnalysis(t *testing.T) {
	c := NewCapture(Options{Device: "none"})
	sig := noise(0.3, 3000, 0, 0)
	ref := make([][2]float64, len(sig)*Decimation)
	for i := range ref {
		v := float64(sig[i/Decimation]) / 32768
		ref[i] = [2]float64{v, v}
	}
	c.OutputTap()(ref)
	mic := make([]float64, len(sig))
	for i, s := range sig {
		mic[i] = 0.5 * float64(s) / 32768
	}
	c.micRing.push(mic)
	c.Calibrate()

	frameLen := c.opts.Detector.FrameLen
	done := make(chan struct{})
	go func() {
		defer close(done)
		clean := make([]float64, frameLen)
		for end := frameLen; end <= len(mic); end += frameLen {
			c.analyse(mic[end-frameLen:end], clean, end)
		}
	}()
	for polling := true; polling; {
		select {
		case <-done:
			polling = false
		default:
			_ = c.Stats()
			_, _ = c.MicLevel()
		}
	}
	if s := c.Stats(); s.Frames == 0 || s.TryShape == 0 {
		t.Fatalf("analysis never published canceller stats: %+v", s)
	}
}

// fakeStream is one scripted capture process: it delivers its audio, then
// either ends or, when hold is set, stays open until the context ends.
type fakeStream struct {
	io.Reader
	hold context.Context
}

func (s *fakeStream) Read(p []byte) (int, error) {
	n, err := s.Reader.Read(p)
	if err == io.EOF && s.hold != nil {
		<-s.hold.Done()
	}
	return n, err
}

func (s *fakeStream) Wait() error { return nil }

// scriptedRun drives Run through a sequence of opens: 0 is a device that
// fails to open, a positive count is a stream of that many bytes of silence
// that then ends, and the last entry stays open. It returns the open count
// at each recalibration.
func scriptedRun(t *testing.T, script []int) []int {
	t.Helper()
	c := NewCapture(Options{Device: "none"})
	c.retry = time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	opens := 0
	var calls []int
	c.SetRecalibrate(func() {
		mu.Lock()
		calls = append(calls, opens)
		mu.Unlock()
	})
	c.open = func(context.Context) (stream, error) {
		mu.Lock()
		defer mu.Unlock()
		opens++
		if opens > len(script) {
			t.Fatal("Run reopened past the script")
		}
		bytes := script[opens-1]
		if bytes == 0 {
			if opens == len(script) {
				cancel()
			}
			return nil, errors.New("no such device")
		}
		s := &fakeStream{Reader: io.LimitReader(zeros{}, int64(bytes))}
		if opens == len(script) {
			s.hold = ctx
			go func() {
				for c.Stats().Frames == 0 {
					time.Sleep(time.Millisecond)
				}
				cancel()
			}()
		}
		return s, nil
	}
	if err := c.Run(ctx); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	return calls
}

type zeros struct{}

func (zeros) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

const oneSecondOfAudio = 2 * CaptureRate

func TestRecalibratesOnlyWhenAHeardDeviceComesBack(t *testing.T) {
	cases := []struct {
		name   string
		script []int
		want   []int
	}{
		{"first start", []int{oneSecondOfAudio}, nil},
		{"first start after a failed open", []int{0, oneSecondOfAudio}, nil},
		{"first start after repeated failed opens", []int{0, 0, oneSecondOfAudio}, nil},
		{"device lost and back", []int{oneSecondOfAudio, oneSecondOfAudio}, []int{2}},
		{"device lost, absent, back", []int{oneSecondOfAudio, 0, 0, oneSecondOfAudio}, []int{4}},
		{"device lost and never back", []int{oneSecondOfAudio, 0, 0, 0}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := scriptedRun(t, tc.script)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("recalibrated at opens %v, want %v", got, tc.want)
			}
		})
	}
}
