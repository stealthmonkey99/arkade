// SPDX-License-Identifier: GPL-2.0-or-later

package net

import (
	"fmt"
	stdnet "net"
	"sync"
	"time"
)

const (
	// MinTokenLength is the shortest ARKADE_HTTP_TOKEN the control routes
	// accept. `openssl rand -hex 24`, the documented recipe, gives 48.
	MinTokenLength = 32
	// MinTokenDistinctBytes rejects long but guessable tokens such as "aaaa...".
	MinTokenDistinctBytes = 8

	// maxAuthFailures wrong tokens from one source within authFailureWindow
	// lock that source out of every token-gated route for authLockout.
	maxAuthFailures   = 10
	authFailureWindow = time.Minute
	authLockout       = time.Minute
	// maxAuthSources bounds the limiter's memory; sources past it share one
	// overflow slot, so a flood of addresses locks itself out together.
	maxAuthSources = 256

	// rejectLogInterval limits how often one kind of unauthenticated rejection
	// writes to the ring; repeats are counted onto the next line so such
	// traffic cannot flush the audit trail.
	rejectLogInterval = 30 * time.Second
)

// CheckToken reports why token is too weak to guard the control routes, or
// nil if it is acceptable. An empty token is not an error: it means remote
// control is switched off.
func CheckToken(token string) error {
	if token == "" {
		return nil
	}
	if len(token) < MinTokenLength {
		return fmt.Errorf("ARKADE_HTTP_TOKEN is %d characters; at least %d are required (generate one with `openssl rand -hex 24`)", len(token), MinTokenLength)
	}
	var seen [256]bool
	distinct := 0
	for i := 0; i < len(token); i++ {
		if !seen[token[i]] {
			seen[token[i]] = true
			distinct++
		}
	}
	if distinct < MinTokenDistinctBytes {
		return fmt.Errorf("ARKADE_HTTP_TOKEN uses only %d distinct characters; at least %d are required (generate one with `openssl rand -hex 24`)", distinct, MinTokenDistinctBytes)
	}
	return nil
}

type authSource struct {
	failures    int
	windowStart time.Time
	lockedUntil time.Time
}

type authLimiter struct {
	mu      sync.Mutex
	now     func() time.Time
	sources map[string]*authSource
}

func newAuthLimiter() *authLimiter {
	return &authLimiter{now: time.Now, sources: make(map[string]*authSource)}
}

func sourceOf(remoteAddr string) string {
	if host, _, err := stdnet.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	return remoteAddr
}

func (l *authLimiter) lockedFor(src string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	s := l.sources[l.key(src)]
	if s == nil {
		return 0
	}
	if d := s.lockedUntil.Sub(l.now()); d > 0 {
		return d
	}
	return 0
}

func (l *authLimiter) fail(src string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	s := l.sources[src]
	if s == nil {
		if len(l.sources) >= maxAuthSources {
			l.prune(now)
		}
		k := l.key(src)
		if s = l.sources[k]; s == nil {
			s = &authSource{}
			l.sources[k] = s
		}
	}
	if now.Sub(s.windowStart) > authFailureWindow {
		s.failures, s.windowStart = 0, now
	}
	s.failures++
	if s.failures >= maxAuthFailures {
		s.failures, s.windowStart = 0, now
		s.lockedUntil = now.Add(authLockout)
		return true
	}
	return false
}

func (l *authLimiter) succeed(src string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.sources, src)
}

func (l *authLimiter) key(src string) string {
	if _, ok := l.sources[src]; ok || len(l.sources) < maxAuthSources {
		return src
	}
	return "*"
}

func (l *authLimiter) prune(now time.Time) {
	for k, s := range l.sources {
		if now.After(s.lockedUntil) && now.Sub(s.windowStart) > authFailureWindow {
			delete(l.sources, k)
		}
	}
}

type rejectKind struct {
	last       time.Time
	suppressed int
}

type rejectLog struct {
	mu    sync.Mutex
	now   func() time.Time
	kinds map[string]*rejectKind
}

func newRejectLog() *rejectLog {
	return &rejectLog{now: time.Now, kinds: make(map[string]*rejectKind)}
}

func (l *rejectLog) append(ring *Ring, kind, msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	k := l.kinds[kind]
	if k == nil {
		k = &rejectKind{}
		l.kinds[kind] = k
	} else if now.Sub(k.last) < rejectLogInterval {
		k.suppressed++
		return
	}
	if k.suppressed > 0 {
		msg += fmt.Sprintf(" (+%d similar in the last %s)", k.suppressed, now.Sub(k.last).Round(time.Second))
	}
	k.last, k.suppressed = now, 0
	ring.Append("warn", msg)
}
