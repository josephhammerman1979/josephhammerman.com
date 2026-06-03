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

	myPlayerIdx   int
	sendEventFn   func(string) // relay game event JSON to peers
	pendingEvent  *GameEvent   // received from network, applied on next Update
	localRolling  bool         // waiting for local animation + settle to finish
	rollRequested bool         // set by external input (e.g. phone shake / Roll button)
	holdRequested bool         // set by external input (Hold button)
	turnChangeFn  func(int)    // optional: invoked with CurrentIndex on every turn change
	gameOverFn    func(int)    // optional: invoked with the winner's index (-1 if no winner)
}

// RequestRoll asks the game to roll on the next Update, as if the local
// player had pressed SPACE. Ignored unless it is the local player's turn
// and the die is idle.
func (g *MultiplayerPigGame) RequestRoll() {
	g.rollRequested = true
}

// RequestHold asks the game to hold on the next Update, as if the local
// player had pressed ENTER.  Ignored unless it is the local player's turn
// and the die is idle.
func (g *MultiplayerPigGame) RequestHold() {
	g.holdRequested = true
}

// Kick marks the given player slot as removed from the game.  Their turn
// is forfeited (any in-flight TurnScore is dropped) and nextPlayer skips
// them from now on.  Called via window.diceGameKick from JS in response to
// a host-issued player_kick broadcast; every client invokes it locally so
// each WASM instance stays in sync.
//
// If only one un-kicked player remains, the game ends with them as the
// winner.
func (g *MultiplayerPigGame) Kick(slot int) {
	if slot < 0 || slot >= len(g.Players) {
		return
	}
	if g.Players[slot].Kicked || g.GameOver {
		return
	}
	g.Players[slot].Kicked = true
	g.Players[slot].TurnScore = 0
	g.Players[slot].IsActive = false

	// Count remaining un-kicked players.
	remaining := 0
	lastIdx := -1
	for i, p := range g.Players {
		if !p.Kicked {
			remaining++
			lastIdx = i
		}
	}
	if remaining <= 1 {
		g.GameOver = true
		winnerIdx := -1
		if remaining == 1 {
			winnerIdx = lastIdx
			g.WinnerID = g.Players[lastIdx].ID
			if lastIdx == g.myPlayerIdx {
				g.Message = "You win by default — every other player was kicked!"
			} else {
				g.Message = fmt.Sprintf("Player %d wins by default.", g.WinnerID)
			}
		} else {
			g.Message = "Game over — all players were kicked."
		}
		if g.gameOverFn != nil {
			g.gameOverFn(winnerIdx)
		}
		return
	}

	// If we just kicked the active player, advance to the next un-kicked one.
	if slot == g.CurrentIndex {
		g.nextPlayer()
	}
}

// SetTurnChangeFn registers a callback fired whenever CurrentIndex changes
// (including the initial value reported immediately on registration).  Used
// by the JS host to enable/disable on-screen Roll/Hold buttons and to drive
// the mobile fullscreen takeover.
func (g *MultiplayerPigGame) SetTurnChangeFn(fn func(int)) {
	g.turnChangeFn = fn
	if fn != nil {
		fn(g.CurrentIndex)
	}
}

// SetGameOverFn registers a callback fired exactly once when the game ends,
// either because a player reached WinningScore or because Kick reduced the
// roster to one un-kicked player.  The argument is the winning player's
// index, or -1 if every player was kicked.
func (g *MultiplayerPigGame) SetGameOverFn(fn func(int)) {
	g.gameOverFn = fn
}

// Reset restarts the game with a (possibly new) player count + local index,
// re-using the existing Ebiten run loop.  Called from JS via diceGameReset
// so a "New Game" click doesn't have to reload the WASM module.
func (g *MultiplayerPigGame) Reset(numPlayers, myPlayerIdx int) {
	if numPlayers < 1 {
		return
	}
	rand.Seed(time.Now().UnixNano())
	g.Players = make([]*Player, numPlayers)
	for i := range g.Players {
		g.Players[i] = &Player{ID: i + 1}
	}
	g.Players[0].IsActive = true
	g.CurrentIndex = 0
	g.GameOver = false
	g.WinnerID = 0
	g.myPlayerIdx = myPlayerIdx
	g.inputCooldown = 30
	g.localRolling = false
	g.rollRequested = false
	g.holdRequested = false
	g.pendingEvent = nil
	g.Die = NewDice()

	if myPlayerIdx == 0 {
		g.Message = fmt.Sprintf("Your turn, Player %d — SPACE to roll", myPlayerIdx+1)
	} else {
		g.Message = "Waiting for Player 1 to roll..."
	}
	if g.turnChangeFn != nil {
		g.turnChangeFn(g.CurrentIndex)
	}
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
		if g.gameOverFn != nil {
			g.gameOverFn(playerIdx)
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

	// Advance to the next un-kicked player. Kick() guarantees at least two
	// un-kicked players exist whenever this is reached (it ends the game
	// itself when one remains), so this loop is bounded.
	n := len(g.Players)
	for i := 1; i <= n; i++ {
		next := (g.CurrentIndex + i) % n
		if !g.Players[next].Kicked {
			g.CurrentIndex = next
			break
		}
	}
	g.Players[g.CurrentIndex].IsActive = true

	if g.CurrentIndex == g.myPlayerIdx {
		g.Message = fmt.Sprintf("Your turn, Player %d — SPACE to roll", g.myPlayerIdx+1)
	} else {
		g.Message = fmt.Sprintf("Player %d's turn...", g.Players[g.CurrentIndex].ID)
	}
	if g.turnChangeFn != nil {
		g.turnChangeFn(g.CurrentIndex)
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
		// Drop any stale shake-roll / button-press requests: they only apply on our turn.
		g.rollRequested = false
		g.holdRequested = false
		return nil
	}

	if g.inputCooldown > 0 {
		g.inputCooldown--
		return nil
	}

	if ebiten.IsKeyPressed(ebiten.KeySpace) || g.rollRequested {
		g.rollRequested = false
		g.Die.StartRoll()
		g.localRolling = true
		g.Message = "Rolling..."
		g.inputCooldown = 10
	}

	if ebiten.IsKeyPressed(ebiten.KeyEnter) || g.holdRequested {
		g.holdRequested = false
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
		case player.Kicked:
			col = color.RGBA{120, 90, 90, 255} // muted red-grey for kicked
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
		var scoreText string
		if player.Kicked {
			scoreText = fmt.Sprintf("%s %d:  %d pts  (kicked)",
				label, player.ID, player.TotalScore)
		} else {
			scoreText = fmt.Sprintf("%s %d:  %d pts  (turn: %d)",
				label, player.ID, player.TotalScore, player.TurnScore)
		}
		text.Draw(screen, scoreText, RegularFont, originX, y, col)
		y += lineHeight
	}
}

func (g *MultiplayerPigGame) Layout(_, _ int) (int, int) {
	return ScreenWidth, ScreenHeight
}
