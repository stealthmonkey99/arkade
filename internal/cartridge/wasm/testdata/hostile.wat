;; hostile.wat — a cartridge that passes malformed pointers/lengths to
;; every "arkade" host import that takes a (ptr, len) or handle argument.
;; ABI version is valid so Init succeeds and the abuse happens inside
;; cart_update/cart_render/cart_alloc, exactly where a hot loop would feel
;; it. None of this may panic or read/write outside guest memory on the
;; host side — host.go's bounds checks must turn every one of these into a
;; clean no-op (or a clean Go error surfaced from Cartridge.DispatchInput
;; for the cart_alloc case).
;;
;; The score, recent-name and strategy imports get the same treatment (see
;; $abuse_v2): the score and recent-name imports take (ptr, len) names and are
;; a place a hostile cart could reach for host memory, and the strategy
;; imports take enums that must clamp rather than index blindly.
;;
;; go:generate wat2wasm hostile.wat -o hostile.wasm
(module
  (import "arkade" "host_blit" (func $host_blit (param i32 i32)))
  (import "arkade" "host_load_sprite" (func $host_load_sprite (param i32 i32) (result i32)))
  (import "arkade" "host_load_sound" (func $host_load_sound (param i32 i32) (result i32)))
  (import "arkade" "host_play_sound" (func $host_play_sound (param i32 i32)))
  (import "arkade" "host_storage_read" (func $host_storage_read (param i32 i32 i32 i32) (result i32)))
  (import "arkade" "host_storage_write" (func $host_storage_write (param i32 i32 i32 i32)))
  (import "arkade" "host_log" (func $host_log (param i32 i32 i32)))
  (import "arkade" "host_now_micros" (func $host_now_micros (result i64)))
  (import "arkade" "host_random_u64" (func $host_random_u64 (result i64)))

  ;; --- scores, recent names, strategy ---
  (import "arkade" "host_score_submit" (func $host_score_submit (param i64 i32 i32) (result i32)))
  (import "arkade" "host_score_read" (func $host_score_read (param i32 i32 i32) (result i32)))
  (import "arkade" "host_score_qualifies" (func $host_score_qualifies (param i64) (result i32)))
  (import "arkade" "host_score_rank" (func $host_score_rank (param i64) (result i32)))
  (import "arkade" "host_recent_name_get" (func $host_recent_name_get (param i32 i32 i32) (result i32)))
  (import "arkade" "host_recent_name_at" (func $host_recent_name_at (param i32 i32 i32 i32) (result i32)))
  (import "arkade" "host_recent_name_note" (func $host_recent_name_note (param i32 i32 i32)))
  (import "arkade" "host_push_player_strategy" (func $host_push_player_strategy (param i32)))
  (import "arkade" "host_pop_player_strategy" (func $host_pop_player_strategy))
  (import "arkade" "host_switch_player" (func $host_switch_player (param i32)))

  ;; A single page (65536 bytes) — every "OOB" pointer/length below is
  ;; chosen to be miles past that.
  (memory (export "memory") 1)

  (func (export "cart_abi_version") (result i32)
    (i32.const 1))

  (func (export "cart_manifest") (result i32 i32)
    (i32.const 0)
    (i32.const 0))

  ;; Returns a pointer that is nowhere near valid guest memory, so the
  ;; host's DispatchInput write (used for cart_handle_input) must fail
  ;; cleanly rather than write OOB.
  (func (export "cart_alloc") (param $size i32) (result i32)
    (i32.const 0x7FFFFFF0))

  (func (export "cart_init") (param $cols i32) (param $rows i32))
  (func (export "cart_handle_input") (param $ptr i32) (param $len i32))

  (func (export "cart_update") (param $dt i64)
    (drop (call $host_now_micros))
    (drop (call $host_random_u64)))

  ;; abuse_v2 attacks every score/recent-name/strategy import that takes guest memory or an
  ;; enum. The scores imports are the sharpest new edge: they are the only
  ;; of these calls that WRITE into guest memory (host_score_read,
  ;; host_recent_name_get), so an unchecked outPtr there would be a host-side
  ;; OOB write, not merely a bad read.
  (func $abuse_v2
    ;; host_score_submit: OOB name pointer/length, and a score far past the
    ;; host's cap (must be rejected, not inserted).
    (drop (call $host_score_submit (i64.const 0x7FFFFFFFFFFFFFFF) (i32.const 0xFFFFFFF0) (i32.const 0xFFFFFFFF)))
    ;; ...and a NEGATIVE score, which must not qualify or place.
    (drop (call $host_score_submit (i64.const -1) (i32.const 0) (i32.const 0)))
    ;; host_score_read: OOB output buffer, and a rank past the end of any table.
    (drop (call $host_score_read (i32.const 0xFFFFFFFF) (i32.const 0xFFFFFFF0) (i32.const 0xFFFFFFFF)))
    ;; host_score_qualifies/rank: absurd scores must clamp, not overflow.
    (drop (call $host_score_qualifies (i64.const -9223372036854775808)))
    (drop (call $host_score_rank (i64.const 0x7FFFFFFFFFFFFFFF)))
    ;; host_recent_name_get: a seat that does not exist, plus an OOB buffer.
    (drop (call $host_recent_name_get (i32.const 99) (i32.const 0xFFFFFFF0) (i32.const 0xFFFFFFFF)))
    ;; host_recent_name_at: a seat that does not exist, an index far past
    ;; any list, and an OOB buffer — all three must return 0, not read or
    ;; write host memory.
    (drop (call $host_recent_name_at (i32.const 99) (i32.const 0xFFFFFFFF) (i32.const 0xFFFFFFF0) (i32.const 0xFFFFFFFF)))
    (drop (call $host_recent_name_at (i32.const -1) (i32.const 0) (i32.const 0) (i32.const 0)))
    ;; host_recent_name_note: negative seat, OOB name.
    (call $host_recent_name_note (i32.const -5) (i32.const 0xFFFFFFF0) (i32.const 0xFFFFFFFF))
    ;; host_push_player_strategy: an unknown kind must be refused, not used
    ;; to index a table.
    (call $host_push_player_strategy (i32.const 0xFFFFFFFF))
    ;; host_pop_player_strategy: popping with nothing pushed must not pop the
    ;; engine's own base strategy out from under the shell.
    (call $host_pop_player_strategy)
    (call $host_pop_player_strategy)
    (call $host_pop_player_strategy)
    ;; host_switch_player: unknown mode.
    (call $host_switch_player (i32.const 77)))

  (func (export "cart_render")
    ;; host_blit: absurd pointer AND absurd length.
    (call $host_blit (i32.const 0xFFFFFFF0) (i32.const 0xFFFFFFFF))
    (call $abuse_v2)
    ;; host_load_sprite: OOB pointer, huge length far past the sandbox cap.
    (drop (call $host_load_sprite (i32.const 0xFFFFFFF0) (i32.const 0x7FFFFFFF)))
    ;; host_load_sound: same abuse, plus a length that's in-bounds-ish but
    ;; still absurdly over any sane path-length cap.
    (drop (call $host_load_sound (i32.const 0) (i32.const 0x00FFFFFF)))
    ;; host_play_sound: a handle that was never returned by host_load_sound.
    (call $host_play_sound (i32.const 0xDEADBEEF) (i32.const 0))
    ;; host_storage_read: valid (zeroed) key but an OOB output buffer.
    (drop (call $host_storage_read (i32.const 0) (i32.const 4) (i32.const 0xFFFFFFF0) (i32.const 0xFFFFFFFF)))
    ;; host_storage_write: OOB key pointer/length.
    (call $host_storage_write (i32.const 0xFFFFFFF0) (i32.const 0xFFFFFFFF) (i32.const 0) (i32.const 0))
    ;; host_log: an out-of-range level enum plus an OOB message.
    (call $host_log (i32.const 99) (i32.const 0xFFFFFFF0) (i32.const 0xFFFFFFFF)))

  (func (export "cart_teardown")))
