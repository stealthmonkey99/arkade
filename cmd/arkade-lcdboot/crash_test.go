// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCrashFromEnv(t *testing.T) {
	now := time.Now()
	env := func(vals map[string]string) func(string) string {
		return func(k string) string { return vals[k] }
	}
	if _, failed := crashFromEnv(env(map[string]string{"SERVICE_RESULT": "success", "EXIT_STATUS": "0"}), now); failed {
		t.Error("a clean stop must not raise a crash card over the engine's own goodbye")
	}
	if _, failed := crashFromEnv(env(nil), now); failed {
		t.Error("no SERVICE_RESULT at all is not a crash")
	}
	c, failed := crashFromEnv(env(map[string]string{"SERVICE_RESULT": "exit-code", "EXIT_CODE": "exited", "EXIT_STATUS": "2"}), now)
	if !failed || c.status != "2" || c.result != "exit-code" {
		t.Fatalf("a failed exit = %+v, failed=%v", c, failed)
	}
	if got := c.message(); got != "engine exit 2 - retry in 5s" {
		t.Errorf("message = %q", got)
	}
	killed, _ := crashFromEnv(env(map[string]string{"SERVICE_RESULT": "watchdog"}), now)
	if killed.status != "watchdog" {
		t.Errorf("a status-less failure = %q, want the result", killed.status)
	}
}

func TestReadCrashConsumesTheFile(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	if _, ok := readCrash(dir, now); ok {
		t.Error("no file is not a crash")
	}

	want := crash{result: "exit-code", status: "2", at: now.Add(-3 * time.Second)}
	if err := writeCrash(dir, want); err != nil {
		t.Fatalf("writeCrash: %v", err)
	}
	got, ok := readCrash(dir, now)
	if !ok || got.status != want.status || got.result != want.result {
		t.Fatalf("readCrash = %+v, %v; want %+v", got, ok, want)
	}
	if _, err := os.Stat(filepath.Join(dir, crashFile)); !os.IsNotExist(err) {
		t.Errorf("the file should be consumed, stat err = %v", err)
	}

	stale := crash{result: "exit-code", status: "2", at: now.Add(-crashWindow - time.Minute)}
	if err := writeCrash(dir, stale); err != nil {
		t.Fatalf("writeCrash: %v", err)
	}
	if _, ok := readCrash(dir, now); ok {
		t.Error("a stale file should not raise a crash card")
	}
	if _, err := os.Stat(filepath.Join(dir, crashFile)); !os.IsNotExist(err) {
		t.Errorf("a stale file should be consumed too, stat err = %v", err)
	}
}

func TestParseCrashRejectsRubbish(t *testing.T) {
	for _, raw := range []string{"", "status=2\n", "at=2001-01-01T18:04:00Z\n", "status=2\nat=yesterday\n"} {
		if _, err := parseCrash(raw); err == nil {
			t.Errorf("parseCrash(%q) should fail", raw)
		}
	}
	c, err := parseCrash("result=exit-code\nstatus=2\nat=2001-01-01T18:04:00Z\n")
	if err != nil {
		t.Fatalf("parseCrash: %v", err)
	}
	if c.status != "2" || c.result != "exit-code" || c.at.UTC().Format(time.RFC3339) != "2001-01-01T18:04:00Z" {
		t.Errorf("parseCrash = %+v", c)
	}
}
