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

	ebiten.SetWindowSize(game.ScreenWidth, game.ScreenHeight)
	ebiten.SetWindowTitle("Pig Dice — Multiplayer")

	// RunGame blocks until the game exits (never in practice for the web build).
	if err := ebiten.RunGame(g); err != nil {
		js.Global().Get("console").Call("error", "ebiten RunGame:", err.Error())
	}
}
