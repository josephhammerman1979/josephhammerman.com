package game

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text"
)

type Launcher struct {
	games       []string
	selectedIdx int
}

func NewLauncher() *Launcher {
	return &Launcher{
		games: []string{"Pig Dice Game"},
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
			gm.playerCount = NewPlayerCount()
			gm.currentState = StatePlayerCount
		}
	}
	return nil
}

func (l *Launcher) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{30, 30, 50, 255})
	text.Draw(screen, "Select a Game", RegularFont, 50, 50, color.White)

	for i, name := range l.games {
		var clr color.Color
		if i == l.selectedIdx {
			clr = color.RGBA{0, 255, 0, 255}
		}
		text.Draw(screen, name, RegularFont, 100, 150+i*50, clr)
	}
	text.Draw(screen, "↑/↓: Navigate  ENTER: Select", RegularFont, 50, 500, color.White)
}
