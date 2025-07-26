package game

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text"
)

type PlayerCount struct {
	count int
}

func NewPlayerCount() *PlayerCount {
	return &PlayerCount{count: 2}
}

func (pc *PlayerCount) Update(gm *GameManager) error {
	if inpututil.IsKeyJustPressed(ebiten.KeyUp) && pc.count < 8 {
		pc.count++
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyDown) && pc.count > 2 {
		pc.count--
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		gm.pigGame = NewPigGame(pc.count)
		gm.currentState = StatePigGame
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		gm.currentState = StateLauncher
	}
	return nil
}

func (pc *PlayerCount) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{30, 30, 50, 255})
	text.Draw(screen, "Select Player Count", RegularFont, 50, 50, color.White)
	text.Draw(screen, fmt.Sprintf("Players: %d", pc.count), RegularFont, 100, 150, color.White)
	text.Draw(screen, "↑/↓: Change  ENTER: Confirm  ESC: Back", RegularFont, 50, 500, color.White)
}
