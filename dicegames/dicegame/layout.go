package game

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/text"
)

// DrawPlayerScores renders the player list in the left section.
// Returns the vertical space used (bottom Y).
func DrawPlayerScores(screen *ebiten.Image, players []*Player, currentIndex int, originX, originY int) int {
	y := originY
	lineHeight := 40

	for i, player := range players {
		status := ""
		if i == currentIndex {
			status = " (Current)"
		}

		scoreText := fmt.Sprintf("Player %d: %d pts (Turn: %d)%s",
			player.ID, player.TotalScore, player.TurnScore, status)

		text.Draw(screen, scoreText, RegularFont, originX, y, color.White)
		y += lineHeight
	}

	return y
}

// DrawPlayArea renders a background "area" on the right for the dice.
// You can customize how large the region should be.
func DrawPlayArea(screen *ebiten.Image, originX, width, screenHeight int) {
	// Background color
	bg := color.RGBA{20, 20, 40, 255}
	ebitenutil.DrawRect(screen, float64(originX), 0, float64(width), float64(screenHeight), bg)

	// Optional separator line
	lineColor := color.RGBA{80, 80, 80, 255}
	ebitenutil.DrawRect(screen, float64(originX), 0, 2, float64(screenHeight), lineColor)
}
