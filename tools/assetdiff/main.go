// SPDX-License-Identifier: GPL-2.0-or-later

// Command assetdiff compares two asset files, or two directories of them, by
// payload rather than bytes: a sound by its "data" chunk, a sprite by its
// decoded pixels. Directory mode pairs files by stem, so "lost.wav" meets
// "lost.arkadesound.wav". Several pairs may be given at once. Exit status
// is 1 if anything differs and 2 if a file could not be read.
//
//	assetdiff old.wav new.arkadesound.wav
//	assetdiff before/ after/
//	assetdiff -tolerance committed.arkadesound.wav rerendered.wav
//
// The comparison is exact by default. -tolerance is for audio re-rendered
// on another machine, which need not be byte-identical: speech, whose
// Kokoro render moves in the low bits across CPU architectures. A sound
// pair that is not identical is then measured, and passes as NEAR when
// its largest per-sample difference is at most -max-delta (full scale is
// 1) and its signal-to-noise ratio, the old side's energy over the
// difference's, is at least -min-snr dB. The two are lined up first, at
// whichever shift of up to 50 ms fits best: speech trims its render to a
// silence threshold in 5 ms frames, and another CPU's arithmetic can move
// an onset across a frame boundary, which shifts the whole clip by a frame
// (one such clip measured SNR -2.2 dB unaligned and 25.3 dB at 5 ms).
// Past the alignment, a length change counts the missing samples as
// error. Other kinds stay exact.
//
// The defaults, -max-delta 0.25 and -min-snr 15, were measured with
// tools/speech (bm_george at speed 1.3, 16 kHz, highpass 150, peak 0.9)
// on an arm64 Mac, re-rendering as tools/speech/rerender does and comparing
// its output pairs:
//
//	go run ./tools/speech/rerender -out <dir> <roots...> > pairs
//	go run ./tools/assetdiff -tolerance $(cat pairs)
//
// with these results:
//
//	the same line rendered twice                     identical
//	ONNX Runtime on 1 thread instead of all          max delta 0.00003, SNR 96-97 dB
//	ONNX Runtime graph optimization off or basic     max delta 0.06-0.09, SNR 25-30 dB
//	"fifteen two" against "fifteen four" or "six"    max delta 1.3-1.4, SNR -3 to -4 dB
//	"fifteen two" against "fifteen two."             max delta 1.32, SNR -3.7 dB
//	"fifteen two" at speed 1.3 against 1.32          max delta 0.95, SNR 0.6 dB
//
// A changed kernel stands in for another CPU's arithmetic, so the floor
// is the graph-optimization row: the thresholds pass it with about 10 dB
// and 2.7x to spare and fail every changed line by 14 dB and 3.8x. They do
// not catch a small level change: peak 0.9 against 0.85 is 25 dB.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/stealthmonkey99/arkade/assetfile"
)

func main() {
	quiet := flag.Bool("q", false, "print only differences")
	htmlOut := flag.String("html", "", "also write a self-contained comparison page here")
	leftName := flag.String("old-name", "before", "what to call the left side in the -html report")
	rightName := flag.String("new-name", "after", "what to call the right side in the -html report")
	tolerant := flag.Bool("tolerance", false, "let sounds differ within -max-delta and -min-snr")
	maxDelta := flag.Float64("max-delta", defaultMaxDelta, "with -tolerance, the largest per-sample difference, full scale 1")
	minSNR := flag.Float64("min-snr", defaultMinSNR, "with -tolerance, the lowest signal-to-noise ratio in dB")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `usage: assetdiff [-q] [-tolerance] <old> <new> [<old> <new> ...]
       assetdiff -html <page> <old> <new> [<old> <new> ...]

Both arguments of a pair are files, or both are directories. With -html,
each pair is one section — normally one cartridge — and the page inlines
every asset so it can be opened straight from disk.

`)
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() < 2 || flag.NArg()%2 != 0 {
		flag.Usage()
		os.Exit(2)
	}
	if *htmlOut != "" {
		if err := runReport(*htmlOut, flag.Args(), *leftName, *rightName); err != nil {
			fatal(err)
		}
	}
	var tol *tolerance
	if *tolerant {
		tol = &tolerance{maxDelta: *maxDelta, minSNR: *minSNR}
	}
	differed := false
	for i := 0; i < flag.NArg(); i += 2 {
		if diffPair(flag.Arg(i), flag.Arg(i+1), *quiet, tol) {
			differed = true
		}
	}
	if differed {
		os.Exit(1)
	}
}

func diffPair(left, right string, quiet bool, tol *tolerance) bool {
	lInfo, err := os.Stat(left)
	if err != nil {
		fatal(err)
	}
	rInfo, err := os.Stat(right)
	if err != nil {
		fatal(err)
	}
	if lInfo.IsDir() != rInfo.IsDir() {
		fatal(errors.New("compare two files or two directories, not one of each"))
	}
	if lInfo.IsDir() {
		return diffDirs(left, right, quiet, tol)
	}
	name := filepath.Base(right)
	if flag.NArg() > 2 {
		name = left // among many pairs a base name may repeat
	}
	return !report(name, left, right, quiet, tol)
}

func diffDirs(left, right string, quiet bool, tol *tolerance) bool {
	l, err := assetStems(left)
	if err != nil {
		fatal(err)
	}
	r, err := assetStems(right)
	if err != nil {
		fatal(err)
	}
	stems := make([]string, 0, len(l))
	for s := range l {
		stems = append(stems, s)
	}
	for s := range r {
		if _, ok := l[s]; !ok {
			stems = append(stems, s)
		}
	}
	sort.Strings(stems)

	differed := false
	for _, s := range stems {
		ln, lok := l[s]
		rn, rok := r[s]
		switch {
		case !rok:
			fmt.Printf("GONE  %s\n", ln)
			differed = true
		case !lok:
			fmt.Printf("NEW   %s\n", rn)
			differed = true
		default:
			if !report(rn, filepath.Join(left, ln), filepath.Join(right, rn), quiet, tol) {
				differed = true
			}
		}
	}
	return differed
}

// report prints one pair's verdict and returns whether the payloads matched,
// exactly or, given a tolerance, within it.
func report(name, leftPath, rightPath string, quiet bool, tol *tolerance) bool {
	a, err := os.ReadFile(leftPath)
	if err != nil {
		fatal(err)
	}
	b, err := os.ReadFile(rightPath)
	if err != nil {
		fatal(err)
	}
	err = assetfile.SamePayload(name, a, b)
	if err != nil && tol != nil && isSound(name, a) {
		return reportNear(name, a, b, quiet, *tol)
	}
	if err != nil {
		fmt.Printf("DIFF  %s: %v\n", name, err)
		return false
	}
	if !quiet {
		fmt.Printf("SAME  %s\n", name)
	}
	return true
}

func reportNear(name string, a, b []byte, quiet bool, tol tolerance) bool {
	d, err := measureAudio(a, b)
	switch {
	case err != nil:
		fmt.Printf("DIFF  %s: %v\n", name, err)
		return false
	case !d.within(tol):
		fmt.Printf("DIFF  %s: %v; allowed %v\n", name, d, tol)
		return false
	}
	if !quiet {
		fmt.Printf("NEAR  %s: %v\n", name, d)
	}
	return true
}

func isSound(name string, data []byte) bool {
	if kind, ok := assetfile.KindFromFilename(name); ok {
		return kind == assetfile.KindSound
	}
	return bytes.HasPrefix(data, []byte("RIFF"))
}

// assetStems maps each asset file in dir to its name, keyed by assetStem.
func assetStems(dir string) (map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		switch strings.ToLower(filepath.Ext(name)) {
		case ".wav", ".png":
		default:
			continue
		}
		out[assetStem(name)] = name
	}
	return out, nil
}

// assetStem strips the extension and any ".arkade<kind>" component.
func assetStem(name string) string {
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	if dot := strings.LastIndex(stem, "."); dot >= 0 && strings.HasPrefix(stem[dot+1:], "arkade") {
		return stem[:dot]
	}
	return stem
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "assetdiff: %v\n", err)
	os.Exit(2)
}
