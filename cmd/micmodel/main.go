// SPDX-License-Identifier: GPL-2.0-or-later

// Command micmodel builds and inspects sound models from a directory of
// recordings — the lab bench for a cartridge's own vocabulary.
//
//	micmodel train -samples clips -out commands.arkademic.json -classes come,treat
//	micmodel eval  -model commands.arkademic.json -samples heldout
//	micmodel run   -model commands.arkademic.json take.wav
//	micmodel info  -model commands.arkademic.json
//
// The sample directory is one subdirectory per class, holding that class's
// clips, plus any underscore-prefixed directory (_noise) of sounds the model
// must not fire on. Everything is a pure function of the samples, so a score
// printed here is the score the cabinet computes.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/stealthmonkey99/arkade/assetfile"
	"github.com/stealthmonkey99/arkade/internal/mic"
)

const generatorVersion = "1.0.0"

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "train":
		err = train(os.Args[2:])
	case "eval":
		err = evaluate(os.Args[2:])
	case "run":
		err = run(os.Args[2:])
	case "info":
		err = info(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "micmodel:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `usage:
  micmodel train -samples DIR -out NAME.arkademic.json [-name NAME] [-classes a,b] [-templates 5]
  micmodel eval  -model NAME.arkademic.json -samples DIR
  micmodel run   -model NAME.arkademic.json clip.wav...
  micmodel info  -model NAME.arkademic.json
`)
	os.Exit(2)
}

func train(args []string) error {
	fs := flag.NewFlagSet("train", flag.ExitOnError)
	dir := fs.String("samples", "", "sample directory: one subdirectory per class, plus _noise")
	out := fs.String("out", "", "model to write, named <name>.arkademic.json")
	name := fs.String("name", "", "envelope name (default: the output file's stem)")
	classes := fs.String("classes", "", "comma-separated class names in cue order (default: every subdirectory, sorted)")
	templates := fs.Int("templates", 5, "maximum templates per class")
	fs.Parse(args)
	if *dir == "" || *out == "" {
		return fmt.Errorf("train needs -samples and -out")
	}

	o := mic.TrainOptions{
		Dir:       *dir,
		Name:      *name,
		Templates: *templates,
		Generator: assetfile.Generator{Tool: "micmodel", Version: generatorVersion},
	}
	if o.Name == "" {
		o.Name = strings.TrimSuffix(strings.TrimSuffix(*out, ".json"), ".arkademic")
		o.Name = stem(o.Name)
	}
	if *classes != "" {
		o.Classes = strings.Split(*classes, ",")
	}
	env, rep, err := mic.Train(o)
	if err != nil {
		if rep != nil {
			printReport(rep)
		}
		return err
	}
	if err := assetfile.Write(*out, env, nil); err != nil {
		return err
	}
	printReport(rep)
	fmt.Printf("\nwrote %s\n", *out)
	return nil
}

func evaluate(args []string) error {
	fs := flag.NewFlagSet("eval", flag.ExitOnError)
	model := fs.String("model", "", "the model to score with")
	dir := fs.String("samples", "", "held-out sample directory, laid out as the trainer's")
	fs.Parse(args)
	if *model == "" || *dir == "" {
		return fmt.Errorf("eval needs -model and -samples")
	}
	env, err := assetfile.Read(*model)
	if err != nil {
		return err
	}
	rep, err := mic.Evaluate(env, *dir)
	if err != nil {
		return err
	}
	printReport(rep)
	return nil
}

func run(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	model := fs.String("model", "", "the model to run")
	fs.Parse(args)
	if *model == "" || fs.NArg() == 0 {
		return fmt.Errorf("run needs -model and at least one WAV")
	}
	env, err := assetfile.Read(*model)
	if err != nil {
		return err
	}
	for _, path := range fs.Args() {
		samples, err := mic.ReadClipFile(path)
		if err != nil {
			return err
		}
		cls, _, err := mic.NewClassifier([]*assetfile.Envelope{env})
		if err != nil {
			return err
		}
		fmt.Printf("%s (%.2fs)\n", path, float64(len(samples))/mic.Rate)
		hits := 0
		for off := 0; off < len(samples); off += chunk {
			end := min(off+chunk, len(samples))
			if h := cls.Write(samples[off:end], false); h.Cue >= 0 {
				hits++
				fmt.Printf("  %7.2fs  cue %d %-12q score %.3f\n", float64(end)/mic.Rate, h.Cue, h.Name, h.Score)
			}
		}
		if hits == 0 {
			fmt.Println("  no hit")
		}
	}
	return nil
}

// chunk is the block size the cabinet hands the classifier, so a replay here
// segments exactly where a live capture would.
const chunk = 256

func info(args []string) error {
	fs := flag.NewFlagSet("info", flag.ExitOnError)
	model := fs.String("model", "", "the model to describe")
	fs.Parse(args)
	if *model == "" {
		return fmt.Errorf("info needs -model")
	}
	env, err := assetfile.Read(*model)
	if err != nil {
		return err
	}
	f, s := env.Features, env.Segment
	fmt.Printf("%s  (%s %s)\n", env.Name, env.Generator.Tool, env.Generator.Version)
	fmt.Printf("features  %d Hz, frame %d, hop %d (%.1f ms), %d bands, %d coeffs, %g..%g Hz\n",
		f.SampleRateHz, f.FrameLen, f.HopLen, float64(f.HopLen)*1000/float64(f.SampleRateHz),
		f.MelBands, f.Coeffs, f.MinHz, f.MaxHz)
	fmt.Printf("segment   %d..%d ms, hangover %d ms, snr %g, min rms %g\n\n",
		s.MinMs, s.MaxMs, s.HangoverMs, s.SNR, s.MinRMS)

	w := table()
	fmt.Fprintln(w, "class\tcue\ttemplates\tframes\tthreshold")
	for _, c := range env.Classes {
		frames := make([]string, len(c.Templates))
		for i, t := range c.Templates {
			frames[i] = fmt.Sprint(len(t))
		}
		fmt.Fprintf(w, "%s\t%d\t%d\t%s\t%.3f\n", c.Name, c.Cue, len(c.Templates), strings.Join(frames, ","), c.Threshold)
	}
	return w.Flush()
}

func printReport(rep *mic.Report) {
	w := table()
	fmt.Fprintln(w, "class\tcue\tclips\ttmpl\tthresh\taccepted\tfalse\tpos med\tpos max\tneg min")
	for _, c := range rep.Classes {
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%.3f\t%d/%d\t%d\t%.3f\t%.3f\t%s\n",
			c.Name, c.Cue, c.Clips, c.Templates, c.Threshold, c.Accepted, c.Clips,
			c.FalseAlarm, c.PosMedian, c.PosMax, dist(c.NegMin))
	}
	w.Flush()

	fmt.Printf("\n%d clips\n\nconfusion (row = actual, column = fired)\n", rep.Clips)
	w = table()
	fmt.Fprintf(w, "\t%s\n", strings.Join(rep.Labels, "\t"))
	for i, row := range rep.Confusion {
		label := "negatives"
		if i < len(rep.Labels)-1 {
			label = rep.Labels[i]
		}
		cells := make([]string, len(row))
		for j, n := range row {
			cells[j] = fmt.Sprint(n)
		}
		fmt.Fprintf(w, "%s\t%s\n", label, strings.Join(cells, "\t"))
	}
	w.Flush()

	for _, s := range rep.Skipped {
		fmt.Printf("skipped: %s\n", s)
	}
}

func table() *tabwriter.Writer {
	return tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
}

func dist(d float64) string {
	if d < 0 {
		return "-"
	}
	return fmt.Sprintf("%.3f", d)
}

func stem(path string) string {
	if i := strings.LastIndexAny(path, "/\\"); i >= 0 {
		return path[i+1:]
	}
	return path
}
