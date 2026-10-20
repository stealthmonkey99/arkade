;; flight.wat — the fixture cartridge for the INPUT FLIGHT RECORDER tests
;; (internal/engine/flight.go, cmd/arkade/replay.go, docs/FLIGHT_RECORDER.md).
;;
;; e2e.wat cannot do this job: it renders the same frame forever, so a
;; "replay produces identical frames" assertion against it would pass even
;; if the replayer fed the guest nothing at all. This fixture is built so
;; that its FRAMES depend on all three streams a recording carries, and
;; therefore so that dropping any one of them changes the pixels:
;;
;;   - INPUT: a RIGHT or DOWN keypress moves the sprite one column on, LEFT
;;     or UP one column back (wrapped into 0..31). All four directions,
;;     deliberately — see the note in cart_handle_input.
;;   - dt_micros: cart_update accumulates it, and the sprite's ROW is that
;;     accumulator divided down — so a replay at the wrong cadence draws in
;;     the wrong place.
;;   - host_random_u64: cart_init draws once and that value IS the sprite's
;;     colour. The engine RNG is a shared stream (see Runtime.CartRandomU64),
;;     so this is what proves the replayer serves the recorded draws back
;;     rather than re-seeding and hoping.
;;
;; It also carries the CRASH trigger's fixture behaviour: a CLEAR keypress
;; (key ordinal 27) arms $boom, and the next cart_update executes
;; `unreachable` — which is exactly what a TinyGo `-panic=trap` build does
;; on any runtime panic, and what wazero surfaces to the host as an error
;; from cart_update. One fixture rather than two because the trap is a
;; behaviour of this same cartridge, reached only by a key no test presses
;; by accident.
;;
;; Size-agnostic like e2e.wat: cart_init records the host's cols/rows and
;; nothing here assumes a particular panel (host_fill_rect is bounds-checked
;; host-side, and the sprite is confined to the first 32x32 regardless).
;;
;; Memory layout:
;;   0     len 81   manifest JSON       cart_manifest() return
;;   4096  len 256  (zeroed)            cart_alloc's reused static buffer
;;
;; go:generate wat2wasm flight.wat -o flight.wasm
(module
  (import "arkade" "host_fill_rect" (func $host_fill_rect (param i32 i32 i32 i32 i32)))
  (import "arkade" "host_random_u64" (func $host_random_u64 (result i64)))

  (memory (export "memory") 1)

  (data (i32.const 0) "{\"abi\":1,\"slug\":\"flight\",\"name\":\"Flight Fixture\",\"version\":\"1.0.0\",\"players\":[1]}")

  (global $cols (mut i32) (i32.const 0))
  (global $rows (mut i32) (i32.const 0))
  ;; $x is the sprite column, driven by input; $t the accumulated dt_micros,
  ;; whose quotient is the row; $c the colour drawn from the host RNG.
  (global $x (mut i32) (i32.const 8))
  (global $t (mut i64) (i64.const 0))
  (global $c (mut i32) (i32.const 0xFF00FF00))
  (global $boom (mut i32) (i32.const 0))

  (func (export "cart_abi_version") (result i32) (i32.const 1))

  (func (export "cart_manifest") (result i32 i32)
    (i32.const 0) (i32.const 81))

  (func (export "cart_alloc") (param $size i32) (result i32)
    (i32.const 4096))

  ;; cart_init draws ONE random value and keeps its low bits as an opaque
  ;; colour. One draw, at a known point, so a test can say exactly what the
  ;; replay had to reproduce.
  (func (export "cart_init") (param $cols i32) (param $rows i32)
    (global.set $cols (local.get $cols))
    (global.set $rows (local.get $rows))
    (global.set $c
      (i32.or
        (i32.and (i32.wrap_i64 (call $host_random_u64)) (i32.const 0x00FFFFFF))
        (i32.const 0xFF000000))))

  ;; cart_handle_input(ptr, len) — walks the packed 8-byte records
  ;; (docs/CARTRIDGE.md "Input records"): byte 0 is the kind, byte 1 the key
  ;; ordinal (see internal/cartridge/wasm/keys.go's keyOrdinal).
  (func (export "cart_handle_input") (param $ptr i32) (param $len i32)
    (local $i i32)
    (local $n i32)
    (local $rec i32)
    (local $key i32)
    (local.set $n (i32.div_u (local.get $len) (i32.const 8)))
    (block $done
      (loop $next
        (br_if $done (i32.ge_u (local.get $i) (local.get $n)))
        (local.set $rec
          (i32.add (local.get $ptr) (i32.mul (local.get $i) (i32.const 8))))
        ;; kind 2 == keypress
        (if (i32.eq (i32.load8_u (local.get $rec)) (i32.const 2))
          (then
            (local.set $key (i32.load8_u offset=1 (local.get $rec)))
            ;; 9 == right, 7 == down: forward one column.
            ;;
            ;; ALL FOUR directions move the sprite, not just left/right,
            ;; because the cabinet remaps a controller's stick by its seat
            ;; mount before anything sees it (internal/engine's remapEvent) —
            ;; a test pressing RIGHT at seat 0 hands this cartridge UP. A
            ;; fixture that only understood left/right would therefore be
            ;; INSENSITIVE to the recorded input records, and the replay
            ;; determinism test would pass while proving nothing.
            (if (i32.or
                  (i32.eq (local.get $key) (i32.const 9))
                  (i32.eq (local.get $key) (i32.const 7)))
              (then (global.set $x
                (i32.and (i32.add (global.get $x) (i32.const 1)) (i32.const 31)))))
            ;; 8 == left, 6 == up: back one column.
            (if (i32.or
                  (i32.eq (local.get $key) (i32.const 8))
                  (i32.eq (local.get $key) (i32.const 6)))
              (then (global.set $x
                (i32.and (i32.add (global.get $x) (i32.const 31)) (i32.const 31)))))
            ;; 27 == clear: arm the trap
            (if (i32.eq (local.get $key) (i32.const 27))
              (then (global.set $boom (i32.const 1))))))
        (local.set $i (i32.add (local.get $i) (i32.const 1)))
        (br $next))))

  (func (export "cart_update") (param $dt i64)
    (if (global.get $boom) (then (unreachable)))
    (global.set $t (i64.add (global.get $t) (local.get $dt))))

  (func (export "cart_render")
    (call $host_fill_rect
      (global.get $x)
      (i32.and (i32.wrap_i64 (i64.div_u (global.get $t) (i64.const 100000))) (i32.const 31))
      (i32.const 2) (i32.const 2)
      (global.get $c)))

  (func (export "cart_teardown")))
