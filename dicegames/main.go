package main

import (
	"log"

	game "dicegames/dicegame"

	"github.com/hajimehoshi/ebiten/v2"
)

func main() {
	gameManager := game.NewGameManager()

	ebiten.SetWindowSize(800, 600)
	ebiten.SetWindowTitle("Dice Games")

	if err := ebiten.RunGame(gameManager); err != nil {
		log.Fatal(err)
	}
}
