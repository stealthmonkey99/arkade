// SPDX-License-Identifier: GPL-2.0-or-later

package carttest

import (
	"reflect"
	"testing"
)

// RequireRuntimesAgree runs a journey twice, once on the cabinet's wasm
// compiler and once on wazero's interpreter, and fails unless both hosts
// recorded identical frames, peripheral calls, lamp levels, sounds and logs.
// journey must build its cartridge with the Options it is handed and return
// the FakeHost it drove. A disagreement means one engine executes the
// cartridge wrongly, which a golden recorded on that engine cannot see.
func RequireRuntimesAgree(t *testing.T, journey func(opts Options) *FakeHost) {
	t.Helper()
	compiled := journey(Options{})
	interpreted := journey(Options{Interpreter: true})

	AssertBlitsIdentical(t, compiled, interpreted)
	if a, b := RecordedPeripherals(compiled), RecordedPeripherals(interpreted); !reflect.DeepEqual(a, b) {
		t.Errorf("peripheral calls differ:\ncompiler:    %+v\ninterpreter: %+v", a, b)
	}
	if !reflect.DeepEqual(compiled.LampLevels, interpreted.LampLevels) {
		t.Errorf("lamp levels differ:\ncompiler:    %+v\ninterpreter: %+v", compiled.LampLevels, interpreted.LampLevels)
	}
	if !reflect.DeepEqual(compiled.Sounds, interpreted.Sounds) {
		t.Errorf("sound calls differ:\ncompiler:    %+v\ninterpreter: %+v", compiled.Sounds, interpreted.Sounds)
	}
	if !reflect.DeepEqual(compiled.Logs, interpreted.Logs) {
		t.Errorf("logs differ:\ncompiler:    %+v\ninterpreter: %+v", compiled.Logs, interpreted.Logs)
	}
}
