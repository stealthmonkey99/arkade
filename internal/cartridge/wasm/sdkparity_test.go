// SPDX-License-Identifier: GPL-2.0-or-later

package wasm

// The SDK is a separate module and cannot import this package, so it
// carries copies of the wire tables and the sandbox caps. These tests are
// what keep the copies honest. sdk/ui/input is importable directly; the
// cartlib caps are read out of its source by the type checker, because
// importing cartlib here would pull a wasm-only package into the engine.

import (
	"github.com/stealthmonkey99/arkade/internal/lcdrune"
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/internal/input"
	sdkinput "github.com/stealthmonkey99/arkade/sdk/ui/input"
)

func TestSDKKeyOrdinalsMatchTheWire(t *testing.T) {
	sdk := map[input.KeyName]uint8{
		input.KeyBrightness: sdkinput.KeyBrightness,
		input.KeyRotate:     sdkinput.KeyRotate,
		input.KeyQuit:       sdkinput.KeyQuit,
		input.KeyEscape:     sdkinput.KeyEscape,
		input.KeyEnter:      sdkinput.KeyEnter,
		input.KeySpace:      sdkinput.KeySpace,
		input.KeyUp:         sdkinput.KeyUp,
		input.KeyDown:       sdkinput.KeyDown,
		input.KeyLeft:       sdkinput.KeyLeft,
		input.KeyRight:      sdkinput.KeyRight,
		input.KeyHome:       sdkinput.KeyHome,
		input.KeyEnd:        sdkinput.KeyEnd,
		input.KeyJoyA:       sdkinput.KeyJoyA,
		input.KeyJoyB:       sdkinput.KeyJoyB,
		input.KeyJoyC:       sdkinput.KeyJoyC,
		input.KeyJoyD:       sdkinput.KeyJoyD,
		input.KeyL1:         sdkinput.KeyL1,
		input.KeyR1:         sdkinput.KeyR1,
		input.KeyL2:         sdkinput.KeyL2,
		input.KeyR2:         sdkinput.KeyR2,
		input.KeyJoyAux1:    sdkinput.KeyJoyAux1,
		input.KeyJoyAux2:    sdkinput.KeyJoyAux2,
		input.KeyJoyAux3:    sdkinput.KeyJoyAux3,
		input.KeyJoyAux4:    sdkinput.KeyJoyAux4,
		input.KeySelect:     sdkinput.KeySelect,
		input.KeyStart:      sdkinput.KeyStart,
		input.KeyTurbo:      sdkinput.KeyTurbo,
		input.KeyClear:      sdkinput.KeyClear,
		input.KeyMicWhistle: sdkinput.KeyMicWhistle,
		input.KeyMicClap:    sdkinput.KeyMicClap,
		input.KeyMicBlow:    sdkinput.KeyMicBlow,
		input.KeyMicCue0:    sdkinput.KeyMicCue0,
		input.KeyMicCue1:    sdkinput.KeyMicCue1,
		input.KeyMicCue2:    sdkinput.KeyMicCue2,
		input.KeyMicCue3:    sdkinput.KeyMicCue3,
		input.KeyMicCue4:    sdkinput.KeyMicCue4,
		input.KeyMicCue5:    sdkinput.KeyMicCue5,
		input.KeyMicCue6:    sdkinput.KeyMicCue6,
		input.KeyMicCue7:    sdkinput.KeyMicCue7,
	}
	if len(sdk) != len(keyOrdinal) {
		t.Fatalf("the wire has %d keys and sdk/ui/input names %d; add the missing Key constant", len(keyOrdinal), len(sdk))
	}
	for name, got := range sdk {
		want, ok := KeyToOrdinal(name)
		if !ok {
			t.Errorf("sdk/ui/input names %q, which the wire does not carry", name)
			continue
		}
		if got != want {
			t.Errorf("sdk/ui/input puts %q at ordinal %d; the wire has %d", name, got, want)
		}
	}
}

func TestSDKJoystickOrdinalsMatchTheWire(t *testing.T) {
	sdk := []uint8{
		sdkinput.JoyN, sdkinput.JoyNE, sdkinput.JoyE, sdkinput.JoySE,
		sdkinput.JoyS, sdkinput.JoySW, sdkinput.JoyW, sdkinput.JoyNW,
	}
	if sdkinput.JoyCenter != 0 {
		t.Errorf("JoyCenter = %d, the wire's centre is 0", sdkinput.JoyCenter)
	}
	for i, pos := range joyOrdinal {
		if got := joyToOrdinal(pos); got != sdk[i] {
			t.Errorf("%s: wire ordinal %d, sdk/ui/input %d", pos, got, sdk[i])
		}
	}
}

func TestSDKRecordBytesMatchTheWire(t *testing.T) {
	pairs := []struct {
		name      string
		sdk, wire uint8
	}{
		{"KindKeydown", sdkinput.KindKeydown, RecordKindKeydown},
		{"KindKeyup", sdkinput.KindKeyup, RecordKindKeyup},
		{"KindKeypress", sdkinput.KindKeypress, RecordKindKeypress},
		{"KindJoystick", sdkinput.KindJoystick, RecordKindJoystickChange},
		{"SourceKeyboard", sdkinput.SourceKeyboard, RecordSourceKeyboard},
		{"SourceController", sdkinput.SourceController, RecordSourceController},
		{"FlagVirtual", sdkinput.FlagVirtual, RecordFlagVirtual},
		{"FlagShift", sdkinput.FlagShift, RecordFlagShift},
		{"SeatNone", uint8(sdkinput.SeatNone), SeatNone},
		{"SeatP1", uint8(sdkinput.SeatP1), input.SeatP1.Byte()},
		{"SeatP2", uint8(sdkinput.SeatP2), input.SeatP2.Byte()},
		{"NoSeat", uint8(sdkinput.SeatNone), input.NoSeat.Byte()},
		{"NoSlot", uint8(sdkinput.SeatNone), input.NoSlot.Byte()},
	}
	for _, p := range pairs {
		if p.sdk != p.wire {
			t.Errorf("%s: sdk/ui/input %d, wire %d", p.name, p.sdk, p.wire)
		}
	}
	for _, seat := range []input.Seat{input.SeatP1, input.SeatP2} {
		if sdkinput.SeatBoth == sdkinput.Seat(seat.Byte()) {
			t.Error("SeatBoth collides with a seat the wire can carry")
		}
	}
}

func TestCartlibCapsMatchTheHost(t *testing.T) {
	consts := cartlibConstants(t)
	caps := []struct {
		name string
		host int64
	}{
		{"maxDecodedRecords", maxInputRecords},
		{"MaxStorageKeyLen", maxKeyBytes},
		{"MaxStorageValueLen", maxStorageBytes},
		{"MaxNameLen", maxNameBytes},
		{"MaxBoundChannels", maxBoundChannels},
		{"MaxBlobBytes", maxChannelBlobBytes},
		{"MaxCartGlyphs", lcdrune.MaxCartGlyphs},
		{"maxCelDim", maxCelDim},
	}
	for _, c := range caps {
		v, ok := consts[c.name]
		if !ok {
			t.Errorf("sdk/cartlib no longer declares %s", c.name)
			continue
		}
		if got, _ := constant.Int64Val(v); got != c.host {
			t.Errorf("sdk/cartlib %s = %d; the host's cap is %d", c.name, got, c.host)
		}
	}
	masks := map[string]string{
		"joya": "MaskJoyA", "joyb": "MaskJoyB", "joyc": "MaskJoyC",
		"joyd": "MaskJoyD", "joyaux1": "MaskJoyAux1", "joyaux2": "MaskJoyAux2",
	}
	if len(masks) != len(buttonOrdinal) {
		t.Fatalf("the host has %d lamps and this test names %d", len(buttonOrdinal), len(masks))
	}
	for i, name := range buttonOrdinal {
		mask := masks[name]
		v, ok := consts[mask]
		if !ok {
			t.Errorf("sdk/cartlib has no %s for lamp %q (bit %d)", mask, name, i)
			continue
		}
		if got, _ := constant.Uint64Val(v); got != 1<<uint(i) {
			t.Errorf("sdk/cartlib %s = %#x; the host reads lamp %q from bit %d", mask, got, name, i)
		}
	}
}

// cartlibConstants type-checks sdk/cartlib's host-Go sources on their own
// and returns every package-level constant whose value the checker could
// fold. Imports resolve to empty packages, so anything defined in terms of
// another package is simply absent, which is fine: the caps are literals.
func cartlibConstants(t *testing.T) map[string]constant.Value {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "sdk", "cartlib")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, e := range entries {
		n := e.Name()
		if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") || strings.HasSuffix(n, "_tinygo.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, n), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", n, err)
		}
		files = append(files, f)
	}
	conf := types.Config{Importer: emptyImporter{}, Error: func(error) {}}
	pkg, _ := conf.Check("cartlib", fset, files, nil)
	out := map[string]constant.Value{}
	for _, name := range pkg.Scope().Names() {
		if c, ok := pkg.Scope().Lookup(name).(*types.Const); ok && c.Val().Kind() != constant.Unknown {
			out[name] = c.Val()
		}
	}
	return out
}

type emptyImporter struct{}

func (emptyImporter) Import(p string) (*types.Package, error) {
	pkg := types.NewPackage(p, path.Base(p))
	pkg.MarkComplete()
	return pkg, nil
}
