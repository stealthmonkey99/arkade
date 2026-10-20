// SPDX-License-Identifier: GPL-2.0-or-later

package wasm

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stealthmonkey99/arkade/internal/lcdrune"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

var writeHostABI = flag.Bool("write-hostabi", false, "rewrite testdata/hostabi.json from the live tables")

const hostABIPath = "testdata/hostabi.json"

// hostABITable is every wire table and sandbox cap the browser host must
// match; its TypeScript twin is the web host's hostAbiParity test, which
// reads the file this test keeps current.
type hostABITable struct {
	Keys          []string         `json:"keys"`
	Joys          []string         `json:"joys"`
	Buttons       []string         `json:"buttons"`
	StrategyKinds map[string]int   `json:"strategyKinds"`
	SwitchModes   map[string]int   `json:"switchModes"`
	LogLevels     map[string]int   `json:"logLevels"`
	Caps          map[string]int64 `json:"caps"`
	RecentNames   int              `json:"recentNames"`
}

func liveHostABITable() hostABITable {
	t := hostABITable{
		RecentNames: ui.NameEntryMaxRecents,
		StrategyKinds: map[string]int{
			"AnyPlayer":          int(StrategyAnyPlayer),
			"OnePlayerSolo":      int(StrategyOnePlayerSolo),
			"OnePlayerAgainstAI": int(StrategyOnePlayerAgainstAI),
			"TwoPlayerTurnBased": int(StrategyTwoPlayerTurnBased),
		},
		SwitchModes: map[string]int{
			"Toggle":      int(SwitchToggle),
			"ToPrimary":   int(SwitchToPrimary),
			"ToSecondary": int(SwitchToSecondary),
		},
		LogLevels: map[string]int{
			"Debug": int(LogDebug),
			"Info":  int(LogInfo),
			"Warn":  int(LogWarn),
			"Error": int(LogError),
		},
		Caps: map[string]int64{
			"MAX_LOG_BYTES":          maxLogBytes,
			"MAX_KEY_BYTES":          maxKeyBytes,
			"MAX_STORAGE_BYTES":      maxStorageBytes,
			"MAX_SPRITE_BYTES":       maxSpriteBytes,
			"MAX_SOUND_PATH_LEN":     maxSoundPathLen,
			"MAX_BLIT_BYTES":         maxBlitBytes,
			"MAX_WASI_RAND_BYTES":    maxWASIRandBytes,
			"MAX_WASI_IOVECS":        maxWASIIovecs,
			"MAX_LCD_LINE_BYTES":     maxLcdLineBytes,
			"MAX_CART_GLYPHS":        lcdrune.MaxCartGlyphs,
			"MAX_NAME_BYTES":         maxNameBytes,
			"MAX_SCORE":              maxScore,
			"MAX_SHEET_PATH_LEN":     maxSheetPathLen,
			"MAX_SHEET_BYTES":        maxSheetBytes,
			"MAX_CEL_NAME_BYTES":     maxCelNameBytes,
			"MAX_SHEETS_PER_CART":    maxSheetsPerCart,
			"MAX_CELS_PER_CART":      maxCelsPerCart,
			"MAX_CEL_DIM":            maxCelDim,
			"MAX_CEL_PIXEL_BYTES":    maxCelPixelBytes,
			"MAX_SHEET_DECODE_BYTES": maxSheetPixelBytes,
			"MAX_BOUND_CHANNELS":     maxBoundChannels,
			"MAX_CHANNEL_BLOB_BYTES": maxChannelBlobBytes,
			"MAX_CHANNEL_NAME_BYTES": maxChannelNameBytes,
			"MAX_BUNDLE_PROFILES":    maxBundleProfiles,
			"MAX_LIGHT_SEQUENCES":    maxLightSequences,
			"MAX_LIGHTS_PATH_LEN":    maxLightsPathLen,
			"LAMP_LEVEL_MAX":         lampLevelMax,
			"MAX_LAMP_FADE_MS":       maxLampFadeMs,
			"MAX_ALLOC_BYTES":        maxAllocBytes,
			"MAX_INPUT_RECORDS":      maxInputRecords,
			"MAX_MANIFEST_BYTES":     maxManifestBytes,
		},
	}
	for _, k := range keyOrdinal {
		t.Keys = append(t.Keys, string(k))
	}
	for _, j := range joyOrdinal {
		t.Joys = append(t.Joys, string(j))
	}
	for _, b := range buttonOrdinal {
		t.Buttons = append(t.Buttons, string(b))
	}
	return t
}

func TestHostABITableFileIsCurrent(t *testing.T) {
	want, err := json.MarshalIndent(liveHostABITable(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')
	if *writeHostABI {
		if err := os.WriteFile(filepath.FromSlash(hostABIPath), want, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(filepath.FromSlash(hostABIPath))
	if err != nil {
		t.Fatalf("%v (regenerate with go test -run TestHostABITableFileIsCurrent -args -write-hostabi)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s is stale: a wire table or sandbox cap changed. Regenerate with go test -run TestHostABITableFileIsCurrent -args -write-hostabi, then bring the browser host's keys.ts and caps.ts to match.", hostABIPath)
	}
}
