;; lights.wat — a fixture for lamp sequences (docs/ASSETS.md,
;; "Lamp sequences"; sdk/docs/reference-lights.md) and nothing else.
;;
;; It is its own fixture rather than more calls bolted onto good.wat because
;; what it has to prove is a CONVERSATION with the host over several ticks —
;; play, let the clock run, take the lamps back by hand — and good.wat is
;; deliberately a cartridge that does one well-behaved thing per export.
;;
;; The guest is a remote control, not a decider: every branch of the test is
;; driven from Go by writing a command byte into guest memory through
;; cart_handle_input, so one fixture serves every case and the .wat stays
;; readable.
;;
;; Commands ride in the KEY byte of one input record — byte 1 of the packed
;; 8-byte record (byte 0 is the kind), so the test drives everything with an
;; ordinary DispatchInput and no special packing:
;;   1  play "assets/lights/win.arkadelights.json" on both seats, file's mode
;;   2  play the same sequence on seat 0 only, forced to cycle
;;   3  play a path the manifest never listed (must return 0)
;;   4  stop both seats
;;   5  host_set_button_lights(p1, joya only, only=true) — the explicit call
;;   6  host_set_lights(p1, off) — the other explicit call
;;   7  host_set_lamp_levels(p1, levels 1..6 with junk in bits 24..31, all
;;      six lamps touched, 250 ms fade)
;;   8  host_set_lamp_levels touching nothing — a complete no-op, which
;;      notably does NOT stop a sequence
;;   9  host_set_lamp_levels on seat 2 — out of range, warns and ignores
;;  10  host_set_lamp_levels(both seats, all lamps full, a 99999 ms fade the
;;      host clamps)
;;  11  host_set_lamp_levels(p1, joya alone, no fade) — the explicit call
;;      wins, and only on the seat it addresses
;;
;; Every host_lights_play result is summed into a counter echoed under the
;; storage key "played", so the test pins the RETURN semantics and not just
;; the resulting lamps.
;;
;; See generate.go for the wat2wasm invocation; lights.wasm is checked in
;; alongside so `go test` never needs wabt.
(module
  (import "arkade" "host_lights_play"
    (func $play (param i32 i32 i32 i32) (result i32)))
  (import "arkade" "host_lights_stop"
    (func $stop (param i32)))
  (import "arkade" "host_set_button_lights"
    (func $set_lamps (param i32 i32 i32)))
  (import "arkade" "host_set_lights"
    (func $set_all (param i32 i32)))
  (import "arkade" "host_set_lamp_levels"
    (func $set_levels (param i32 i32 i32 i32)))
  (import "arkade" "host_storage_write"
    (func $storage_write (param i32 i32 i32 i32)))

  (memory (export "memory") 1)

  ;; 0: the storage key the play-result sum is echoed under.
  (data (i32.const 0) "played")
  ;; 16: cart_manifest's JSON.
  (data (i32.const 16) "{\"abi\":1,\"slug\":\"lightsfixture\",\"name\":\"Lights Fixture\",\"version\":\"1.0.0\",\"players\":[1],\"icon\":\"assets/icon.png\"}")
  ;; 160: the sequence path the manifest lists (35 bytes).
  (data (i32.const 160) "assets/lights/win.arkadelights.json")
  ;; 224: a path the manifest does NOT list (36 bytes).
  (data (i32.const 224) "assets/lights/nope.arkadelights.json")
  ;; 256: cart_alloc's reused static buffer — where the command byte lands.
  ;; 512: 4-byte scratch the play-result sum is written into.

  (global $played (mut i32) (i32.const 0))

  (func (export "cart_abi_version") (result i32)
    (i32.const 1))

  (func (export "cart_manifest") (result i32 i32)
    (i32.const 16)
    (i32.const 113))

  (func (export "cart_alloc") (param $size i32) (result i32)
    (i32.const 256))

  (func (export "cart_init") (param $cols i32) (param $rows i32))

  (func (export "cart_handle_input") (param $ptr i32) (param $len i32)
    (local $cmd i32)
    (local.set $cmd (i32.load8_u offset=1 (local.get $ptr)))

    ;; 1: both seats, the file's own loop mode.
    (if (i32.eq (local.get $cmd) (i32.const 1)) (then
      (global.set $played (i32.add (global.get $played)
        (call $play (i32.const 160) (i32.const 35) (i32.const -1) (i32.const 0))))))

    ;; 2: seat 0 only, forced to cycle.
    (if (i32.eq (local.get $cmd) (i32.const 2)) (then
      (global.set $played (i32.add (global.get $played)
        (call $play (i32.const 160) (i32.const 35) (i32.const 0) (i32.const 2))))))

    ;; 3: an unlisted path — the host warns and returns 0.
    (if (i32.eq (local.get $cmd) (i32.const 3)) (then
      (global.set $played (i32.add (global.get $played)
        (call $play (i32.const 224) (i32.const 36) (i32.const -1) (i32.const 0))))))

    ;; 4: stop both seats. The lamps stay as the sequence left them.
    (if (i32.eq (local.get $cmd) (i32.const 4)) (then
      (call $stop (i32.const -1))))

    ;; 5: the explicit call — light joya on seat 0 and nothing else. This
    ;; must also STOP any sequence running on that seat.
    (if (i32.eq (local.get $cmd) (i32.const 5)) (then
      (call $set_lamps (i32.const 0) (i32.const 1) (i32.const 1))))

    ;; 6: the other explicit call, all six lamps off on seat 0.
    (if (i32.eq (local.get $cmd) (i32.const 6)) (then
      (call $set_all (i32.const 0) (i32.const 0))))

    ;; 7: levels 1..6, one per lamp, with junk in bits 24..31 the host must
    ;; ignore; all six lamps touched, fading over 250 ms.
    (if (i32.eq (local.get $cmd) (i32.const 7)) (then
      (call $set_levels (i32.const 0) (i32.const 0xff654321)
        (i32.const 0x3f) (i32.const 250))))

    ;; 8: touching nothing. No write, no record, and no sequence stop.
    (if (i32.eq (local.get $cmd) (i32.const 8)) (then
      (call $set_levels (i32.const 0) (i32.const 0x00ffffff)
        (i32.const 0) (i32.const 100))))

    ;; 9: seat 2 — out of range, so nothing happens but a warning.
    (if (i32.eq (local.get $cmd) (i32.const 9)) (then
      (call $set_levels (i32.const 2) (i32.const 0x00ffffff)
        (i32.const 0x3f) (i32.const 0))))

    ;; 10: both seats to full over a fade far past the ceiling.
    (if (i32.eq (local.get $cmd) (i32.const 10)) (then
      (call $set_levels (i32.const -1) (i32.const 0x00ffffff)
        (i32.const 0x3f) (i32.const 99999))))

    ;; 11: seat 0's joya alone, no fade — the explicit call wins on that
    ;; seat and leaves the other seat's sequence running.
    (if (i32.eq (local.get $cmd) (i32.const 11)) (then
      (call $set_levels (i32.const 0) (i32.const 0x0000000f)
        (i32.const 0x01) (i32.const 0))))

    (i32.store (i32.const 512) (global.get $played))
    (call $storage_write (i32.const 0) (i32.const 6) (i32.const 512) (i32.const 4)))

  (func (export "cart_update") (param $dt i64))
  (func (export "cart_render"))
  (func (export "cart_teardown"))
)
