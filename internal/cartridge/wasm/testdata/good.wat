;; good.wat — a well-behaved cartridge fixture for internal/cartridge/wasm
;; tests. Hand-written (not TinyGo output) so the test suite can pin exact
;; expected host-call sequences and doesn't need wabt at test time — see
;; the go:generate line below for the wat2wasm invocation that produced
;; good.wasm (checked in alongside this file).
;;
;; IMPORTANT: this fixture assumes the host calls NewCartridge with
;; Options{Width: 4, Height: 4} — host_blit's payload below is a hardcoded
;; 4x4x4-byte (64 byte) RGBA buffer, and cart_render's host_draw_pixel calls
;; assume a 4x4 canvas. Using any other size will make the host reject the
;; host_blit call's length as a bounds-check failure.
;;
;; Memory layout (all offsets/lengths generated once with a throwaway
;; script and hand-verified against the raw bytes below — see the "Go
;; const block" values mirrored in cartridge_test.go):
;;   0   len 10  "last_input"        storage key cart_handle_input echoes into
;;   16  len 10  "now_micros"        storage key for host_now_micros() echo
;;   32  len 10  "random_u64"        storage key for host_random_u64() echo
;;   48  len 11  "wasi_now_ns"       storage key for wasi clock_time_get() echo
;;   64  len 9   "wasi_rand"         storage key for wasi random_get() echo
;;   80  len 9   "dt_micros"         storage key for cart_update's dt echo
;;   96  len 2   "OK"                host_draw_text payload
;;   112 len 9   "render ok"         host_log message from cart_render
;;   128 len 8   "teardown"          host_log message from cart_teardown
;;   144 len 4   "bye\n"             fd_write payload (WASI -> host_log bridge)
;;   160 len 8   "PNGFAKE1"          host_load_sprite payload (bridge doesn't
;;                                   validate PNG-ness, only bounds/length)
;;   176 len 16  "assets/sound.wav"  host_load_sound payload
;;   192 len 64  4x4 RGBA gradient   host_blit payload
;;   256 len 7   "init ok"           host_log message from cart_init
;;   272 len 110 manifest JSON       cart_manifest() return
;;   384 len 256 (zeroed)            cart_alloc's reused static buffer
;;   640 len 64  (zeroed) scratch:
;;     640 now_micros (i64)     648 random_u64 (i64)
;;     656 wasi_rand (8 bytes)  664 wasi_now_ns (i64)
;;     672 dt_micros (i64)      680 fd_write nwritten (i32)
;;     688 fd_write iov0 {ptr,len} (8 bytes)
;;   704 len 3   "ABC"               name submitted to the score table
;;   720 len 72  (zeroed)            host_score_read output buffer
;;   800 len 64  (zeroed)            host_recent_name_get output buffer
;;   880 len 8   "mru_name"          storage key for the MRU walk's hit
;;   888 len 8   "mru_past"          storage key for the past-the-end read
;;   896 len 64  (zeroed)            host_recent_name_at output buffer
;;   976 len 4   (zeroed)            past-the-end byte count (i32)
;;   992 len 27  "assets/blip.arkadesound.wav"
;;                                   host_load_sound payload for an ASSET
;;                                   FILE (docs/ASSETS.md's sound kind).
;;                                   It is here to pin that host_load_sound
;;                                   is untouched by the envelope: the path
;;                                   is longer and the file carries an
;;                                   "arkd" chunk, and the import still just
;;                                   hands the string to the host.
;;
;; go:generate wat2wasm good.wat -o good.wasm
(module
  ;; --- "arkade" host imports (docs/CARTRIDGE.md "Host imports") ---
  (import "arkade" "host_blit" (func $host_blit (param i32 i32)))
  (import "arkade" "host_draw_pixel" (func $host_draw_pixel (param i32 i32 i32)))
  (import "arkade" "host_fill_rect" (func $host_fill_rect (param i32 i32 i32 i32 i32)))
  (import "arkade" "host_load_sprite" (func $host_load_sprite (param i32 i32) (result i32)))
  (import "arkade" "host_draw_sprite" (func $host_draw_sprite (param i32 i32 i32)))
  (import "arkade" "host_load_sound" (func $host_load_sound (param i32 i32) (result i32)))
  (import "arkade" "host_play_sound" (func $host_play_sound (param i32 i32)))
  (import "arkade" "host_stop_sound" (func $host_stop_sound (param i32)))
  (import "arkade" "host_stop_all_sounds" (func $host_stop_all_sounds))
  (import "arkade" "host_storage_read" (func $host_storage_read (param i32 i32 i32 i32) (result i32)))
  (import "arkade" "host_storage_write" (func $host_storage_write (param i32 i32 i32 i32)))
  (import "arkade" "host_now_micros" (func $host_now_micros (result i64)))
  (import "arkade" "host_random_u64" (func $host_random_u64 (result i64)))
  (import "arkade" "host_log" (func $host_log (param i32 i32 i32)))

  ;; --- scores, recent names, strategy ---
  (import "arkade" "host_reject_key" (func $host_reject_key))
  (import "arkade" "host_score_submit" (func $host_score_submit (param i64 i32 i32) (result i32)))
  (import "arkade" "host_score_read" (func $host_score_read (param i32 i32 i32) (result i32)))
  (import "arkade" "host_score_count" (func $host_score_count (result i32)))
  (import "arkade" "host_score_qualifies" (func $host_score_qualifies (param i64) (result i32)))
  (import "arkade" "host_score_rank" (func $host_score_rank (param i64) (result i32)))
  (import "arkade" "host_recent_name_get" (func $host_recent_name_get (param i32 i32 i32) (result i32)))
  (import "arkade" "host_recent_name_at" (func $host_recent_name_at (param i32 i32 i32 i32) (result i32)))
  (import "arkade" "host_recent_name_note" (func $host_recent_name_note (param i32 i32 i32)))
  (import "arkade" "host_push_player_strategy" (func $host_push_player_strategy (param i32)))
  (import "arkade" "host_pop_player_strategy" (func $host_pop_player_strategy))
  (import "arkade" "host_lock_player_strategy" (func $host_lock_player_strategy))
  (import "arkade" "host_unlock_player_strategy" (func $host_unlock_player_strategy))
  (import "arkade" "host_switch_player" (func $host_switch_player (param i32)))

  ;; --- minimal WASI stub the host provides for toolchain-incidental
  ;; imports (see runtime.go); exercised here to prove the redirect to
  ;; deterministic host time/random, and the fd_write -> host_log bridge.
  (import "wasi_snapshot_preview1" "fd_write" (func $fd_write (param i32 i32 i32 i32) (result i32)))
  (import "wasi_snapshot_preview1" "proc_exit" (func $proc_exit (param i32)))
  (import "wasi_snapshot_preview1" "random_get" (func $random_get (param i32 i32) (result i32)))
  (import "wasi_snapshot_preview1" "clock_time_get" (func $clock_time_get (param i32 i64 i32) (result i32)))

  (memory (export "memory") 1)

  (data (i32.const 0) "last_input")
  (data (i32.const 16) "now_micros")
  (data (i32.const 32) "random_u64")
  (data (i32.const 48) "wasi_now_ns")
  (data (i32.const 64) "wasi_rand")
  (data (i32.const 80) "dt_micros")
  (data (i32.const 96) "OK")
  (data (i32.const 112) "render ok")
  (data (i32.const 128) "teardown")
  (data (i32.const 144) "bye\0a")
  (data (i32.const 160) "PNGFAKE1")
  (data (i32.const 176) "assets/sound.wav")
  (data (i32.const 192) "\ff\00\00\ff\00\ff\00\ff\00\00\ff\ff\ff\ff\00\ff\00\ff\ff\ff\ff\00\ff\ff\11\"3\ffDUf\ffw\88\99\ff\aa\bb\cc\ff\dd\ee\ff\ff\10 0\ff@P`\ffp\80\90\ff\a0\b0\c0\ff\d0\e0\f0\ff")
  (data (i32.const 256) "init ok")
  (data (i32.const 272) "{\"abi\":1,\"slug\":\"fixture\",\"name\":\"Fixture\",\"version\":\"1.0.0\",\"players\":[1,2],\"icon\":\"assets/sprites/icon.png\"}")

  ;; Name submitted to the score table / recent-name memory below.
  (data (i32.const 704) "ABC")

  ;; Storage keys the host_recent_name_at walk echoes its results under.
  (data (i32.const 880) "mru_name")
  (data (i32.const 888) "mru_past")

  ;; A sound asset file's bundle path — the same path space a plain .wav
  ;; uses, which is the entire point (docs/ASSETS.md: "Sprites and sounds
  ;; need no listing: the cartridge asks for them by path").
  (data (i32.const 992) "assets/blip.arkadesound.wav")

  ;; cart_abi_version() -> u32 — this document = 1.
  (func (export "cart_abi_version") (result i32)
    (i32.const 1))

  ;; cart_manifest() -> (ptr, len) — UTF-8 JSON at offset 272.
  (func (export "cart_manifest") (result i32 i32)
    (i32.const 272)
    (i32.const 110))

  ;; cart_alloc(size) -> ptr — always hands back the same 256-byte static
  ;; buffer at offset 384 (permitted by the doc: "cartridges may return a
  ;; reused static buffer").
  (func (export "cart_alloc") (param $size i32) (result i32)
    (i32.const 384))

  ;; cart_init(cols, rows) — one-time setup; here just proves host_log works
  ;; this early (host has already validated the ABI version by this point).
  (func (export "cart_init") (param $cols i32) (param $rows i32)
    (call $host_log (i32.const 0) (i32.const 256) (i32.const 7)))

  ;; cart_handle_input(ptr, len) — echoes the whole packed record buffer
  ;; verbatim into storage so the test can decode it back and assert the
  ;; host packed what the cartridge expects.
  (func (export "cart_handle_input") (param $ptr i32) (param $len i32)
    (call $host_storage_write
      (i32.const 0) (i32.const 10)
      (local.get $ptr) (local.get $len)))

  ;; cart_update(dt_micros) — echoes host_now_micros/host_random_u64 and
  ;; their WASI-stub twins (clock_time_get/random_get) into storage. A test
  ;; scripting a FakeHost's Now/Random sequence can read these back and
  ;; assert both the "arkade" import and the WASI stub route to the exact
  ;; same deterministic source.
  (func (export "cart_update") (param $dt i64)
    (i64.store (i32.const 640) (call $host_now_micros))
    (call $host_storage_write (i32.const 16) (i32.const 10) (i32.const 640) (i32.const 8))

    (i64.store (i32.const 648) (call $host_random_u64))
    (call $host_storage_write (i32.const 32) (i32.const 10) (i32.const 648) (i32.const 8))

    (drop (call $random_get (i32.const 656) (i32.const 8)))
    (call $host_storage_write (i32.const 64) (i32.const 9) (i32.const 656) (i32.const 8))

    (drop (call $clock_time_get (i32.const 0) (i64.const 1) (i32.const 664)))
    (call $host_storage_write (i32.const 48) (i32.const 11) (i32.const 664) (i32.const 8))

    (i64.store (i32.const 672) (local.get $dt))
    (call $host_storage_write (i32.const 80) (i32.const 9) (i32.const 672) (i32.const 8)))

  ;; cart_v2_surface() — calls every score/recent-name/strategy import once. This is a LINKAGE
  ;; proof, not realistic usage: a real game pushes a strategy in cart_init
  ;; and submits a score at game over, not both in one breath. What it pins
  ;; is that every one of these imports resolves and executes, so a host that
  ;; forgets to register one fails here loudly instead of in a game.
  (func $cart_v2_surface
    (local $n i32)
    (call $host_reject_key)

    ;; strategy: the real lifecycle a game follows — push, lock on play,
    ;; switch turns, unlock, pop.
    (call $host_push_player_strategy (i32.const 1)) ;; one-player solo
    (call $host_lock_player_strategy)
    (call $host_switch_player (i32.const 0))        ;; toggle
    (call $host_unlock_player_strategy)
    (call $host_pop_player_strategy)

    ;; scores: qualify -> rank -> submit -> read back -> count.
    (drop (call $host_score_qualifies (i64.const 100)))
    (drop (call $host_score_rank (i64.const 100)))
    (drop (call $host_score_submit (i64.const 100) (i32.const 704) (i32.const 3)))
    (drop (call $host_score_read (i32.const 1) (i32.const 720) (i32.const 72)))
    (drop (call $host_score_count))

    ;; recent names: note this seat's name, then read it back — first as
    ;; the 1h PREFILL (host_recent_name_get), then as entry 0 of that seat's
    ;; non-expiring MRU (host_recent_name_at). The name walked back is
    ;; echoed under "mru_name"; a read past the end of a one-entry list is
    ;; echoed under "mru_past" and must be zero bytes, which is how a guest
    ;; knows to stop walking.
    (call $host_recent_name_note (i32.const 0) (i32.const 704) (i32.const 3))
    (drop (call $host_recent_name_get (i32.const 0) (i32.const 800) (i32.const 64)))
    (local.set $n (call $host_recent_name_at (i32.const 0) (i32.const 0) (i32.const 896) (i32.const 64)))
    (call $host_storage_write (i32.const 880) (i32.const 8) (i32.const 896) (local.get $n))
    (i32.store (i32.const 976) (call $host_recent_name_at (i32.const 0) (i32.const 4) (i32.const 896) (i32.const 64)))
    (call $host_storage_write (i32.const 888) (i32.const 8) (i32.const 976) (i32.const 4)))

  ;; cart_render() — issues one of every draw call plus host_blit, proving
  ;; the whole "arkade" display/audio surface links and executes.
  (func (export "cart_render")
    (local $snd i32)
    (call $host_draw_pixel (i32.const 1) (i32.const 1) (i32.const 0xFF0000FF))
    (call $host_draw_pixel (i32.const 2) (i32.const 2) (i32.const 0x00FF00FF))
    (call $host_fill_rect (i32.const 0) (i32.const 3) (i32.const 4) (i32.const 1) (i32.const 0x0000FFFF))
    (call $cart_v2_surface)
    (call $host_draw_sprite
      (call $host_load_sprite (i32.const 160) (i32.const 8))
      (i32.const 0) (i32.const 0))
    (local.set $snd (call $host_load_sound (i32.const 176) (i32.const 16)))
    (call $host_play_sound (local.get $snd) (i32.const 0))
    (call $host_stop_sound (local.get $snd))
    (call $host_stop_all_sounds)
    ;; ...and the same two calls again for a .arkadesound.wav. Nothing
    ;; about the import changes: a path in, a handle out, the handle
    ;; played. The envelope is the host's business and the host does not
    ;; read it either.
    (local.set $snd (call $host_load_sound (i32.const 992) (i32.const 27)))
    (call $host_play_sound (local.get $snd) (i32.const 0))
    (call $host_blit (i32.const 192) (i32.const 64))
    (call $host_log (i32.const 1) (i32.const 112) (i32.const 9)))

  ;; cart_teardown() — logs, then exercises fd_write (stdout) once so the
  ;; test can assert the WASI stub forwards it to host_log.
  (func (export "cart_teardown")
    (call $host_log (i32.const 1) (i32.const 128) (i32.const 8))
    (i32.store (i32.const 688) (i32.const 144))
    (i32.store (i32.const 692) (i32.const 4))
    (drop (call $fd_write (i32.const 1) (i32.const 688) (i32.const 1) (i32.const 680)))))
