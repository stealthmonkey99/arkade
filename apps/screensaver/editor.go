// SPDX-License-Identifier: GPL-2.0-or-later

// Package screensaver holds the pieces the scheduled screensavers share.
package screensaver

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
)

// The schedule rows every editor starts with; app fields follow from RowFields.
const (
	RowEnabled = iota
	RowFrom
	RowTo
	RowDays
	RowFields
)

// DayLetters labels the week Sunday first, like engine.Schedule.Days.
const DayLetters = "SMTWTFS"

// TimeStepMinutes is how far FROM and TO move per press.
const TimeStepMinutes = 15

// Field is one app-specific row of a settings screen, after the schedule rows.
type Field struct {
	Label  string
	Value  func() string
	Adjust func(delta int) // -1 for left, +1 for right
}

// Editor is the settings-screen model the scheduled screensavers share: the
// schedule rows, then the app's own fields, with a row cursor and a day cursor.
type Editor struct {
	Schedule *engine.Schedule
	Fields   []Field
	Row, Day int
	// On and Off are ENABLED's two readings; empty means "ON" and "OFF".
	On, Off string
}

// Action is what HandleKey did with a key.
type Action int

const (
	// Unhandled means the key does nothing on the current row.
	Unhandled Action = iota
	// Changed means the model changed and the screen needs repainting.
	Changed
	// Save means the user asked to save and leave.
	Save
)

// Rows is the number of rows, schedule rows included.
func (e *Editor) Rows() int { return RowFields + len(e.Fields) }

// Label is the row's caption.
func (e *Editor) Label(row int) string {
	switch row {
	case RowEnabled:
		return "ENABLED"
	case RowFrom:
		return "FROM"
	case RowTo:
		return "TO"
	case RowDays:
		return "DAYS"
	}
	return e.Fields[row-RowFields].Label
}

// Value is the row's displayed value; DAYS has none, the caller draws the week.
func (e *Editor) Value(row int) string {
	switch row {
	case RowEnabled:
		switch {
		case e.Schedule.Enabled && e.On != "":
			return e.On
		case e.Schedule.Enabled:
			return "ON"
		case e.Off != "":
			return e.Off
		}
		return "OFF"
	case RowFrom:
		return e.Schedule.From
	case RowTo:
		return e.Schedule.To
	case RowDays:
		return ""
	}
	return e.Fields[row-RowFields].Value()
}

// Adjust steps the current row's value by delta; on DAYS it moves the day cursor.
func (e *Editor) Adjust(delta int) {
	switch e.Row {
	case RowEnabled:
		e.Schedule.Enabled = !e.Schedule.Enabled
	case RowFrom:
		e.Schedule.From = ShiftHHMM(e.Schedule.From, delta*TimeStepMinutes)
	case RowTo:
		e.Schedule.To = ShiftHHMM(e.Schedule.To, delta*TimeStepMinutes)
	case RowDays:
		e.Day = (e.Day + 7 + delta) % 7
	default:
		e.Fields[e.Row-RowFields].Adjust(delta)
	}
}

// HandleKey applies a keypress: up and down move the row cursor with
// wraparound, left and right adjust, green toggles the cursor day on the DAYS
// row only, and red saves.
func (e *Editor) HandleKey(name input.KeyName) Action {
	switch name {
	case input.KeyUp:
		e.Row = (e.Row + e.Rows() - 1) % e.Rows()
	case input.KeyDown:
		e.Row = (e.Row + 1) % e.Rows()
	case input.KeyLeft:
		e.Adjust(-1)
	case input.KeyRight:
		e.Adjust(1)
	case input.KeyJoyA:
		if e.Row != RowDays {
			return Unhandled
		}
		e.Schedule.Days[e.Day] = !e.Schedule.Days[e.Day]
	case input.KeyJoyB:
		return Save
	default:
		return Unhandled
	}
	return Changed
}

// Lights are the lamps the screen lights: red always, green on the DAYS row.
func (e *Editor) Lights() hardware.ButtonLights {
	return hardware.ButtonLights{"joyb": true, "joya": e.Row == RowDays}
}

// ShiftHHMM moves an "HH:MM" time by mins, wrapping at midnight; an
// unparseable time shifts from 00:00.
func ShiftHHMM(s string, mins int) string {
	total := 0
	if hh, mm, ok := strings.Cut(s, ":"); ok {
		h, errH := strconv.Atoi(hh)
		m, errM := strconv.Atoi(mm)
		if errH == nil && errM == nil {
			total = h*60 + m
		}
	}
	total = ((total+mins)%1440 + 1440) % 1440
	return fmt.Sprintf("%02d:%02d", total/60, total%60)
}
