// SPDX-License-Identifier: GPL-2.0-or-later

package shell

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/stealthmonkey99/arkade/internal/engine"
)

func TestSettingsMRUReadsBackAsTheEnginesRecents(t *testing.T) {
	want := []string{"cart/snake", "cart/life"}
	raw, err := json.Marshal(settings{ShellRecents: engine.ShellRecents{MRU: want}, Favorites: []string{"cart/life"}})
	if err != nil {
		t.Fatal(err)
	}
	var got engine.ShellRecents
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.MRU, want) {
		t.Errorf("engine read MRU %v from %s, want %v", got.MRU, raw, want)
	}
}
