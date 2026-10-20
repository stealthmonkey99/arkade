// SPDX-License-Identifier: GPL-2.0-or-later

package fleet

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EngineModule is the go.mod line that marks the engine checkout.
const EngineModule = "module github.com/stealthmonkey99/arkade"

// EngineRoot walks up from start to the first directory whose go.mod
// declares EngineModule. Nested modules on the way (sdk/template) are
// walked through.
func EngineRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if declaresEngine(filepath.Join(dir, "go.mod")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no Årkade repo root above %s (looking for a go.mod declaring %q)", start, EngineModule)
		}
		dir = parent
	}
}

func declaresEngine(goMod string) bool {
	raw, err := os.ReadFile(goMod)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == EngineModule {
			return true
		}
	}
	return false
}
