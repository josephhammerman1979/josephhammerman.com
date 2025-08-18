package game

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text"
)

type MockGame struct {
	Pool      *DicePool
	DiceCount int
	GameState string
	Message   string
}

func NewMockGame(diceCount int) *MockGame {
	// Cup in bottom middle of play area
	//cupX := float64(ScorePanelWidth) + (float64(ScreenWidth-ScorePanelWidth) / 2)
	///cupY := float64(ScreenHeight) - 60

	cupX := float64(ScorePanelWidth) + 40 // Left edge (with some margin)
	cupY := float64(ScreenHeight) - 60    // Still near bottom

	pool := NewDicePool(
		diceCount,
		cupX, cupY,
		float64(ScorePanelWidth)+10, 60,
		float64(ScreenWidth)-10, float64(ScreenHeight)-10,
	)

	return &MockGame{
		Pool:      pool,
		DiceCount: diceCount,
		GameState: "waiting",
		Message:   "Press SPACE to roll the dice.",
	}
}

func (g *MockGame) Update() error {
	switch g.GameState {
	case "waiting":
		if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
			g.Pool.StartRoll()
			g.GameState = "rolling"
			g.Message = "Rolling..."
		}
	case "rolling":
		g.Pool.Update()
		if !g.Pool.Rolling {
			g.GameState = "results"
			g.Message = "Click a die to keep/reroll. ENTER to reroll unkept. SPACE for new roll."
		}
	case "results":
		mx, my := ebiten.CursorPosition()
		// Get displayed positions of all dice (same as in PresentFinalResults)
		positions := makeDiceInRow(
			len(g.Pool.Dice),
			g.Pool.PlayMinX, g.Pool.PlayMinY+40,
			g.Pool.PlayMaxX, g.Pool.PlayMinY+g.Pool.DiceSize*2+40,
			g.Pool.DiceSize*1.2,
		)
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			for i := range g.Pool.Dice {
				x, y := positions[i][0], positions[i][1]
				size := int(g.Pool.DiceSize * 1.2)
				// Don't shift y for selected dice in hit-test—just use actual drawn position from PresentFinalResults!
				if mx >= int(x) && mx <= int(x)+size && my >= int(y) && my <= int(y)+size {
					g.Pool.ToggleKeep(i)
				}
			}
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
			g.Pool.RerollUnkeptFromCup()
			g.GameState = "rolling"
			g.Message = "Rerolling unkept dice..."
		}
		if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
			g.Pool.ResetAll()
			g.Pool.StartRoll()
			g.GameState = "rolling"
			g.Message = "Rolling..."
		}
	}
	return nil
}

func (g *MockGame) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{30, 30, 50, 255})
	ebitenutil.DrawRect(screen, float64(ScorePanelWidth), 0,
		float64(ScreenWidth-ScorePanelWidth), float64(ScreenHeight),
		color.RGBA{20, 20, 40, 255})

	if g.GameState == "rolling" {
		// Draw dice at their live animated positions (throw/bounce phase)
		for _, d := range g.Pool.Dice {
			d.Draw(screen, int(d.X), int(d.Y), int(d.Size))
			if d.Selected {
				drawRedOutline(screen, int(d.X), int(d.Y), int(d.Size))
			}
		}
	} else {
		// Now present results in a neat row (results/nongameplay phase)
		g.Pool.PresentFinalResults(screen)
	}

	text.Draw(screen, g.Message, RegularFont, ScorePanelWidth+20, 30, color.White)
}

func (g *MockGame) Layout(_, _ int) (int, int) {
	return ScreenWidth, ScreenHeight
}
