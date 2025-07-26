package game

import (
	"fmt"
	"image/color"
	"math/rand"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text"
)

const WinningScore = 100

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
	Message       string
	inputCooldown int
	Die           *Dice
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
		Message:       fmt.Sprintf("Player %d's turn - SPACE to roll", 1),
		inputCooldown: 30,
		Die:           NewDice(),
	}
}

func (g *PigGame) Roll() {
	if g.Die.Animating {
		return
	}
	rand.Seed(time.Now().UnixNano())
	g.Die.StartRoll()
}

func (g *PigGame) ProcessRollResult() {
	currentPlayer := g.Players[g.CurrentIndex]

	if g.Die.Value == 1 {
		currentPlayer.TurnScore = 0
		g.Message = fmt.Sprintf("Player %d rolled 1! Turn lost", currentPlayer.ID)
		g.nextPlayer()
	} else {
		currentPlayer.TurnScore += g.Die.Value
		g.Message = fmt.Sprintf("Player %d rolled %d (Turn: %d)",
			currentPlayer.ID, g.Die.Value, currentPlayer.TurnScore)
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

	g.Die.Update()

	if !g.Die.Animating && g.Die.animationTick > 0 {
		g.ProcessRollResult()
		g.Die.animationTick = 0 // Reset animation tick so result isn't re-processed
		return nil
	}

	// No input accepted while animation is running
	if g.Die.Animating {
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

	// Draw dice area background
	DrawPlayArea(screen, ScreenWidth-250, 250, ScreenHeight)

	// Render player scores on left side
	DrawPlayerScores(screen, g.Players, g.CurrentIndex, 50, 100)

	// Draw the die in the right-hand play area
	dieX := ScreenWidth - 200
	dieY := ScreenHeight/2 - 50
	g.Die.Draw(screen, dieX, dieY, 100)

	// Instructions
	text.Draw(screen, "SPACE = Roll, ENTER = Hold", RegularFont, 50, ScreenHeight-50, color.White)
}

func (g *PigGame) Layout(_, _ int) (int, int) {
	return ScreenWidth, ScreenHeight
}
