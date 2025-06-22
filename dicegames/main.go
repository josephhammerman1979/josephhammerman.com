package main

import (
	"fmt"
	"log"

	"github.com/hajimehoshi/ebiten/v2"

	"dicegames/dicegame"
)

func main() {
	// Get player count from user
	numPlayers := promptPlayerCount()
	if numPlayers < 2 || numPlayers > 8 {
		log.Fatal("Invalid player count. Must be 2-8 players.")
	}

	// Initialize game
	app := dicegame.NewPigGame(numPlayers)

	// Configure window
	ebiten.SetWindowSize(dicegame.ScreenWidth, dicegame.ScreenHeight)
	ebiten.SetWindowTitle("Pig Dice Game")

	// Start game loop
	if err := ebiten.RunGame(app); err != nil {
		log.Fatal(err)
	}
}

func promptPlayerCount() int {
	fmt.Println("Welcome to Pig Dice Game!")
	fmt.Print("Enter number of players (2-8): ")

	var count int
	_, err := fmt.Scan(&count)
	if err != nil {
		log.Fatal("Invalid input. Please enter a number.")
	}
	return count
}
