package game

import (
	"encoding/json"
	"fmt"
	"image/color"
	"math/rand"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/text"
)

// GameEvent is exchanged between peers over WebSocket to synchronise game state.
type GameEvent struct {
	Action    string `json:"action"`          // "roll" | "hold"
	Value     int    `json:"value,omitempty"` // die result (roll events only)
	PlayerIdx int    `json:"playerIdx"`       // zero-based index of the acting player
}

// MultiplayerPigGame is a network-aware Pig Dice game.
// Each browser tab runs its own instance; actions are broadcast to peers and
// applied locally so every player sees the same game state.
//
// myPlayerIdx identifies which player "owns" this instance.
// sendEventFn(json) is called whenever the local player takes a game action;
// the caller is responsible for relaying the JSON to all peers.
type MultiplayerPigGame struct {
	Players       []*Player
	CurrentIndex  int
	GameOver      bool
	WinnerID      int
	Message       string
	inputCooldown int
	Die           *Dice

	myPlayerIdx  int
	sendEventFn  func(string) // relay game event JSON to peers
	pendingEvent *GameEvent   // received from network, applied on next Update
	localRolling bool         // waiting for local animation + settle to finish
}

func NewMultiplayerPigGame(numPlayers, myPlayerIdx int, sendEventFn func(string)) *MultiplayerPigGame {
	rand.Seed(time.Now().UnixNano())
	players := make([]*Player, numPlayers)
	for i := range players {
		players[i] = &Player{ID: i + 1}
	}
	players[0].IsActive = true

	var msg string
	if myPlayerIdx == 0 {
		msg = fmt.Sprintf("Your turn, Player %d — SPACE to roll", myPlayerIdx+1)
	} else {
		msg = "Waiting for Player 1 to roll..."
	}

	return &MultiplayerPigGame{
		Players:       players,
		CurrentIndex:  0,
		Message:       msg,
		inputCooldown: 30,
		Die:           NewDice(),
		myPlayerIdx:   myPlayerIdx,
		sendEventFn:   sendEventFn,
	}
}

// ReceiveEvent is called from JavaScript when a game event arrives over WebSocket.
// It is safe to call between Ebiten frames (WASM is single-threaded).
func (g *MultiplayerPigGame) ReceiveEvent(jsonStr string) {
	var evt GameEvent
	if err := json.Unmarshal([]byte(jsonStr), &evt); err != nil {
		return
	}
	g.pendingEvent = &evt
}

func (g *MultiplayerPigGame) broadcast(evt GameEvent) {
	if g.sendEventFn == nil {
		return
	}
	data, _ := json.Marshal(evt)
	g.sendEventFn(string(data))
}

func (g *MultiplayerPigGame) applyRollResult() {
	cur := g.Players[g.CurrentIndex]
	if g.Die.Value == 1 {
		cur.TurnScore = 0
		if g.CurrentIndex == g.myPlayerIdx {
			g.Message = "You rolled 1 — turn lost!"
		} else {
			g.Message = fmt.Sprintf("Player %d rolled 1 — turn lost!", cur.ID)
		}
		g.nextPlayer()
	} else {
		cur.TurnScore += g.Die.Value
		if g.CurrentIndex == g.myPlayerIdx {
			g.Message = fmt.Sprintf("You rolled %d! Turn total: %d — SPACE to roll again, ENTER to hold",
				g.Die.Value, cur.TurnScore)
		} else {
			g.Message = fmt.Sprintf("Player %d rolled %d (turn total: %d)",
				cur.ID, g.Die.Value, cur.TurnScore)
		}
	}
}

func (g *MultiplayerPigGame) applyHold(playerIdx int) {
	if playerIdx != g.CurrentIndex {
		return
	}
	cur := g.Players[g.CurrentIndex]
	cur.TotalScore += cur.TurnScore
	cur.TurnScore = 0

	if cur.TotalScore >= WinningScore {
		g.GameOver = true
		g.WinnerID = cur.ID
		if playerIdx == g.myPlayerIdx {
			g.Message = fmt.Sprintf("You win with %d points!", cur.TotalScore)
		} else {
			g.Message = fmt.Sprintf("Player %d wins with %d points!", g.WinnerID, cur.TotalScore)
		}
	} else {
		if playerIdx == g.myPlayerIdx {
			g.Message = fmt.Sprintf("You held — total: %d. Passing turn.", cur.TotalScore)
		} else {
			g.Message = fmt.Sprintf("Player %d holds. Total: %d", cur.ID, cur.TotalScore)
		}
		g.nextPlayer()
	}
}

func (g *MultiplayerPigGame) nextPlayer() {
	g.Players[g.CurrentIndex].IsActive = false
	g.CurrentIndex = (g.CurrentIndex + 1) % len(g.Players)
	g.Players[g.CurrentIndex].IsActive = true

	if g.CurrentIndex == g.myPlayerIdx {
		g.Message = fmt.Sprintf("Your turn, Player %d — SPACE to roll", g.myPlayerIdx+1)
	} else {
		g.Message = fmt.Sprintf("Player %d's turn...", g.Players[g.CurrentIndex].ID)
	}
}

func (g *MultiplayerPigGame) Update() error {
	if g.GameOver {
		return nil
	}

	// Apply a queued network event only when the die is idle.
	if g.pendingEvent != nil && !g.Die.Animating && !g.Die.Settling {
		evt := g.pendingEvent
		g.pendingEvent = nil
		switch evt.Action {
		case "roll":
			if evt.PlayerIdx == g.CurrentIndex {
				g.Die.StartRollWithFinal(evt.Value)
			}
		case "hold":
			g.applyHold(evt.PlayerIdx)
		}
	}

	// Advance die animation / settle.
	g.Die.Update()

	// When animation + settle is complete, process the result.
	if !g.Die.Animating && !g.Die.Settling && g.Die.animationTick > 0 {
		if g.localRolling {
			// Broadcast result only after the local animation finishes.
			g.broadcast(GameEvent{Action: "roll", Value: g.Die.Value, PlayerIdx: g.myPlayerIdx})
			g.localRolling = false
		}
		g.applyRollResult()
		g.Die.animationTick = 0
		return nil
	}

	// Only accept input when it is our turn and the die is idle.
	if g.CurrentIndex != g.myPlayerIdx ||
		g.Die.Animating || g.Die.Settling || g.localRolling {
		return nil
	}

	if g.inputCooldown > 0 {
		g.inputCooldown--
		return nil
	}

	if ebiten.IsKeyPressed(ebiten.KeySpace) {
		g.Die.StartRoll()
		g.localRolling = true
		g.Message = "Rolling..."
		g.inputCooldown = 10
	}

	if ebiten.IsKeyPressed(ebiten.KeyEnter) {
		g.broadcast(GameEvent{Action: "hold", PlayerIdx: g.myPlayerIdx})
		g.applyHold(g.myPlayerIdx)
		g.inputCooldown = 10
	}

	return nil
}

func (g *MultiplayerPigGame) Draw(screen *ebiten.Image) {
	// Background
	screen.Fill(color.RGBA{18, 20, 38, 255})

	// Decorative header strip
	ebitenutil.DrawRect(screen, 0, 0, float64(ScreenWidth), 60, color.RGBA{40, 35, 80, 255})
	text.Draw(screen, "Pig Dice  —  Multiplayer", RegularFont, 24, 40, color.RGBA{180, 160, 255, 255})

	// Status message
	msgCol := color.RGBA{255, 225, 90, 255}
	if g.GameOver {
		msgCol = color.RGBA{100, 255, 160, 255}
	} else if g.CurrentIndex != g.myPlayerIdx {
		msgCol = color.RGBA{160, 200, 255, 255}
	}
	text.Draw(screen, g.Message, RegularFont, 24, 90, msgCol)

	// Play-area panel (right side)
	DrawPlayArea(screen, ScreenWidth-260, 260, ScreenHeight)

	// Player score list (left side)
	DrawPlayerScoresMultiplayer(screen, g.Players, g.CurrentIndex, g.myPlayerIdx, 24, 120)

	// Die
	dieX := ScreenWidth - 205
	dieY := ScreenHeight/2 - 55
	g.Die.Draw(screen, dieX, dieY, 110)

	// Footer instructions
	footerCol := color.RGBA{120, 120, 140, 255}
	footerMsg := ""
	if !g.GameOver {
		if g.CurrentIndex == g.myPlayerIdx && !g.Die.Animating && !g.Die.Settling && !g.localRolling {
			footerMsg = "SPACE = Roll   ENTER = Hold"
			footerCol = color.RGBA{140, 240, 140, 255}
		} else if g.Die.Animating || g.Die.Settling || g.localRolling {
			footerMsg = "Rolling..."
			footerCol = color.RGBA{255, 200, 100, 255}
		} else {
			footerMsg = fmt.Sprintf("Waiting for Player %d...", g.Players[g.CurrentIndex].ID)
		}
	} else {
		footerMsg = "Game over! Refresh to play again."
		footerCol = color.RGBA{100, 255, 160, 255}
	}
	text.Draw(screen, footerMsg, RegularFont, 24, ScreenHeight-30, footerCol)
}

// DrawPlayerScoresMultiplayer renders the score list, highlighting the local player.
func DrawPlayerScoresMultiplayer(
	screen *ebiten.Image,
	players []*Player,
	currentIndex, myPlayerIdx int,
	originX, originY int,
) {
	y := originY
	lineHeight := 44

	for i, player := range players {
		var col color.RGBA

		switch {
		case i == currentIndex && i == myPlayerIdx:
			col = color.RGBA{255, 230, 80, 255}  // active + mine: gold
		case i == currentIndex:
			col = color.RGBA{100, 200, 255, 255} // active other: cyan
		case i == myPlayerIdx:
			col = color.RGBA{200, 180, 255, 255} // my non-active: lavender
		default:
			col = color.RGBA{160, 160, 170, 255} // others
		}

		label := "Player"
		if i == myPlayerIdx {
			label = "You   "
		}
		scoreText := fmt.Sprintf("%s %d:  %d pts  (turn: %d)",
			label, player.ID, player.TotalScore, player.TurnScore)
		text.Draw(screen, scoreText, RegularFont, originX, y, col)
		y += lineHeight
	}
}

func (g *MultiplayerPigGame) Layout(_, _ int) (int, int) {
	return ScreenWidth, ScreenHeight
}
