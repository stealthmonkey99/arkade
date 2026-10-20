// SPDX-License-Identifier: GPL-2.0-or-later

package wasm

import (
	"context"

	"github.com/tetratelabs/wazero/api"
)

type speakerImports struct {
	host interface {
		Speaker
		Logger
	}
}

func (s speakerImports) load(_ context.Context, mod api.Module, ptr, length uint32) uint32 {
	path, ok := guestString(s.host, mod, "host_load_sound:", ptr, length, maxSoundPathLen)
	if !ok {
		return 0
	}
	id, err := s.host.LoadSound(path)
	if err != nil {
		hostWarnf(s.host, "host_load_sound: %v", err)
		return 0
	}
	return id
}

func (s speakerImports) play(_ context.Context, _ api.Module, id, loopCount uint32) {
	s.host.PlaySound(id, loopCount)
}

func (s speakerImports) stop(_ context.Context, _ api.Module, id uint32) {
	s.host.StopSound(id)
}

func (s speakerImports) stopAll(_ context.Context, _ api.Module) {
	s.host.StopAllSounds()
}
