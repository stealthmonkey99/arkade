// SPDX-License-Identifier: GPL-2.0-or-later

// Package net is the cabinet's HTTP surface: a status, logs and scores JSON
// API plus a self-contained dashboard, served beside the game loop.
//
// Mutating routes are bearer-token gated and audited to the Ring. Handlers
// reach the engine only through engine.Remote, whose methods are safe from any
// goroutine and never call into a running app.
//
// Uptime and Ring timestamps use time.Now because they describe the real
// process, not engine time.
package net
