// SPDX-License-Identifier: GPL-2.0-or-later

package net

import (
	"sync"
	"time"
)

// LogEntry is one Ring line: level, message, a monotonic sequence number and
// the wall-clock append time. It must never feed back into engine logic.
type LogEntry struct {
	Seq   uint64    `json:"seq"`
	Level string    `json:"level"`
	Msg   string    `json:"msg"`
	Time  time.Time `json:"time"`
}

// Ring holds the most recent log lines; safe for concurrent use.
type Ring struct {
	mu      sync.Mutex
	entries []LogEntry
	size    int
	next    int
	count   int
	seq     uint64
}

// NewRing builds a Ring holding up to size entries; size<=0 means 500.
func NewRing(size int) *Ring {
	if size <= 0 {
		size = 500
	}
	return &Ring{entries: make([]LogEntry, size), size: size}
}

func (r *Ring) Append(level, msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	r.entries[r.next] = LogEntry{Seq: r.seq, Level: level, Msg: msg, Time: time.Now()}
	r.next = (r.next + 1) % r.size
	if r.count < r.size {
		r.count++
	}
}

// Tail returns up to n of the most recent entries, oldest first. n<=0 or
// n greater than the number of stored entries returns everything held.
func (r *Ring) Tail(n int) []LogEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n <= 0 || n > r.count {
		n = r.count
	}
	out := make([]LogEntry, n)
	start := (((r.next - n) % r.size) + r.size) % r.size
	for i := 0; i < n; i++ {
		out[i] = r.entries[(start+i)%r.size]
	}
	return out
}
