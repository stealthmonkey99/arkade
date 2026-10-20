// SPDX-License-Identifier: GPL-2.0-or-later

package engine

// Remote is the part of the Runtime an HTTP handler may touch: published
// snapshots and requests drained at the top of a tick, every method safe
// from any goroutine. Nothing here reads engine state live.
type Remote interface {
	// Status is the engine's last published state snapshot.
	Status() Status
	// PeriphSnapshot is the last published peripheral snapshot, zero until a tick after WatchPeriph.
	PeriphSnapshot() PeriphSnapshot
	// WatchPeriph arms peripheral observation until the returned release is called.
	WatchPeriph() (release func())
	// DrainPeriphInputs returns the observed inputs from cursor on, the next cursor and how many were lost.
	DrainPeriphInputs(cursor uint64) (events []PeriphInput, next, missed uint64)
	// DumpFlight writes the flight recorder's ring to a file and reports what it wrote.
	DumpFlight(reason, note string) (FlightDump, error)
	// RequestAttractPreview shows slug's attract slot at the top of the next tick.
	RequestAttractPreview(slug string)
	// ShowScreensaver pins slug's screensaver on at the next tick, refusing a slug that is not one.
	ShowScreensaver(slug string) error
	// RequestLabPreview overlays a lab preview on the panel, LCDs or speaker at the next tick.
	RequestLabPreview(p LabPreview)
	// CancelLabPreview ends any lab preview at the next tick.
	CancelLabPreview()
	// RequestLightShow plays a lamp choreography from the next tick.
	RequestLightShow(s LightShow)
	// CancelLightShow ends any choreography at the next tick.
	CancelLightShow()
	// Microphone is the cabinet's sound-model sink, nil without a microphone.
	Microphone() MicModels
}

var _ Remote = (*Runtime)(nil)

// Microphone returns Options.Mic, nil on a cabinet without one.
func (rt *Runtime) Microphone() MicModels { return rt.Mic }
