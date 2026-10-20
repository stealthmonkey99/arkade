// SPDX-License-Identifier: GPL-2.0-or-later

package speech

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
)

// VoiceInfo describes one voice the renderer can speak with.
type VoiceInfo struct {
	ID       string `json:"id"` // what Voice.Name takes: "bm_george"
	Name     string `json:"name"`
	Language string `json:"language"` // "en-gb", "en-us"
	Gender   string `json:"gender"`
	Grade    string `json:"grade"` // the model card's quality grade, "A" best
}

// Renderer keeps one Kokoro worker alive so successive lines cost a
// fraction of a second instead of a model load each. The worker starts
// on first use; a Renderer is safe for concurrent use and serializes
// requests. Recent renders are cached by Voice and text, so
// re-shaping a line already spoken does not speak it again.
type Renderer struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	tmp    string
	seq    int
	cache  map[string][]float32
	order  []string
	voices []VoiceInfo
}

const cacheEntries = 64

// NewRenderer returns a Renderer whose worker has not started yet.
func NewRenderer() *Renderer {
	return &Renderer{cache: map[string][]float32{}}
}

// Dir is this package's source directory, where render.mjs and its
// node_modules live.
func Dir() string {
	_, self, _, _ := runtime.Caller(0)
	return filepath.Dir(self)
}

// Installed reports why the renderer cannot run, or nil if it can. It
// checks for Node and the npm install, not for the model, which the
// worker fetches on first start.
func Installed() error {
	if _, err := exec.LookPath("node"); err != nil {
		return errors.New("speech: node is not on PATH")
	}
	if _, err := os.Stat(filepath.Join(Dir(), "node_modules", "kokoro-js")); err != nil {
		return fmt.Errorf("speech: renderer not installed; run `npm ci` in %s", Dir())
	}
	return nil
}

// Start launches the worker and waits until the model is loaded, which
// on the very first run includes the ~330 MB model download. Calling it
// is optional (Speak starts the worker itself); it exists so a server
// can warm up in the background.
func (r *Renderer) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.start()
}

func (r *Renderer) start() error {
	if r.cmd != nil {
		return nil
	}
	if err := Installed(); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "arkade-speech")
	if err != nil {
		return err
	}
	cmd := exec.Command("node", filepath.Join(Dir(), "render.mjs"), "serve")
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	r.cmd, r.stdin, r.stdout, r.tmp = cmd, stdin, bufio.NewReader(stdout), tmp
	var hello struct {
		Ready bool   `json:"ready"`
		Error string `json:"error"`
	}
	if err := r.read(&hello); err != nil || !hello.Ready {
		if err == nil {
			err = errors.New(hello.Error)
		}
		r.stop()
		return fmt.Errorf("speech: worker failed to start: %w", err)
	}
	return nil
}

func (r *Renderer) read(v any) error {
	line, err := r.stdout.ReadBytes('\n')
	if err != nil {
		return err
	}
	return json.Unmarshal(line, v)
}

// call sends one request and reads its answer, restarting the worker
// once if it has died.
func (r *Renderer) call(req, resp any) error {
	b, err := json.Marshal(req)
	if err != nil {
		return err
	}
	for attempt := 0; ; attempt++ {
		if err := r.start(); err != nil {
			return err
		}
		_, werr := r.stdin.Write(append(b, '\n'))
		if werr == nil {
			werr = r.read(resp)
		}
		if werr == nil {
			return nil
		}
		r.stop()
		if attempt == 1 {
			return fmt.Errorf("speech: worker: %w", werr)
		}
	}
}

func (r *Renderer) stop() {
	if r.stdin != nil {
		r.stdin.Close()
	}
	if r.cmd != nil {
		r.cmd.Process.Kill()
		r.cmd.Wait()
	}
	if r.tmp != "" {
		os.RemoveAll(r.tmp)
	}
	r.cmd, r.stdin, r.stdout, r.tmp = nil, nil, nil, ""
}

// Close stops the worker. The Renderer stays usable; the next call
// starts a new one.
func (r *Renderer) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stop()
}

// Voices lists the voices the model ships, in the model card's order.
func (r *Renderer) Voices() ([]VoiceInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.voices != nil {
		return r.voices, nil
	}
	var resp struct {
		Voices []VoiceInfo `json:"voices"`
		Error  string      `json:"error"`
	}
	if err := r.call(map[string]string{"op": "voices"}, &resp); err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("speech: %s", resp.Error)
	}
	r.voices = resp.Voices
	return r.voices, nil
}

// Speak renders text in v and returns mono samples at sampleRate.
func (r *Renderer) Speak(v Voice, text string, sampleRate int) ([]float64, error) {
	raw, err := r.speak(v, text)
	if err != nil {
		return nil, err
	}
	s := make([]float64, len(raw))
	for i, x := range raw {
		s[i] = float64(x)
	}
	return Resample(s, ModelRate, sampleRate), nil
}

func (r *Renderer) speak(v Voice, text string) ([]float32, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	req := newJob(v)
	key := v.Name + "\x00" + strconv.FormatFloat(v.Speed, 'g', -1, 64) + "\x00" + text
	if req.Blend != nil {
		key += "\x00" + req.Blend.Voice + "\x00" + strconv.FormatFloat(req.Blend.Weight, 'g', -1, 64)
	}
	if s, ok := r.cache[key]; ok {
		return s, nil
	}
	if err := r.start(); err != nil {
		return nil, err
	}
	r.seq++
	out := filepath.Join(r.tmp, fmt.Sprintf("%d.f32", r.seq))
	req.Items = []jobItem{{Text: text, Out: out}}
	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := r.call(req, &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("speech: %s", resp.Error)
	}
	b, err := os.ReadFile(out)
	os.Remove(out)
	if err != nil {
		return nil, err
	}
	s := make([]float32, len(b)/4)
	for i := range s {
		s[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:]))
	}
	if len(r.order) >= cacheEntries {
		delete(r.cache, r.order[0])
		r.order = r.order[1:]
	}
	r.cache[key] = s
	r.order = append(r.order, key)
	return s, nil
}
