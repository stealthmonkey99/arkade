// SPDX-License-Identifier: GPL-2.0-or-later

package cartlib

// Game is the interface a cartridge implements and registers with [Run];
// cartlib's cart_* exports delegate to it.
//
// The host calls [Game.Init] once, then HandleInput, Update and Render in its
// frame loop, then [Teardowner.Teardown] if the Game is a [Teardowner]. Manifest may be called
// at any point, before or after Init. All methods run on one thread; there
// are no goroutines.
type Game interface {
	// Init runs once, from cart_init, and is the first point at which
	// computed setup code runs: allocate everything the game will need
	// here, and load assets, bind channels and define glyphs through s,
	// the [Setup] that is live only until Init returns. The canvas is
	// always [FBWidth] by [FBHeight].
	Init(s *Setup)

	// HandleInput receives the tick's decoded input records
	// (docs/CARTRIDGE.md, "Input records"). Act on presses
	// ([InputRecord.IsPress]): a physical press arrives as a keydown and a
	// keypress, so acting on keydowns as well double-fires. Unless the Game
	// is a [QuitOwner], any quit-key press in the batch has already
	// requested [Exit]. records is reused on the next call; copy what must
	// outlive it.
	HandleInput(records []InputRecord)

	// Update advances the simulation by dtMicros, the microseconds since
	// the previous Update. dtMicros, or [NowMicros] on the same engine
	// clock, is the only source of time and [RandomU64] the only source of
	// randomness; a wall clock or OS RNG breaks replay determinism.
	Update(dtMicros uint64)

	// Render draws one frame into fb, which cartlib sends to the host when
	// Render returns; nothing else reaches the screen. fb keeps the previous
	// frame's pixels. Render may run before the first Update, so it must
	// draw a valid frame from the state Init left.
	Render(fb *Framebuffer)

	// Manifest returns the cart_manifest JSON (docs/CARTRIDGE.md,
	// "Manifest"): manifestBytes, the package-level slice manifest_gen.go
	// declares from a string constant. The SDK's manifestgen writes that
	// file from manifest.json on every build. Anything else, built at call
	// time or from runtime state, fails the post-build wasmfix pass:
	//
	//	func (g *myGame) Manifest() []byte { return manifestBytes }
	Manifest() []byte
}

// Teardowner is an optional [Game] extension for a Game with state to flush
// before the cartridge unloads. Without it cart_teardown does nothing.
type Teardowner interface {
	// Teardown runs from cart_teardown, just before the cartridge unloads.
	Teardown()
}

// QuitOwner is an optional [Game] extension for a cartridge that runs its
// own quit flow, such as a ui.HoldConfirm overlay.
//
// By default cartlib calls [Exit] for any quit-key press in a batch
// ([InputRecord.IsPress] and [InputRecord.IsQuit]: KeyQuit, KeyEscape,
// KeyJoyB) before passing the batch to HandleInput. On the cabinet the
// cartridge owns those keys, so without this default it could not be left.
type QuitOwner interface {
	// OwnsQuitKeys reports whether the Game handles quit keys itself. It is
	// consulted once per input batch, before HandleInput. While it returns
	// true cartlib does not call [Exit] and the Game must; it may return
	// false on screens where an immediate quit is right, such as a title.
	OwnsQuitKeys() bool
}

func ownsQuitKeys(g Game) bool {
	o, ok := g.(QuitOwner)
	return ok && o.OwnsQuitKeys()
}

// Loader is an optional [Game] extension for a custom loading screen.
// During the load phase, after Init and before the first Update, cartlib
// calls LoadFrame in place of Render. Without Loader, cartlib draws the
// manifest "name" centred on black. Either way the load phase lasts at least
// [MinSplashMicros], which also gives the browser's asynchronous sound loads
// time to resolve.
type Loader interface {
	// LoadFrame draws one loading frame into fb and returns ready once the
	// game can play; the first Update follows when the load phase has also
	// lasted [MinSplashMicros]. elapsedMicros is the time spent loading so
	// far, the only clock during load. [Game.Update] does not run until the
	// phase ends.
	LoadFrame(elapsedMicros uint64, fb *Framebuffer) (ready bool)
}

// Attractor is an optional [Game] extension for the cabinet's attract mode.
// A Game opts in by implementing it and declaring the export:
//
//	//export cart_attract
//	func cartAttract(dt uint64) { cartlib.DispatchAttract(dt) }
//
// Without the export, attract mode shows the engine's default showcase.
type Attractor interface {
	// Attract runs each frame while attract mode shows this cartridge,
	// advancing by dtMicros and drawing into fb, which cartlib sends as it
	// does for Render. Pixels with alpha below 255 composite over the
	// engine's starfield (see [Framebuffer.ClearTransparent]). Attract may
	// set LCDs and button lights; if it leaves the lights alone they show
	// the default chase.
	Attract(dtMicros uint64, fb *Framebuffer)
}

// currentGame is a plain package var so Run's assignment folds into static
// data at compile time.
var currentGame Game

// Run registers game as the cartridge's implementation and returns 0, so it
// fits a `var _ =` statement. Call it exactly once, as a package-level
// initializer:
//
//	var game = &myGame{}
//	var _ = cartlib.Run(game)
//
//	func main() {}
//
// The hosts run _start once at instantiation, before any cart_* export, so
// the initializer runs then and the TinyGo compiler folds it into static
// data. main must stay empty: preflight refuses a non-empty main or an init
// function, and a Run called from main would run too late to be relied on.
// Computed setup belongs in [Game.Init].
func Run(game Game) int {
	currentGame = game
	return 0
}
