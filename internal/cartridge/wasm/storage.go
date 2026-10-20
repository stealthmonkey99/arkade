// SPDX-License-Identifier: GPL-2.0-or-later

package wasm

import (
	"context"
	"encoding/binary"

	"github.com/tetratelabs/wazero/api"
)

type storageImports struct {
	host interface {
		Storage
		Logger
	}
}

func (s storageImports) read(_ context.Context, mod api.Module, keyPtr, keyLen, outPtr, outCap uint32) uint32 {
	key, ok := guestString(s.host, mod, "host_storage_read: key", keyPtr, keyLen, maxKeyBytes)
	if !ok {
		return 0
	}
	data, found := s.host.StorageRead(key)
	if !found || len(data) == 0 {
		return 0
	}
	n := min(uint32(len(data)), outCap, maxStorageBytes)
	if n == 0 {
		return 0
	}
	if !mod.Memory().Write(outPtr, data[:n]) {
		hostWarnf(s.host, "host_storage_read: out ptr %#x/%d out of bounds", outPtr, n)
		return 0
	}
	return n
}

func (s storageImports) write(_ context.Context, mod api.Module, keyPtr, keyLen, valPtr, valLen uint32) {
	key, ok := guestString(s.host, mod, "host_storage_write: key", keyPtr, keyLen, maxKeyBytes)
	if !ok {
		return
	}
	val, ok := guestBytes(s.host, mod, "host_storage_write: value", valPtr, valLen, maxStorageBytes)
	if !ok {
		return
	}
	s.host.StorageWrite(key, val)
}

// scoreImports passes scores as i64 and rejects negatives, so a cart cannot
// probe the table.
type scoreImports struct {
	host interface {
		Scoreboard
		Logger
	}
}

func (s scoreImports) submit(_ context.Context, mod api.Module, score int64, namePtr, nameLen uint32) uint32 {
	name, ok := guestString(s.host, mod, "host_score_submit: name", namePtr, nameLen, maxNameBytes)
	if !ok {
		return 0
	}
	if score < 0 || score > maxScore {
		hostWarnf(s.host, "host_score_submit: score %d out of range [0,%d]", score, maxScore)
		return 0
	}
	return uint32(s.host.ScoreSubmit(int(score), name))
}

// read writes a little-endian i64 score then the UTF-8 name; 0 past the end.
func (s scoreImports) read(_ context.Context, mod api.Module, rank, outPtr, outCap uint32) uint32 {
	score, name, ok := s.host.ScoreRead(int(rank))
	if !ok {
		return 0
	}
	if len(name) > maxNameBytes {
		name = name[:maxNameBytes]
	}
	buf := make([]byte, 8+len(name))
	binary.LittleEndian.PutUint64(buf, uint64(score))
	copy(buf[8:], name)
	if uint32(len(buf)) > outCap {
		hostWarnf(s.host, "host_score_read: entry needs %d bytes, guest offered %d", len(buf), outCap)
		return 0
	}
	if !mod.Memory().Write(outPtr, buf) {
		hostWarnf(s.host, "host_score_read: out ptr %#x/%d out of bounds", outPtr, len(buf))
		return 0
	}
	return uint32(len(buf))
}

func (s scoreImports) count(_ context.Context, _ api.Module) uint32 {
	return uint32(s.host.ScoreCount())
}

func (s scoreImports) qualifies(_ context.Context, _ api.Module, score int64) uint32 {
	if score < 0 || score > maxScore || !s.host.ScoreQualifies(int(score)) {
		return 0
	}
	return 1
}

func (s scoreImports) rank(_ context.Context, _ api.Module, score int64) uint32 {
	if score < 0 || score > maxScore {
		return 0
	}
	return uint32(s.host.ScoreRank(int(score)))
}

func (s scoreImports) recentName(_ context.Context, mod api.Module, player int32, outPtr, outCap uint32) uint32 {
	seat, ok := physicalSeatArg(s.host, "host_recent_name_get: player", player)
	if !ok {
		return 0
	}
	name, ok := s.host.RecentName(seat)
	if !ok {
		return 0
	}
	return writeName(s.host, mod, "host_recent_name_get", name, outPtr, outCap)
}

func (s scoreImports) recentNameAt(_ context.Context, mod api.Module, player int32, index, outPtr, outCap uint32) uint32 {
	seat, ok := physicalSeatArg(s.host, "host_recent_name_at: player", player)
	if !ok {
		return 0
	}
	name, ok := s.host.RecentNameAt(seat, int(index))
	if !ok {
		return 0
	}
	return writeName(s.host, mod, "host_recent_name_at", name, outPtr, outCap)
}

func (s scoreImports) noteRecentName(_ context.Context, mod api.Module, player int32, namePtr, nameLen uint32) {
	seat, ok := physicalSeatArg(s.host, "host_recent_name_note: player", player)
	if !ok {
		return
	}
	name, ok := guestString(s.host, mod, "host_recent_name_note: name", namePtr, nameLen, maxNameBytes)
	if !ok {
		return
	}
	s.host.NoteRecentName(seat, name)
}

// writeName returns the bytes written, or 0 when the name is empty or does
// not fit.
func writeName(log Logger, mod api.Module, what, name string, outPtr, outCap uint32) uint32 {
	if len(name) > maxNameBytes {
		name = name[:maxNameBytes]
	}
	if name == "" || uint32(len(name)) > outCap {
		return 0
	}
	if !mod.Memory().Write(outPtr, []byte(name)) {
		hostWarnf(log, "%s: out ptr %#x/%d out of bounds", what, outPtr, len(name))
		return 0
	}
	return uint32(len(name))
}
