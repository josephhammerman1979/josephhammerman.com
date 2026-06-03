//go:build js && wasm

package main

import (
	"syscall/js"

	game "dicegames/dicegame"

	"github.com/hajimehoshi/ebiten/v2"
)

func main() {
	// Read configuration injected by the host page via window.diceGameConfig.
	config := js.Global().Get("diceGameConfig")
	numPlayers := config.Get("numPlayers").Int()
	myPlayerIdx := config.Get("myPlayerIdx").Int()

	// Create the game; wire the send callback through to JS.
	g := game.NewMultiplayerPigGame(numPlayers, myPlayerIdx, func(jsonStr string) {
		// window.diceGameSendEvent is set by dice_game.js before WASM starts.
		fn := js.Global().Get("diceGameSendEvent")
		if fn.Truthy() {
			fn.Invoke(jsonStr)
		}
	})

	// Expose window.diceGameReceiveEvent so dice_game.js can deliver peer events.
	js.Global().Set("diceGameReceiveEvent", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			g.ReceiveEvent(args[0].String())
		}
		return nil
	}))

	// Expose window.diceGameRoll so dice_game.js can request a roll from
	// non-keyboard input (e.g. a phone shake gesture or on-screen button).
	js.Global().Set("diceGameRoll", js.FuncOf(func(_ js.Value, _ []js.Value) any {
		g.RequestRoll()
		return nil
	}))

	// Expose window.diceGameHold for the on-screen Hold button.
	js.Global().Set("diceGameHold", js.FuncOf(func(_ js.Value, _ []js.Value) any {
		g.RequestHold()
		return nil
	}))

	// Expose window.diceGameKick so the host's player_kick broadcast can
	// apply the kick locally in every peer's WASM instance.
	js.Global().Set("diceGameKick", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			g.Kick(args[0].Int())
		}
		return nil
	}))

	// Bridge turn changes back to JS so the host page can enable/disable
	// the Roll/Hold buttons and drive the mobile fullscreen takeover.
	g.SetTurnChangeFn(func(currentIdx int) {
		fn := js.Global().Get("diceGameOnTurnChange")
		if fn.Truthy() {
			fn.Invoke(currentIdx)
		}
	})

	// Bridge game-over so the host page can show a "New Game" button.
	g.SetGameOverFn(func(winnerIdx int) {
		fn := js.Global().Get("diceGameOnGameOver")
		if fn.Truthy() {
			fn.Invoke(winnerIdx)
		}
	})

	// Expose window.diceGameReset(numPlayers, myPlayerIdx) so a "New Game"
	// click in JS restarts the game without reloading the WASM module.
	js.Global().Set("diceGameReset", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) >= 2 {
			g.Reset(args[0].Int(), args[1].Int())
		}
		return nil
	}))

	ebiten.SetWindowSize(game.ScreenWidth, game.ScreenHeight)
	ebiten.SetWindowTitle("Pig Dice — Multiplayer")

	// RunGame blocks until the game exits (never in practice for the web build).
	if err := ebiten.RunGame(g); err != nil {
		js.Global().Get("console").Call("error", "ebiten RunGame:", err.Error())
	}
}
