package game

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text"
)

// Add games to this list as you implement them!
var GameMenuChoices = []string{
	"Pig Dice Game",
	"Mock Dice Demo", // For demonstration; assumes NewMockGame() exists.
}

type Launcher struct {
	games       []string
	selectedIdx int
}

func NewLauncher() *Launcher {
	return &Launcher{
		games: GameMenuChoices,
	}
}

func (l *Launcher) Update(gm *GameManager) error {
	if inpututil.IsKeyJustPressed(ebiten.KeyDown) {
		l.selectedIdx = (l.selectedIdx + 1) % len(l.games)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyUp) {
		l.selectedIdx = (l.selectedIdx - 1 + len(l.games)) % len(l.games)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		selectedGame := l.games[l.selectedIdx]
		switch selectedGame {
		case "Pig Dice Game":
			gm.selectedGame = "Pig Dice Game"
			gm.playerCount = NewPlayerCount() // After Done, will call NewPigGame in GameManager
			gm.currentState = StatePlayerCount
		case "Mock Dice Demo":
			gm.selectedGame = "Mock Dice Demo"
			gm.activeGame = NewMockGame(5) // Launch mock game with 5 dice
			gm.currentState = StateInGame
		}
	}
	return nil
}

func (l *Launcher) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{30, 30, 50, 255})
	text.Draw(screen, "Select a Game", RegularFont, 50, 50, color.White)

	for i, name := range l.games {
		var clr color.Color = color.White
		if i == l.selectedIdx {
			clr = color.RGBA{0, 255, 0, 255}
		}
		text.Draw(screen, name, RegularFont, 100, 150+i*50, clr)
	}
	text.Draw(screen, "↑/↓: Navigate  ENTER: Select", RegularFont, 50, 500, color.White)
}
