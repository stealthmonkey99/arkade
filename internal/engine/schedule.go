// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"strconv"
	"strings"
	"time"
)

// Schedule is a screensaver's standing time window, kept under the "schedule"
// key of its settings file and re-read by the engine at most once a minute.
type Schedule struct {
	// Enabled is the master switch: a disabled schedule contains no instant.
	Enabled bool `json:"enabled"`
	// From and To are "HH:MM" 24-hour local time, inclusive of From and
	// exclusive of To. To earlier than From crosses midnight. Equal times, or
	// anything that does not parse, contain nothing.
	From string `json:"from"`
	To   string `json:"to"`
	// Days are the weekdays the window runs on, Sunday first (time.Weekday).
	// A window crossing midnight is checked against the day it started on.
	// All false means the schedule never runs.
	Days [7]bool `json:"days"`
	// Priority picks among open windows: highest wins, ties break by slug.
	Priority int `json:"priority"`
}

// EveryDay is a Days value covering the whole week.
func EveryDay() [7]bool { return [7]bool{true, true, true, true, true, true, true} }

// Contains reports whether now falls inside the window, in now's location.
func (s Schedule) Contains(now time.Time) bool {
	if !s.Enabled {
		return false
	}
	from, ok := parseHHMM(s.From)
	if !ok {
		return false
	}
	to, ok := parseHHMM(s.To)
	if !ok || from == to {
		return false
	}
	minute := now.Hour()*60 + now.Minute()
	day := int(now.Weekday())
	if from < to {
		return minute >= from && minute < to && s.Days[day]
	}
	// Crosses midnight: before To belongs to yesterday's opening of the window.
	if minute >= from {
		return s.Days[day]
	}
	return minute < to && s.Days[(day+6)%7]
}

func parseHHMM(s string) (int, bool) {
	hh, mm, found := strings.Cut(s, ":")
	if !found || len(hh) != 2 || len(mm) != 2 {
		return 0, false
	}
	h, err := strconv.Atoi(hh)
	if err != nil || h < 0 || h > 23 {
		return 0, false
	}
	m, err := strconv.Atoi(mm)
	if err != nil || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}
