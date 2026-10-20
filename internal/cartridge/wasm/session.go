// SPDX-License-Identifier: GPL-2.0-or-later

package wasm

import (
	"context"

	"github.com/tetratelabs/wazero/api"
)

type sessionImports struct {
	host Session
}

func (s sessionImports) nowMicros(_ context.Context, _ api.Module) uint64 {
	return s.host.NowMicros()
}

func (s sessionImports) randomU64(_ context.Context, _ api.Module) uint64 {
	return s.host.RandomU64()
}

func (s sessionImports) log(_ context.Context, mod api.Module, level, ptr, length uint32) {
	msg, ok := guestString(s.host, mod, "host_log:", ptr, length, maxLogBytes)
	if !ok {
		return
	}
	lv := LogLevel(level)
	if lv > LogError {
		lv = LogError
	}
	s.host.Log(lv, msg)
}
