// SPDX-License-Identifier: GPL-2.0-or-later

package controller

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/stealthmonkey99/arkade/internal/input"
)

// DeviceInfo identifies an attached HID device.
type DeviceInfo struct {
	VID  uint16
	PID  uint16
	Path string
	// Bus is assetfile.BusUSB, assetfile.BusBluetooth, or "" when unknown.
	Bus string
	// Serial is empty on wired sticks; a Bluetooth pad reports its MAC.
	Serial string
}

// StableID is the name a seat is remembered by: the serial if any, else the
// platform path.
func (d DeviceInfo) StableID() string {
	if d.Serial != "" {
		return d.Serial
	}
	return d.Path
}

// Device is an open HID handle. The hiddev package provides the real one;
// tests provide fakes.
type Device interface {
	// Read blocks until an input report arrives (or the device closes).
	Read() ([]byte, error)
	Close() error
}

// Enumerator lists and opens HID devices.
type Enumerator interface {
	Enumerate() []DeviceInfo
	Open(path string) (Device, error)
}

const hotplugInterval = 2 * time.Second

// Manager owns the attached controllers: it enumerates devices matching a
// profile, runs a read goroutine per device that decodes reports into
// input events, and re-enumerates for hot-plug. Seats are assigned in
// attach order; a device past the two seats has input.NoSeat until it
// claims one.
type Manager struct {
	onSeatClaim func(device string, seat input.Seat)
	// onAttach is announced when a controller appears. Called from the
	// hot-plug goroutine, so a host must not block in it.
	onAttach func(Attached)
	handler  *input.Handler
	enum     Enumerator
	profiles []Profile
	logger   func(level, msg string)

	channels *ChannelBus

	mu      sync.Mutex
	active  map[string]*activeDevice
	session []Profile
	gen     int
	slots   map[string]*slot // path to persistent slot, survives a detach
	// seatMemory is StableID to seat; the path changes on reconnect, the
	// StableID does not.
	seatMemory map[string]input.Seat
	nextIdx    int
	quit       chan struct{}
	started    bool
	// stopping makes read loops exit without closing (Stop owns the close)
	// and halts new attaches; wg lets Stop wait out the read goroutines.
	stopping atomic.Bool
	wg       sync.WaitGroup
}

// slot keeps a controller's attach slot and seat across a reconnect on the
// same path.
type slot struct {
	slot input.Slot
	seat input.Seat
}

type activeDevice struct {
	info    DeviceInfo
	profile Profile
	dev     Device
	slot    input.Slot
	// gen is the session-profile generation the read loop last resolved at.
	gen        int
	selectHeld bool
	// seat is guarded by Manager.mu and changes only through setSeatLocked.
	seat input.Seat
}

// Attached describes one connected controller for pairing UIs.
type Attached struct {
	ID       string
	Name     string
	StableID string
	Bus      string
	Index    int
	Seat     input.Seat
}

func (a *activeDevice) attached() Attached {
	return Attached{
		ID: a.info.Path, Name: a.profile.Name, StableID: a.info.StableID(), Bus: a.info.Bus,
		Index: a.slot.Index(), Seat: a.seat,
	}
}

func NewManager(handler *input.Handler, enum Enumerator, logger func(level, msg string)) *Manager {
	if logger == nil {
		logger = func(string, string) {}
	}
	return &Manager{
		handler:    handler,
		enum:       enum,
		profiles:   Profiles,
		logger:     logger,
		channels:   NewChannelBus(),
		active:     map[string]*activeDevice{},
		seatMemory: map[string]input.Seat{},
		slots:      map[string]*slot{},
		quit:       make(chan struct{}),
	}
}

func (m *Manager) Channels() *ChannelBus { return m.channels }

// SetSessionProfiles installs the mounted cartridge's own HID profiles,
// replacing the whole set (nil clears them). Attached devices pick the new
// profile up on their next report. A profile that fails to compile is
// skipped with a warning.
func (m *Manager) SetSessionProfiles(specs []*Spec) {
	var profiles []Profile
	for _, spec := range specs {
		if spec == nil {
			continue
		}
		p, err := ProfileFromEnvelope(spec.Envelope)
		if err != nil {
			m.logger("warn", fmt.Sprintf("controller: session profile %q: %v", spec.Envelope.Name, err))
			continue
		}
		profiles = append(profiles, p)
	}
	m.mu.Lock()
	m.session = profiles
	m.gen++
	m.mu.Unlock()
	if len(profiles) > 0 {
		m.scan()
	}
}

// profileFor matches a device to a profile, session profiles first. Within a
// set, a profile naming the device's bus beats one naming none, and a
// profile naming a different bus never matches: a DualShock 4 over Bluetooth
// prefixes its reports with two header bytes, so its USB layout decodes wrong.
func (m *Manager) profileFor(info DeviceInfo) (Profile, bool) {
	m.mu.Lock()
	session := m.session
	m.mu.Unlock()
	for _, set := range [][]Profile{session, m.profiles} {
		var fallback Profile
		var haveFallback bool
		for _, p := range set {
			if p.VID != info.VID || p.PID != info.PID {
				continue
			}
			if p.Bus != "" && p.Bus == info.Bus {
				return p, true
			}
			if p.Bus == "" && !haveFallback {
				fallback, haveFallback = p, true
			}
		}
		if haveFallback {
			return fallback, true
		}
	}
	return Profile{}, false
}

func (m *Manager) Start() {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true
	m.mu.Unlock()

	m.scan()
	go m.hotplugLoop()
}

// Stop closes all devices, ends the hot-plug loop, and waits for every read
// goroutine to exit.
func (m *Manager) Stop() {
	if m.stopping.Swap(true) {
		return
	}
	close(m.quit)
	m.mu.Lock()
	for path, a := range m.active {
		_ = a.dev.Close()
		delete(m.active, path)
	}
	m.mu.Unlock()
	m.wg.Wait()
}

// Controllers returns a snapshot of attached controllers as name[index].
func (m *Manager) Controllers() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.active))
	for _, a := range m.active {
		out = append(out, fmt.Sprintf("%s[%d]", a.profile.Name, a.slot.Index()))
	}
	return out
}

func (m *Manager) List() []Attached {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Attached, 0, len(m.active))
	for _, a := range m.active {
		out = append(out, a.attached())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out
}

func (m *Manager) OnAttach(fn func(Attached)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onAttach = fn
}

// PairSeat assigns the controller with the given ID to input.SeatP1 or
// input.SeatP2, swapping with its holder. Unknown IDs and input.NoSeat are
// ignored.
func (m *Manager) PairSeat(id string, seat input.Seat) {
	if !seat.Valid() {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	target, ok := m.active[id]
	if !ok || target.seat == seat {
		return
	}
	for _, other := range m.active {
		if other.seat == seat {
			m.setSeatLocked(other, target.seat)
			break
		}
	}
	m.setSeatLocked(target, seat)
	m.logger("log", fmt.Sprintf("controller: %s paired to seat %v", target.profile.Name, seat))
}

// setSeatLocked moves a device, its slot and its channels to seat.
func (m *Manager) setSeatLocked(a *activeDevice, seat input.Seat) {
	if a.seat == seat {
		return
	}
	m.channels.Withdraw(a.seat, a.profile.Spec)
	a.seat = seat
	if s := m.slots[a.info.Path]; s != nil {
		s.seat = seat
	}
	m.channels.Offer(seat, a.profile.Spec)
}

func (m *Manager) hotplugLoop() {
	ticker := time.NewTicker(hotplugInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.quit:
			return
		case <-ticker.C:
			m.scan()
		}
	}
}

func (m *Manager) scan() {
	if m.stopping.Load() {
		return
	}
	for _, info := range m.enum.Enumerate() {
		profile, ok := m.profileFor(info)
		if !ok {
			continue
		}
		m.mu.Lock()
		_, already := m.active[info.Path]
		m.mu.Unlock()
		if already {
			continue
		}
		dev, err := m.enum.Open(info.Path)
		if err != nil {
			m.logger("warn", fmt.Sprintf("controller: open %s: %v", profile.Name, err))
			continue
		}
		m.attach(info, profile, dev)
	}
}

func (m *Manager) attach(info DeviceInfo, profile Profile, dev Device) {
	m.mu.Lock()
	// Checked under Stop's mutex so wg.Add cannot race wg.Wait.
	if m.stopping.Load() {
		m.mu.Unlock()
		_ = dev.Close()
		return
	}
	s, ok := m.slots[info.Path]
	if !ok {
		s = &slot{slot: input.SlotOf(m.nextIdx), seat: input.SeatOf(m.nextIdx)}
		m.nextIdx++
		m.slots[info.Path] = s
		if seat, remembered := m.seatMemory[info.StableID()]; remembered {
			s.seat = seat
		}
	}
	a := &activeDevice{info: info, profile: profile, dev: dev, slot: s.slot, seat: input.NoSeat, gen: m.gen}
	m.active[info.Path] = a
	// Offered at attach because a cartridge binds during cart_init, before an
	// idle stick has reported.
	m.setSeatLocked(a, s.seat)
	attached := a.attached()
	m.wg.Add(1)
	m.mu.Unlock()

	m.logger("log", fmt.Sprintf("controller: %s attached in %v at %v", profile.Name, a.slot, a.seat))
	m.mu.Lock()
	onAttach := m.onAttach
	m.mu.Unlock()
	if onAttach != nil {
		onAttach(attached)
	}
	go func() {
		defer m.wg.Done()
		m.readLoop(a)
	}()
}

func (m *Manager) detach(a *activeDevice, cause error) {
	m.mu.Lock()
	if m.active[a.info.Path] == a {
		delete(m.active, a.info.Path)
	}
	m.channels.Withdraw(a.seat, a.profile.Spec)
	m.mu.Unlock()
	_ = a.dev.Close()
	m.logger("log", fmt.Sprintf("controller: %s (%v) detached: %v", a.profile.Name, a.slot, cause))
}

func (m *Manager) readLoop(a *activeDevice) {
	decoder := NewDecoder(a.profile.Spec)
	for {
		data, err := a.dev.Read()
		if err != nil {
			// Detaching during Stop would close the handle twice, a double
			// free in hidapi.
			if m.stopping.Load() {
				return
			}
			m.detach(a, err)
			return
		}
		// A new Decoder is needed on a profile swap: the old one's primed
		// state would fire phantom edges.
		m.mu.Lock()
		gen := m.gen
		m.mu.Unlock()
		if gen != a.gen {
			if p, ok := m.profileFor(a.info); ok && p.Name != a.profile.Name {
				m.mu.Lock()
				m.channels.Withdraw(a.seat, a.profile.Spec)
				a.profile = p
				m.channels.Offer(a.seat, p.Spec)
				m.mu.Unlock()
				decoder = NewDecoder(p.Spec)
			}
			a.gen = gen
		}
		m.mu.Lock()
		seat := a.seat
		m.channels.Publish(seat, a.profile.Spec, data)
		m.mu.Unlock()
		source := input.PlayerSource{
			Type: input.SourceController,
			ID:   a.info.Path,
			Slot: a.slot,
			Seat: seat,
		}
		for _, ev := range decoder.Feed(data) {
			if m.seatChord(a, ev) {
				continue
			}
			ev.Source = source
			m.handler.Push(ev)
		}
	}
}
