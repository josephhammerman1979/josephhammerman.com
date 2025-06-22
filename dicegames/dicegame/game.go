// dicegame/game.go
package dicegame

import (
	"fmt"
	"image/color"
	"math/rand"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
)

const (
	ScreenWidth  = 800
	ScreenHeight = 600
	WinningScore = 100
)

var RegularFont font.Face

func init() {
	tt, _ := opentype.Parse(goregular.TTF)
	RegularFont, _ = opentype.NewFace(tt, &opentype.FaceOptions{
		Size:    24,
		DPI:     72,
		Hinting: font.HintingFull,
	})
}

type Player struct {
	ID         int
	TotalScore int
	TurnScore  int
	IsActive   bool
}

type PigGame struct {
	Players       []*Player
	CurrentIndex  int
	GameOver      bool
	WinnerID      int
	DieValue      int
	Message       string
	inputCooldown int
}

func NewPigGame(numPlayers int) *PigGame {
	players := make([]*Player, numPlayers)
	for i := range players {
		players[i] = &Player{ID: i + 1}
	}
	players[0].IsActive = true

	return &PigGame{
		Players:       players,
		CurrentIndex:  0,
		DieValue:      1,
		Message:       fmt.Sprintf("Player %d's turn - SPACE to roll", 1),
		inputCooldown: 0,
	}
}

func (g *PigGame) Roll() {
	rand.Seed(time.Now().UnixNano())
	g.DieValue = rand.Intn(6) + 1
	currentPlayer := g.Players[g.CurrentIndex]

	if g.DieValue == 1 {
		currentPlayer.TurnScore = 0
		g.Message = fmt.Sprintf("Player %d rolled 1! Turn lost", currentPlayer.ID)
		g.nextPlayer()
	} else {
		currentPlayer.TurnScore += g.DieValue
		g.Message = fmt.Sprintf("Player %d rolled %d (Turn: %d)",
			currentPlayer.ID, g.DieValue, currentPlayer.TurnScore)
	}
}

func (g *PigGame) Hold() {
	currentPlayer := g.Players[g.CurrentIndex]
	currentPlayer.TotalScore += currentPlayer.TurnScore
	currentPlayer.TurnScore = 0

	if currentPlayer.TotalScore >= WinningScore {
		g.GameOver = true
		g.WinnerID = currentPlayer.ID
		g.Message = fmt.Sprintf("Player %d wins with %d points!",
			g.WinnerID, currentPlayer.TotalScore)
	} else {
		g.Message = fmt.Sprintf("Player %d holds. Total: %d",
			currentPlayer.ID, currentPlayer.TotalScore)
		g.nextPlayer()
	}
}

func (g *PigGame) nextPlayer() {
	g.Players[g.CurrentIndex].IsActive = false
	g.CurrentIndex = (g.CurrentIndex + 1) % len(g.Players)
	g.Players[g.CurrentIndex].IsActive = true
	g.Message = fmt.Sprintf("Player %d's turn - SPACE to roll",
		g.Players[g.CurrentIndex].ID)
}

func (g *PigGame) Update() error {
	if g.GameOver {
		return nil
	}

	if g.inputCooldown > 0 {
		g.inputCooldown--
		return nil
	}

	if ebiten.IsKeyPressed(ebiten.KeySpace) {
		g.Roll()
		g.inputCooldown = 10
	}

	if ebiten.IsKeyPressed(ebiten.KeyEnter) {
		g.Hold()
		g.inputCooldown = 10
	}

	return nil
}

func (g *PigGame) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{30, 30, 50, 255})
	text.Draw(screen, g.Message, RegularFont, 50, 50, color.White)

	// Draw die
	dieText := fmt.Sprintf("Die: %d", g.DieValue)
	text.Draw(screen, dieText, RegularFont, 50, 100, color.White)

	// Draw scores
	for i, player := range g.Players {
		status := ""
		if player.IsActive {
			status = " (Current)"
		}
		scoreText := fmt.Sprintf("Player %d: %d points (Turn: %d)%s",
			player.ID, player.TotalScore, player.TurnScore, status)
		text.Draw(screen, scoreText, RegularFont, 50, 150+i*40, color.White)
	}

	// Instructions
	text.Draw(screen, "SPACE = Roll, ENTER = Hold", RegularFont, 50, ScreenHeight-50, color.White)
}

func (g *PigGame) Layout(_, _ int) (int, int) {
	return ScreenWidth, ScreenHeight
}
