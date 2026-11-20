package game

import (
	"encoding/json"
	"testing"
)

// rollWith resolves a roll for the current player with the given faces, as
// if the dice animation had just finished.
func rollWith(g *MultiplayerPigGame, faces ...int) {
	g.Die.Value = faces[0]
	if g.Die2 != nil {
		g.Die2.Value = faces[1]
	}
	g.applyRollResult()
}

// runUntilIdle steps Update until no events are queued and the dice are at rest.
func runUntilIdle(t *testing.T, g *MultiplayerPigGame) {
	t.Helper()
	for i := 0; i < 2000; i++ {
		if len(g.pendingEvents) == 0 && !g.diceAnimating() && !g.diceFinished() {
			return
		}
		g.Update()
	}
	t.Fatal("game did not settle")
}

func TestBigPigDoubleOnesScores25AndContinues(t *testing.T) {
	g := NewMultiplayerPigGame(2, 0, VariantBigPig, nil)
	rollWith(g, 3, 4)
	rollWith(g, 1, 1)

	p := g.Players[0]
	if p.TurnScore != 7+BigPigDoubleOnes {
		t.Fatalf("turn score = %d, want %d", p.TurnScore, 7+BigPigDoubleOnes)
	}
	if g.CurrentIndex != 0 {
		t.Fatalf("turn passed after double ones; current = %d", g.CurrentIndex)
	}
}

func TestBigPigSingleOneBustsTurnOnly(t *testing.T) {
	g := NewMultiplayerPigGame(2, 0, VariantBigPig, nil)
	g.Players[0].TotalScore = 40
	rollWith(g, 5, 6)
	rollWith(g, 1, 4)

	p := g.Players[0]
	if p.TurnScore != 0 || p.TotalScore != 40 {
		t.Fatalf("after bust: turn=%d total=%d, want 0 and 40", p.TurnScore, p.TotalScore)
	}
	if g.CurrentIndex != 1 {
		t.Fatalf("turn did not pass after bust; current = %d", g.CurrentIndex)
	}
	if g.LastAction.Kind != ActionBust || g.LastAction.Points != 11 {
		t.Fatalf("last action = %+v, want bust losing 11", g.LastAction)
	}
}

func TestBigPigOtherDoublesScoreTheirSum(t *testing.T) {
	g := NewMultiplayerPigGame(2, 0, VariantBigPig, nil)
	rollWith(g, 6, 6)
	if got := g.Players[0].TurnScore; got != 12 {
		t.Fatalf("6+6 scored %d, want 12", got)
	}
}

func TestBustResultSurvivesTurnChange(t *testing.T) {
	g := NewMultiplayerPigGame(2, 0, VariantPig, nil)
	rollWith(g, 4)
	rollWith(g, 1)

	if g.CurrentIndex != 1 {
		t.Fatalf("current = %d, want 1", g.CurrentIndex)
	}
	if g.LastAction.Kind != ActionBust || g.LastAction.PlayerIdx != 0 || g.LastAction.Points != 4 {
		t.Fatalf("last action = %+v, want player 0 bust losing 4", g.LastAction)
	}
}

func TestQueuedEventsAreAllApplied(t *testing.T) {
	// We are player 2; player 0's roll and hold arrive in the same frame
	// (e.g. our tab was in the background).
	g := NewMultiplayerPigGame(3, 2, VariantPig, nil)
	g.ReceiveEvent(`{"action":"roll","values":[5],"playerIdx":0}`)
	g.ReceiveEvent(`{"action":"hold","playerIdx":0}`)
	runUntilIdle(t, g)

	if got := g.Players[0].TotalScore; got != 5 {
		t.Fatalf("player 0 total = %d, want 5", got)
	}
	if g.CurrentIndex != 1 {
		t.Fatalf("current = %d, want 1", g.CurrentIndex)
	}
}

func TestQueuedRollAnimatesWhenAlone(t *testing.T) {
	g := NewMultiplayerPigGame(3, 2, VariantPig, nil)
	g.ReceiveEvent(`{"action":"roll","values":[3],"playerIdx":0}`)
	runUntilIdle(t, g)
	if got := g.Players[0].TurnScore; got != 3 {
		t.Fatalf("player 0 turn score = %d, want 3", got)
	}
}

func TestKickMidRollDoesNotScoreForNextPlayer(t *testing.T) {
	g := NewMultiplayerPigGame(3, 2, VariantPig, nil)
	g.ReceiveEvent(`{"action":"roll","values":[6],"playerIdx":0}`)
	g.Update() // starts the animation
	if !g.diceAnimating() {
		t.Fatal("expected roll animation to be running")
	}
	g.Kick(0)
	runUntilIdle(t, g)

	if g.CurrentIndex != 1 {
		t.Fatalf("current = %d, want 1", g.CurrentIndex)
	}
	if got := g.Players[1].TurnScore; got != 0 {
		t.Fatalf("kicked player's roll scored %d for player 1", got)
	}
}

func TestKickLastOpponentEndsGame(t *testing.T) {
	g := NewMultiplayerPigGame(2, 0, VariantPig, nil)
	var winner = -2
	g.SetGameOverFn(func(i int) { winner = i })
	g.Kick(1)
	if !g.GameOver || winner != 0 {
		t.Fatalf("game over = %v, winner = %d; want true, 0", g.GameOver, winner)
	}
}

func TestLoadStateRestoresLiveGame(t *testing.T) {
	src := NewMultiplayerPigGame(3, 0, VariantBigPig, nil)
	rollWith(src, 3, 4)
	src.applyHold(0)
	rollWith(src, 5, 5)
	src.Kick(2)

	dst := NewMultiplayerPigGame(3, 1, VariantBigPig, nil)
	turns := []int{}
	dst.SetTurnChangeFn(func(i int) { turns = append(turns, i) })
	dst.LoadState(src.StateJSON())

	if dst.CurrentIndex != 1 || dst.Players[0].TotalScore != 7 ||
		dst.Players[1].TurnScore != 10 || !dst.Players[2].Kicked {
		got, _ := json.Marshal(dst.snapshot())
		t.Fatalf("state not restored: %s", got)
	}
	if turns[len(turns)-1] != 1 {
		t.Fatalf("turn change not reported: %v", turns)
	}
	if dst.myPlayerIdx != 1 {
		t.Fatalf("LoadState overwrote local player index")
	}
}

func TestLoadStateIgnoresMismatchedGame(t *testing.T) {
	src := NewMultiplayerPigGame(2, 0, VariantPig, nil)
	src.Players[0].TotalScore = 50
	dst := NewMultiplayerPigGame(3, 0, VariantPig, nil)
	dst.LoadState(src.StateJSON())
	if dst.Players[0].TotalScore != 0 {
		t.Fatal("snapshot for a different player count was applied")
	}
}
