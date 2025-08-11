package game

import (
	"image/color"
	"strconv"

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
	cupX := float64(ScorePanelWidth) + (float64(ScreenWidth-ScorePanelWidth) / 2)
	cupY := float64(ScreenHeight) - 60

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
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			for i, d := range g.Pool.Dice {
				if mx >= int(d.X) && mx <= int(d.X+d.Size) && my >= int(d.Y) && my <= int(d.Y+d.Size) {
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

	for i, d := range g.Pool.Dice {
		y := d.Y
		if g.GameState == "results" && d.Selected {
			y = 70
		}
		d.Draw(screen, int(d.X), int(y), int(d.Size))

		var col color.Color = color.White
		if d.Selected {
			col = color.RGBA{220, 60, 60, 255}
			ebitenutil.DrawRect(screen, d.X-3, y-3, d.Size+6, d.Size+6, col)
		}
		numStr := strconv.Itoa(d.Value)
		text.Draw(screen, numStr, RegularFont, int(d.X+d.Size*0.4), int(y+d.Size+22), col)
		if g.DiceCount > 6 {
			text.Draw(screen, strconv.Itoa(i+1), RegularFont, int(d.X+d.Size/3), int(y)-9, color.White)
		}
	}

	text.Draw(screen, g.Message, RegularFont, ScorePanelWidth+20, 30, color.White)
}

func (g *MockGame) Layout(_, _ int) (int, int) {
	return ScreenWidth, ScreenHeight
}
