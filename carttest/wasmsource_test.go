// SPDX-License-Identifier: GPL-2.0-or-later

package carttest

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// engineRoot is the engine checkout this test was compiled from.
func engineRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(filepath.Dir(file))
}

func TestBuildArgsMatchTheCartridgeMakefiles(t *testing.T) {
	for _, mk := range []string{"sdk/template/Makefile", "sdk/tutorial/Makefile"} {
		raw, err := os.ReadFile(filepath.Join(engineRoot(), mk))
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, line := range strings.Split(string(raw), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 || fields[0] != "tinygo" || fields[1] != "build" {
				continue
			}
			for _, f := range fields[2:] {
				if f == "-o" {
					break
				}
				got = append(got, f)
			}
		}
		if got == nil {
			t.Fatalf("%s has no tinygo build recipe", mk)
		}
		if !reflect.DeepEqual(got, tinyGoCartridgeBuildArgs) {
			t.Errorf("%s builds with %v, tinyGoCartridgeBuildArgs is %v", mk, got, tinyGoCartridgeBuildArgs)
		}
	}
}
