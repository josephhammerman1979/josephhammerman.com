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

// Variant identifies which Pig ruleset the multiplayer game is running.
// "pig" is the classic single-die variant; "bigpig" rolls two dice (a 1 on
// either die busts the turn, double ones score 25).
const (
	VariantPig    = "pig"
	VariantBigPig = "bigpig"
)

// BigPigDoubleOnes is the score for rolling 1+1 in Big Pig.
const BigPigDoubleOnes = 25

// Action kinds reported in ActionSummary.Kind.
const (
	ActionRoll = "roll" // scoring roll; player continues
	ActionBust = "bust" // rolled a 1; turn points lost
	ActionHold = "hold" // banked turn points
	ActionWin  = "win"  // banked and reached WinningScore
	ActionKick = "kick" // player removed from the game
)

// GameEvent is exchanged between peers over WebSocket to synchronise game state.
type GameEvent struct {
	Action    string `json:"action"`           // "roll" | "hold"
	Values    []int  `json:"values,omitempty"` // die results — len 1 for Pig, len 2 for BigPig
	PlayerIdx int    `json:"playerIdx"`        // zero-based index of the acting player
}

// ActionSummary describes the most recent thing that happened in the game.
// It is kept separate from the turn prompt so that a bust or hold result is
// still visible after the turn has passed to the next player.
type ActionSummary struct {
	Seq       int    `json:"seq"`       // increments on every action; 0 = nothing yet
	Kind      string `json:"kind"`      // one of the Action* constants
	PlayerIdx int    `json:"playerIdx"` // who acted (or who was kicked)
	Dice      []int  `json:"dice,omitempty"`
	Points    int    `json:"points"`    // roll: points gained; bust: points lost; hold/win: points banked
	TurnTotal int    `json:"turnTotal"` // turn total after the action
	Total     int    `json:"total"`     // banked total after the action
	Note      string `json:"note,omitempty"`
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
	Message       string        // prompt for the current turn
	LastAction    ActionSummary // most recent roll / bust / hold / kick
	inputCooldown int
	Variant       string // VariantPig (1 die) or VariantBigPig (2 dice)
	Die           *Dice  // primary die (both variants)
	Die2          *Dice  // BigPig only — nil for Pig

	myPlayerIdx   int
	sendEventFn   func(string) // relay game event JSON to peers
	pendingEvents []GameEvent  // received from network, applied in order once dice are idle
	localRolling  bool         // waiting for local animation + settle to finish
	rollRequested bool         // set by external input (e.g. phone shake / Roll button)
	holdRequested bool         // set by external input (Hold button)
	turnChangeFn  func(int)    // optional: invoked with CurrentIndex on every turn change
	gameOverFn    func(int)    // optional: invoked with the winner's index (-1 if no winner)
	stateChangeFn func(string) // optional: invoked with a JSON snapshot whenever scores/message/turn change
}

// StateSnapshot is the serialisable game state surfaced to JS via the
// SetStateChangeFn callback (and the diceGameGetState bridge).  It contains
// everything the DOM panel needs to render scores, turn, and roll status
// without depending on the canvas being visible.  It is also the payload a
// rejoining player receives so LoadState can restore the live game.
type StateSnapshot struct {
	Variant      string               `json:"variant"`
	CurrentIndex int                  `json:"currentIndex"`
	MyPlayerIdx  int                  `json:"myPlayerIdx"`
	Message      string               `json:"message"`
	GameOver     bool                 `json:"gameOver"`
	WinnerID     int                  `json:"winnerID"`
	WinningScore int                  `json:"winningScore"`
	DieValues    []int                `json:"dieValues"`
	Rolling      bool                 `json:"rolling"`
	LastAction   ActionSummary        `json:"lastAction"`
	Players      []PlayerStateSummary `json:"players"`
}

// PlayerStateSummary is the per-player slice of StateSnapshot.
type PlayerStateSummary struct {
	ID         int  `json:"id"`
	TotalScore int  `json:"totalScore"`
	TurnScore  int  `json:"turnScore"`
	Kicked     bool `json:"kicked"`
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
// is forfeited (any in-flight TurnScore and roll animation are dropped) and
// nextPlayer skips them from now on.  Called via window.diceGameKick from JS
// in response to a player_kick broadcast; every client invokes it locally so
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
	kicked := g.Players[slot]
	kicked.Kicked = true
	kicked.TurnScore = 0
	kicked.IsActive = false
	g.setAction(ActionSummary{Kind: ActionKick, PlayerIdx: slot, Total: kicked.TotalScore})

	// Drop any queued actions from the kicked player.
	kept := g.pendingEvents[:0]
	for _, evt := range g.pendingEvents {
		if evt.PlayerIdx != slot {
			kept = append(kept, evt)
		}
	}
	g.pendingEvents = kept

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
		g.cancelRoll()
		winnerIdx := -1
		if remaining == 1 {
			winnerIdx = lastIdx
			g.WinnerID = g.Players[lastIdx].ID
		}
		g.refreshPrompt()
		g.emitState()
		if g.gameOverFn != nil {
			g.gameOverFn(winnerIdx)
		}
		return
	}

	// If we just kicked the active player, abandon their roll (so its result
	// can't land on the next player) and advance to the next un-kicked one.
	if slot == g.CurrentIndex {
		g.cancelRoll()
		g.nextPlayer()
	}
	g.emitState()
}

// cancelRoll aborts any in-flight roll animation without scoring it.
func (g *MultiplayerPigGame) cancelRoll() {
	if g.Die != nil {
		g.Die.Cancel()
	}
	if g.Die2 != nil {
		g.Die2.Cancel()
	}
	g.localRolling = false
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

// SetStateChangeFn registers a callback fired whenever any scoring / message
// / turn state changes.  The argument is a JSON-encoded StateSnapshot — see
// that type for the schema.  The callback is also fired once immediately on
// registration so the DOM panel can paint its initial state.
func (g *MultiplayerPigGame) SetStateChangeFn(fn func(string)) {
	g.stateChangeFn = fn
	if fn != nil {
		g.emitState()
	}
}

// StateJSON returns the current state as a JSON string. Exposed via
// window.diceGameGetState for late binding (callers that didn't catch the
// initial SetStateChangeFn fire-and-forget can poll once on mount) and so a
// peer can hand the live state to a player who rejoins mid-game.
func (g *MultiplayerPigGame) StateJSON() string {
	data, _ := json.Marshal(g.snapshot())
	return string(data)
}

// LoadState replaces the scores, turn and game-over state with a snapshot
// taken on another peer (see StateJSON).  Used when a player rejoins mid-game
// so they resume the live game instead of a fresh 0–0 board.  Snapshots for
// a different variant or player count are ignored.
func (g *MultiplayerPigGame) LoadState(jsonStr string) {
	var s StateSnapshot
	if err := json.Unmarshal([]byte(jsonStr), &s); err != nil {
		return
	}
	if normaliseVariant(s.Variant) != g.Variant || len(s.Players) != len(g.Players) {
		return
	}
	if s.CurrentIndex < 0 || s.CurrentIndex >= len(g.Players) {
		return
	}

	g.cancelRoll()
	g.pendingEvents = nil
	g.rollRequested = false
	g.holdRequested = false
	for i, p := range s.Players {
		g.Players[i].TotalScore = p.TotalScore
		g.Players[i].TurnScore = p.TurnScore
		g.Players[i].Kicked = p.Kicked
		g.Players[i].IsActive = i == s.CurrentIndex
	}
	g.CurrentIndex = s.CurrentIndex
	g.GameOver = s.GameOver
	g.WinnerID = s.WinnerID
	g.LastAction = s.LastAction
	if len(s.DieValues) > 0 && validFace(s.DieValues[0]) {
		g.Die.Value = s.DieValues[0]
	}
	if g.Die2 != nil && len(s.DieValues) > 1 && validFace(s.DieValues[1]) {
		g.Die2.Value = s.DieValues[1]
	}
	g.refreshPrompt()

	if g.turnChangeFn != nil {
		g.turnChangeFn(g.CurrentIndex)
	}
	g.emitState()
	if g.GameOver && g.gameOverFn != nil {
		g.gameOverFn(g.WinnerID - 1)
	}
}

func validFace(v int) bool { return v >= 1 && v <= 6 }

func (g *MultiplayerPigGame) snapshot() StateSnapshot {
	players := make([]PlayerStateSummary, len(g.Players))
	for i, p := range g.Players {
		players[i] = PlayerStateSummary{
			ID:         p.ID,
			TotalScore: p.TotalScore,
			TurnScore:  p.TurnScore,
			Kicked:     p.Kicked,
		}
	}
	return StateSnapshot{
		Variant:      g.Variant,
		CurrentIndex: g.CurrentIndex,
		MyPlayerIdx:  g.myPlayerIdx,
		Message:      g.Message,
		GameOver:     g.GameOver,
		WinnerID:     g.WinnerID,
		WinningScore: WinningScore,
		DieValues:    g.diceValues(),
		Rolling:      g.diceAnimating() || g.localRolling,
		LastAction:   g.LastAction,
		Players:      players,
	}
}

func (g *MultiplayerPigGame) diceValues() []int {
	values := []int{}
	if g.Die != nil {
		values = append(values, g.Die.Value)
	}
	if g.Die2 != nil {
		values = append(values, g.Die2.Value)
	}
	return values
}

func (g *MultiplayerPigGame) numDice() int {
	if g.Die2 != nil {
		return 2
	}
	return 1
}

// diceAnimating reports whether any die is mid-animation or settling.
func (g *MultiplayerPigGame) diceAnimating() bool {
	if g.Die != nil && (g.Die.Animating || g.Die.Settling) {
		return true
	}
	if g.Die2 != nil && (g.Die2.Animating || g.Die2.Settling) {
		return true
	}
	return false
}

// diceFinished returns true once every die has finished its roll AND at least
// one has actually run a roll since the last reset (animationTick > 0).
func (g *MultiplayerPigGame) diceFinished() bool {
	if g.diceAnimating() {
		return false
	}
	ticked := false
	if g.Die != nil && g.Die.animationTick > 0 {
		ticked = true
	}
	if g.Die2 != nil && g.Die2.animationTick > 0 {
		ticked = true
	}
	return ticked
}

func (g *MultiplayerPigGame) clearDiceTicks() {
	if g.Die != nil {
		g.Die.animationTick = 0
	}
	if g.Die2 != nil {
		g.Die2.animationTick = 0
	}
}

func (g *MultiplayerPigGame) emitState() {
	if g.stateChangeFn == nil {
		return
	}
	data, _ := json.Marshal(g.snapshot())
	g.stateChangeFn(string(data))
}

// setAction records a new LastAction, bumping its sequence number so the JS
// action log can tell a new event from a re-emitted one.
func (g *MultiplayerPigGame) setAction(a ActionSummary) {
	a.Seq = g.LastAction.Seq + 1
	g.LastAction = a
}

// normaliseVariant maps unknown / empty variant strings to VariantPig.
func normaliseVariant(v string) string {
	if v == VariantBigPig {
		return VariantBigPig
	}
	return VariantPig
}

// Reset restarts the game with a (possibly new) player count + local index +
// variant, re-using the existing Ebiten run loop.  Called from JS via
// diceGameReset so a "New Game" click can both change the variant and avoid
// reloading the WASM module.
func (g *MultiplayerPigGame) Reset(numPlayers, myPlayerIdx int, variant string) {
	if numPlayers < 1 {
		return
	}
	variant = normaliseVariant(variant)
	rand.Seed(time.Now().UnixNano())
	g.Players = make([]*Player, numPlayers)
	for i := range g.Players {
		g.Players[i] = &Player{ID: i + 1}
	}
	g.Players[0].IsActive = true
	g.CurrentIndex = 0
	g.GameOver = false
	g.WinnerID = 0
	g.LastAction = ActionSummary{}
	g.myPlayerIdx = myPlayerIdx
	g.inputCooldown = 30
	g.localRolling = false
	g.rollRequested = false
	g.holdRequested = false
	g.pendingEvents = nil
	g.Variant = variant
	g.Die = NewDice()
	if variant == VariantBigPig {
		g.Die2 = NewDice()
	} else {
		g.Die2 = nil
	}
	g.setupDiceBounce()

	g.refreshPrompt()
	if g.turnChangeFn != nil {
		g.turnChangeFn(g.CurrentIndex)
	}
	g.emitState()
}

func NewMultiplayerPigGame(numPlayers, myPlayerIdx int, variant string, sendEventFn func(string)) *MultiplayerPigGame {
	variant = normaliseVariant(variant)
	rand.Seed(time.Now().UnixNano())
	players := make([]*Player, numPlayers)
	for i := range players {
		players[i] = &Player{ID: i + 1}
	}
	players[0].IsActive = true

	g := &MultiplayerPigGame{
		Players:       players,
		CurrentIndex:  0,
		inputCooldown: 30,
		Variant:       variant,
		Die:           NewDice(),
		myPlayerIdx:   myPlayerIdx,
		sendEventFn:   sendEventFn,
	}
	if variant == VariantBigPig {
		g.Die2 = NewDice()
	}
	g.setupDiceBounce()
	g.refreshPrompt()
	return g
}

// setupDiceBounce pins each die's home position + play-area rectangle so
// rolls launch from a fixed spot and tumble inside the right-hand panel
// before settling. The numbers here mirror DrawPlayArea's geometry — the
// dice live in the right ~260px column of the 800×600 canvas.
func (g *MultiplayerPigGame) setupDiceBounce() {
	playMinX := float64(ScreenWidth - 260 + 18)
	playMinY := float64(160)
	playMaxX := float64(ScreenWidth - 18)
	playMaxY := float64(ScreenHeight - 90)

	if g.Die2 == nil {
		// Single die — centered in the play area.
		size := 110.0
		g.Die.Size = size
		homeX := (playMinX+playMaxX)/2 - size/2
		homeY := (playMinY+playMaxY)/2 - size/2
		g.Die.SetBounceArea(homeX, homeY, playMinX, playMinY, playMaxX, playMaxY)
		return
	}

	// Two dice — side by side, centered in the play area.
	size := 90.0
	gap := 18.0
	totalW := size*2 + gap
	leftX := (playMinX+playMaxX)/2 - totalW/2
	homeY := (playMinY+playMaxY)/2 - size/2
	g.Die.Size, g.Die2.Size = size, size
	g.Die.SetBounceArea(leftX, homeY, playMinX, playMinY, playMaxX, playMaxY)
	g.Die2.SetBounceArea(leftX+size+gap, homeY, playMinX, playMinY, playMaxX, playMaxY)
}

// refreshPrompt recomputes Message — the "what happens now" line — from the
// current turn and game-over state.  What just happened lives in LastAction.
func (g *MultiplayerPigGame) refreshPrompt() {
	if g.GameOver {
		switch {
		case g.WinnerID == 0:
			g.Message = "Game over — all players were removed."
		case g.WinnerID-1 == g.myPlayerIdx:
			g.Message = fmt.Sprintf("You win with %d points!", g.Players[g.WinnerID-1].TotalScore)
		default:
			g.Message = fmt.Sprintf("Player %d wins with %d points!",
				g.WinnerID, g.Players[g.WinnerID-1].TotalScore)
		}
		return
	}
	cur := g.Players[g.CurrentIndex]
	if g.CurrentIndex == g.myPlayerIdx {
		if cur.TurnScore > 0 {
			g.Message = fmt.Sprintf("Your turn — %d at risk. Roll again or Hold.", cur.TurnScore)
		} else {
			g.Message = "Your turn — Roll!"
		}
		return
	}
	if cur.TurnScore > 0 {
		g.Message = fmt.Sprintf("Player %d's turn — %d at risk", cur.ID, cur.TurnScore)
	} else {
		g.Message = fmt.Sprintf("Player %d's turn", cur.ID)
	}
}

// describeAction renders LastAction as a sentence for the canvas.  The DOM
// panel formats the same data itself so it can use display names.
func (g *MultiplayerPigGame) describeAction() string {
	a := g.LastAction
	if a.Seq == 0 || a.PlayerIdx < 0 || a.PlayerIdx >= len(g.Players) {
		return ""
	}
	who := fmt.Sprintf("Player %d", g.Players[a.PlayerIdx].ID)
	if a.PlayerIdx == g.myPlayerIdx {
		who = "You"
	}
	dice := ""
	if len(a.Dice) == 2 {
		dice = fmt.Sprintf("%d + %d", a.Dice[0], a.Dice[1])
	} else if len(a.Dice) == 1 {
		dice = fmt.Sprintf("%d", a.Dice[0])
	}
	switch a.Kind {
	case ActionRoll:
		if a.Note != "" {
			return fmt.Sprintf("%s rolled %s — %s +%d (turn %d)", who, dice, a.Note, a.Points, a.TurnTotal)
		}
		return fmt.Sprintf("%s rolled %s: +%d (turn %d)", who, dice, a.Points, a.TurnTotal)
	case ActionBust:
		return fmt.Sprintf("%s rolled %s — BUST, lost %d", who, dice, a.Points)
	case ActionHold:
		return fmt.Sprintf("%s held +%d → %d", who, a.Points, a.Total)
	case ActionWin:
		return fmt.Sprintf("%s held +%d → %d and won!", who, a.Points, a.Total)
	case ActionKick:
		return fmt.Sprintf("%s removed from the game", who)
	}
	return ""
}

// ReceiveEvent is called from JavaScript when a game event arrives over WebSocket.
// It is safe to call between Ebiten frames (WASM is single-threaded).  Events
// are queued so that several arriving between frames (e.g. while the tab was
// in the background and the run loop paused) are all applied, in order.
func (g *MultiplayerPigGame) ReceiveEvent(jsonStr string) {
	var evt GameEvent
	if err := json.Unmarshal([]byte(jsonStr), &evt); err != nil {
		return
	}
	g.pendingEvents = append(g.pendingEvents, evt)
}

func (g *MultiplayerPigGame) broadcast(evt GameEvent) {
	if g.sendEventFn == nil {
		return
	}
	data, _ := json.Marshal(evt)
	g.sendEventFn(string(data))
}

// processPendingEvents applies queued network events while the dice are idle.
// A roll with nothing queued behind it is animated; a roll with a backlog
// behind it is applied instantly so a returning background tab catches up
// immediately instead of replaying every animation.
func (g *MultiplayerPigGame) processPendingEvents() {
	for len(g.pendingEvents) > 0 && !g.diceAnimating() && !g.GameOver {
		evt := g.pendingEvents[0]
		g.pendingEvents = g.pendingEvents[1:]

		switch evt.Action {
		case "roll":
			if evt.PlayerIdx != g.CurrentIndex || len(evt.Values) < g.numDice() {
				continue
			}
			if !validFace(evt.Values[0]) || (g.Die2 != nil && !validFace(evt.Values[1])) {
				continue
			}
			if len(g.pendingEvents) > 0 {
				g.Die.Value = evt.Values[0]
				if g.Die2 != nil {
					g.Die2.Value = evt.Values[1]
				}
				g.applyRollResult()
				continue
			}
			g.Die.StartRollWithFinal(evt.Values[0])
			if g.Die2 != nil {
				g.Die2.StartRollWithFinal(evt.Values[1])
			}
			g.emitState()
			return
		case "hold":
			g.applyHold(evt.PlayerIdx)
		}
	}
}

func (g *MultiplayerPigGame) applyRollResult() {
	if g.Variant == VariantBigPig {
		g.applyBigPigRollResult()
		return
	}
	cur := g.Players[g.CurrentIndex]
	v := g.Die.Value
	if v == 1 {
		lost := cur.TurnScore
		cur.TurnScore = 0
		g.setAction(ActionSummary{Kind: ActionBust, PlayerIdx: g.CurrentIndex,
			Dice: []int{v}, Points: lost, Total: cur.TotalScore})
		g.nextPlayer()
		return
	}
	cur.TurnScore += v
	g.setAction(ActionSummary{Kind: ActionRoll, PlayerIdx: g.CurrentIndex,
		Dice: []int{v}, Points: v, TurnTotal: cur.TurnScore, Total: cur.TotalScore})
	g.refreshPrompt()
	g.emitState()
}

// applyBigPigRollResult applies the Big Pig scoring rules:
//   - Double ones (1+1): worth BigPigDoubleOnes; player may continue.
//   - Any single 1:      turn score lost, turn ends.
//   - Other:             score is the sum; player may continue.
func (g *MultiplayerPigGame) applyBigPigRollResult() {
	cur := g.Players[g.CurrentIndex]
	d1, d2 := g.Die.Value, g.Die2.Value
	a := ActionSummary{PlayerIdx: g.CurrentIndex, Dice: []int{d1, d2}}

	switch {
	case d1 == 1 && d2 == 1:
		a.Kind = ActionRoll
		a.Points = BigPigDoubleOnes
		a.Note = "double ones!"
	case d1 == 1 || d2 == 1:
		a.Kind = ActionBust
		a.Points = cur.TurnScore
		cur.TurnScore = 0
		a.Total = cur.TotalScore
		g.setAction(a)
		g.nextPlayer()
		return
	default:
		a.Kind = ActionRoll
		a.Points = d1 + d2
	}
	cur.TurnScore += a.Points
	a.TurnTotal = cur.TurnScore
	a.Total = cur.TotalScore
	g.setAction(a)
	g.refreshPrompt()
	g.emitState()
}

func (g *MultiplayerPigGame) applyHold(playerIdx int) {
	if playerIdx != g.CurrentIndex || g.GameOver {
		return
	}
	cur := g.Players[g.CurrentIndex]
	banked := cur.TurnScore
	cur.TotalScore += banked
	cur.TurnScore = 0

	if cur.TotalScore >= WinningScore {
		g.GameOver = true
		g.WinnerID = cur.ID
		g.setAction(ActionSummary{Kind: ActionWin, PlayerIdx: playerIdx,
			Points: banked, Total: cur.TotalScore})
		g.refreshPrompt()
		g.emitState()
		if g.gameOverFn != nil {
			g.gameOverFn(playerIdx)
		}
		return
	}
	g.setAction(ActionSummary{Kind: ActionHold, PlayerIdx: playerIdx,
		Points: banked, Total: cur.TotalScore})
	g.nextPlayer()
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

	g.refreshPrompt()
	if g.turnChangeFn != nil {
		g.turnChangeFn(g.CurrentIndex)
	}
	g.emitState()
}

func (g *MultiplayerPigGame) Update() error {
	if g.GameOver {
		return nil
	}

	g.processPendingEvents()

	// Advance dice animation / settle.
	if g.Die != nil {
		g.Die.Update()
	}
	if g.Die2 != nil {
		g.Die2.Update()
	}

	// When the whole roll has resolved, process the result.
	if g.diceFinished() {
		if g.localRolling {
			g.broadcast(GameEvent{Action: "roll", Values: g.diceValues(), PlayerIdx: g.myPlayerIdx})
			g.localRolling = false
		}
		g.applyRollResult()
		g.clearDiceTicks()
		return nil
	}

	// Only accept input when it is our turn and the dice are idle.
	if g.CurrentIndex != g.myPlayerIdx || g.Players[g.myPlayerIdx].Kicked ||
		g.diceAnimating() || g.localRolling {
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
		if g.Die2 != nil {
			g.Die2.StartRoll()
		}
		g.localRolling = true
		g.inputCooldown = 10
		g.emitState()
		return nil
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
	title := "Pig Dice  —  Multiplayer"
	if g.Variant == VariantBigPig {
		title = "Big Pig  —  Multiplayer"
	}
	text.Draw(screen, title, RegularFont, 24, 40, color.RGBA{180, 160, 255, 255})

	// Turn prompt
	msgCol := color.RGBA{255, 225, 90, 255}
	if g.GameOver {
		msgCol = color.RGBA{100, 255, 160, 255}
	} else if g.CurrentIndex != g.myPlayerIdx {
		msgCol = color.RGBA{160, 200, 255, 255}
	}
	text.Draw(screen, g.Message, RegularFont, 24, 90, msgCol)

	// Last action — busts in red so they register after the turn moves on.
	actCol := color.RGBA{170, 170, 190, 255}
	if g.LastAction.Kind == ActionBust {
		actCol = color.RGBA{255, 110, 110, 255}
	}
	text.Draw(screen, g.describeAction(), RegularFont, 24, 120, actCol)

	// Play-area panel (right side)
	DrawPlayArea(screen, ScreenWidth-260, 260, ScreenHeight)

	// Player score list (left side)
	DrawPlayerScoresMultiplayer(screen, g.Players, g.CurrentIndex, g.myPlayerIdx, 24, 160)

	// Dice — drawn at their physics-driven position. setupDiceBounce sets
	// Size and HomeX/Y so even when idle (X = HomeX, Y = HomeY) the die is
	// centered in the right-hand play-area panel.
	g.Die.Draw(screen, int(g.Die.X), int(g.Die.Y), int(g.Die.Size))
	if g.Die2 != nil {
		g.Die2.Draw(screen, int(g.Die2.X), int(g.Die2.Y), int(g.Die2.Size))
	}

	// Footer instructions
	footerCol := color.RGBA{120, 120, 140, 255}
	footerMsg := ""
	rolling := g.diceAnimating() || g.localRolling
	if !g.GameOver {
		if g.CurrentIndex == g.myPlayerIdx && !rolling {
			footerMsg = "SPACE = Roll   ENTER = Hold"
			footerCol = color.RGBA{140, 240, 140, 255}
		} else if rolling {
			footerMsg = "Rolling..."
			footerCol = color.RGBA{255, 200, 100, 255}
		} else {
			footerMsg = fmt.Sprintf("Waiting for Player %d...", g.Players[g.CurrentIndex].ID)
		}
	} else {
		footerMsg = "Game over!"
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
			col = color.RGBA{255, 230, 80, 255} // active + mine: gold
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
			scoreText = fmt.Sprintf("%s %d:  %d pts  (removed)",
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
