// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"fmt"
	"sort"
	"strings"
)

// Status is the outcome of a check or finding.
type Status string

const (
	StatusPass Status = "pass"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
	// StatusSkip means the check could not run here.
	StatusSkip Status = "skip"
	// StatusSuppressed marks a finding matched by a preflight.json
	// suppression. It ranks below pass and is always printed.
	StatusSuppressed Status = "suppressed"
)

// rank orders statuses by severity; skip and suppressed rank below pass.
func (s Status) rank() int {
	switch s {
	case StatusFail:
		return 3
	case StatusWarn:
		return 2
	case StatusPass:
		return 1
	default:
		return 0
	}
}

// worse returns whichever of the two statuses is more severe.
func worse(a, b Status) Status {
	if b.rank() > a.rank() {
		return b
	}
	return a
}

// Finding is one thing a check noticed. Rule cites the canon it enforces.
type Finding struct {
	Status  Status `json:"status"`
	Loc     Loc    `json:"-"`
	Where   string `json:"where,omitempty"` // Loc as printed
	Message string `json:"message"`
	Rule    string `json:"rule,omitempty"`
	// Severity and Reason are set only on a suppressed finding: the status it
	// would have had, and the suppression's justification.
	Severity Status `json:"severity,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// Result is one check's outcome for one cartridge.
type Result struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Rule is the check's headline citation; findings may cite a narrower one.
	Rule      string    `json:"rule"`
	Status    Status    `json:"status"`
	Summary   string    `json:"summary"`
	Findings  []Finding `json:"findings,omitempty"`
	ElapsedMS int64     `json:"elapsedMs,omitempty"`
}

// note records a finding and rolls its severity into the check's status.
func (r *Result) note(status Status, rule string, at Loc, format string, args ...any) {
	r.Findings = append(r.Findings, Finding{
		Status:  status,
		Loc:     at,
		Where:   at.String(),
		Message: fmt.Sprintf(format, args...),
		Rule:    rule,
	})
	r.Status = worse(r.Status, status)
}

// fail records a violation.
func (r *Result) fail(rule string, at Loc, format string, args ...any) {
	r.note(StatusFail, rule, at, format, args...)
}

// warn records something a human should look at.
func (r *Result) warn(rule string, at Loc, format string, args ...any) {
	r.note(StatusWarn, rule, at, format, args...)
}

// info records a measurement, shown under -v, that does not affect status.
func (r *Result) info(at Loc, format string, args ...any) {
	r.Findings = append(r.Findings, Finding{
		Status:  StatusPass,
		Loc:     at,
		Where:   at.String(),
		Message: fmt.Sprintf(format, args...),
	})
}

// skip marks the check inapplicable here.
func (r *Result) skip(format string, args ...any) {
	r.Status = StatusSkip
	r.Summary = fmt.Sprintf(format, args...)
}

// summarize sets the summary line unless the check already set one.
func (r *Result) summarize(format string, args ...any) {
	if r.Summary == "" {
		r.Summary = fmt.Sprintf(format, args...)
	}
}

// restatus recomputes the check's status from its findings after
// suppression, leaving a skip unchanged.
func (r *Result) restatus() {
	if r.Status == StatusSkip {
		return
	}
	s := StatusPass
	for _, f := range r.Findings {
		s = worse(s, f.Status)
	}
	r.Status = s
}

// counts tallies findings by status for the summary line.
func (r *Result) counts() (fails, warns int) {
	for _, f := range r.Findings {
		switch f.Status {
		case StatusFail:
			fails++
		case StatusWarn:
			warns++
		}
	}
	return fails, warns
}

// CartReport is one cartridge's whole report card.
type CartReport struct {
	Slug    string   `json:"slug"`
	Title   string   `json:"title,omitempty"`
	Version string   `json:"version,omitempty"`
	Status  Status   `json:"status"`
	Checks  []Result `json:"checks"`
}

// check returns the card's result for a check id, or nil if it did not run.
func (cr *CartReport) check(id string) *Result {
	for i := range cr.Checks {
		if cr.Checks[i].ID == id {
			return &cr.Checks[i]
		}
	}
	return nil
}

// verdict is the worst status across the card's checks.
func (cr *CartReport) verdict() Status {
	s := StatusPass
	for i := range cr.Checks {
		s = worse(s, cr.Checks[i].Status)
	}
	return s
}

// Report is the whole scan: one card per cartridge plus a roll-up.
type Report struct {
	Tool    string        `json:"tool"`
	Root    string        `json:"root"`
	Strict  bool          `json:"strict"`
	Carts   []CartReport  `json:"carts"`
	Summary ReportSummary `json:"summary"`
}

// ReportSummary counts cartridges by verdict.
type ReportSummary struct {
	Cartridges int      `json:"cartridges"`
	Passed     int      `json:"passed"`
	Warned     int      `json:"warned"`
	Failed     int      `json:"failed"`
	FailedList []string `json:"failedList,omitempty"`
	WarnedList []string `json:"warnedList,omitempty"`
}

// finish computes every card's verdict and the roll-up; under Strict a
// warning counts as a failure.
func (rep *Report) finish() {
	rep.Summary = ReportSummary{Cartridges: len(rep.Carts)}
	for i := range rep.Carts {
		v := rep.Carts[i].verdict()
		if rep.Strict && v == StatusWarn {
			v = StatusFail
		}
		rep.Carts[i].Status = v
		switch v {
		case StatusFail:
			rep.Summary.Failed++
			rep.Summary.FailedList = append(rep.Summary.FailedList, rep.Carts[i].Slug)
		case StatusWarn:
			rep.Summary.Warned++
			rep.Summary.WarnedList = append(rep.Summary.WarnedList, rep.Carts[i].Slug)
		default:
			rep.Summary.Passed++
		}
	}
	sort.Strings(rep.Summary.FailedList)
	sort.Strings(rep.Summary.WarnedList)
}

// badge is the fixed-width status column of the text report.
func badge(s Status) string {
	switch s {
	case StatusFail:
		return "FAIL"
	case StatusWarn:
		return "WARN"
	case StatusSkip:
		return "skip"
	case StatusSuppressed:
		return "supp"
	default:
		return "pass"
	}
}

// Text renders the human report card; verbose adds informational findings
// and rule citations.
func (rep *Report) Text(verbose bool) string {
	var b strings.Builder
	for i := range rep.Carts {
		cr := &rep.Carts[i]
		head := cr.Slug
		if cr.Version != "" {
			head += " " + cr.Version
		}
		if cr.Title != "" {
			head += " — " + cr.Title
		}
		fmt.Fprintf(&b, "%s\n", head)
		for j := range cr.Checks {
			res := &cr.Checks[j]
			fmt.Fprintf(&b, "  %-4s %-15s %s\n", badge(res.Status), res.ID, res.Summary)
			for _, f := range res.Findings {
				if f.Status == StatusPass && !verbose {
					continue
				}
				where := f.Where
				if where != "" {
					where += ": "
				}
				if f.Status == StatusSuppressed {
					fmt.Fprintf(&b, "       - %s[suppressed %s: %s] %s\n", where, f.Severity, f.Reason, f.Message)
					continue
				}
				fmt.Fprintf(&b, "       - %s%s\n", where, f.Message)
				if f.Rule != "" && verbose {
					fmt.Fprintf(&b, "         (%s)\n", f.Rule)
				}
			}
			if verbose && res.Rule != "" {
				fmt.Fprintf(&b, "       enforces: %s\n", res.Rule)
			}
		}
		fmt.Fprintf(&b, "  verdict: %s\n\n", strings.ToUpper(string(cr.Status)))
	}
	s := &rep.Summary
	fmt.Fprintf(&b, "%d cartridges: %d pass, %d warn, %d fail\n", s.Cartridges, s.Passed, s.Warned, s.Failed)
	if len(s.FailedList) > 0 {
		fmt.Fprintf(&b, "failing: %s\n", strings.Join(s.FailedList, " "))
	}
	if len(s.WarnedList) > 0 {
		fmt.Fprintf(&b, "warning: %s\n", strings.Join(s.WarnedList, " "))
	}
	return b.String()
}
