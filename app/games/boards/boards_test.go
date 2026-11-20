package boards

import (
	"strings"
	"testing"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

func TestBoardGamesPlayToCompletion(t *testing.T) {
	cases := []struct {
		id   string
		opts map[string]string
	}{
		{"klondike", nil},
		{"klondike", map[string]string{"variant": "goldrush", "chips": "50", "banks": "2"}},
		{"klondike", map[string]string{"variant": "classic", "chips": "50", "banks": "2"}},
		{"klondike", map[string]string{"variant": "goldrush", "chips": "20", "banks": "1"}},
		{"hundreds", nil},
		{"hundreds", map[string]string{"variant": "postman"}},
		{"threehands", nil},
		{"threehands", map[string]string{"variant": "five"}},
		{"rulemaker", nil},
	}
	for _, c := range cases {
		if err := games.SimulateMany(games.Lookup(c.id), 5, c.opts); err != nil {
			t.Fatal(err)
		}
	}
}

func act(t *testing.T, g games.Game, seat int, a games.Action) {
	t.Helper()
	if err := g.Act(seat, a); err != nil {
		t.Fatalf("seat %d %+v: %v", seat, a, err)
	}
}

// ── Klondike ──────────────────────────────────────────────────────────────────

func TestKlondikeClassicSettles(t *testing.T) {
	// Banker (seat 0) rolls a pair of 3s; seat 1 rolls two pair (wins),
	// seat 2 rolls a pair of 3s with the same kickers (tie → banker).
	env := games.NewTestEnv(
		3, 3, 1, 2, 5, // banker: pair of 3s
		4, 4, 2, 2, 6, // seat 1: two pair
		3, 3, 5, 2, 1, // seat 2: same pair, same kickers
	)
	g := newKlondike(env, 3, false, 20, 1)
	if err := g.Act(0, games.Action{Type: "wager", Amount: 3}); err == nil {
		t.Fatal("banker shouldn't be able to bet")
	}
	if err := g.Act(1, games.Action{Type: "wager", Amount: 6}); err == nil {
		t.Fatal("bets over 5 should be rejected")
	}
	act(t, g, 1, games.Action{Type: "wager", Amount: 4})
	// Seat 1's bet is hidden from seat 2 until all bets are in.
	if v := g.View(2).(bankerView); v.Seats[1].Wager != -1 || !v.Seats[1].Wagered || g.View(1).(bankerView).Seats[1].Wager != 4 {
		t.Fatalf("wager visibility: %+v", v.Seats)
	}
	act(t, g, 2, games.Action{Type: "wager", Amount: 5})
	if g.phase != bkPoint {
		t.Fatalf("phase %s, want point", g.phase)
	}
	act(t, g, 0, games.Action{Type: "roll"})
	act(t, g, 1, games.Action{Type: "roll"})
	act(t, g, 2, games.Action{Type: "roll"})
	if g.chips[0] != 21 || g.chips[1] != 24 || g.chips[2] != 15 {
		t.Fatalf("chips %v, want [21 24 15]", g.chips)
	}
	if g.banker != 1 || g.phase != bkWager {
		t.Fatalf("banker %d phase %s, want 1 wager", g.banker, g.phase)
	}
}

func TestKlondikeGoldrushRerollAndBankerLast(t *testing.T) {
	env := games.NewTestEnv(
		1, 2, 3, 4, 6, // seat 1 rolls 6 high
		6, 6, // re-rolls dice 0 and 1 → 6 6 3 4 6: three 6s
		2, 2, 5, 5, 1, // banker: two pair
		5, // banker re-rolls die 4 → full house 5s over 2s
	)
	g := newKlondike(env, 2, true, 20, 1)
	act(t, g, 1, games.Action{Type: "wager", Amount: 5})
	if g.phase != bkPlay || g.cur() != 1 {
		t.Fatalf("goldrush should go straight to players; phase %s cur %d", g.phase, g.cur())
	}
	act(t, g, 1, games.Action{Type: "roll"})
	if err := g.Act(1, games.Action{Type: "reroll", Dice: []int{0, 1, 2, 3}}); err == nil {
		t.Fatal("re-rolling four dice should be rejected")
	}
	act(t, g, 1, games.Action{Type: "reroll", Dice: []int{0, 1}})
	if g.hand[1].Cat != games.ThreeKind || g.phase != bkBank {
		t.Fatalf("hand %s phase %s", g.hand[1].Name, g.phase)
	}
	act(t, g, 0, games.Action{Type: "roll"})
	if g.phase != bkDecide {
		t.Fatalf("phase %s, want decide", g.phase)
	}
	if err := g.Act(0, games.Action{Type: "reroll", Dice: []int{4}}); err == nil {
		t.Fatal("banker must wait for the table's decisions")
	}
	act(t, g, 1, games.Action{Type: "stay"})
	act(t, g, 0, games.Action{Type: "reroll", Dice: []int{4}})
	if g.chips[0] != 25 || g.chips[1] != 15 {
		t.Fatalf("chips %v, want [25 15]", g.chips)
	}
	if g.last == nil || g.last.BankHand != "Full house, 5s over 2s" {
		t.Fatalf("last %+v", g.last)
	}
}

func TestKlondikeGoldrushDoubleAndFold(t *testing.T) {
	env := games.NewTestEnv(
		6, 6, 6, 2, 1, // seat 1: three 6s
		2, 3, 4, 5, 1, // seat 2: low straight
		3, 3, 1, 2, 5, // seat 3: pair of 3s
		4, 4, 4, 1, 2, // banker: three 4s
		6, 6, // banker re-rolls dice 3, 4 → full house 4s over 6s
	)
	g := newKlondike(env, 4, true, 20, 1)
	g.chips[3] = 5
	act(t, g, 1, games.Action{Type: "wager", Amount: 3})
	act(t, g, 2, games.Action{Type: "wager", Amount: 4})
	act(t, g, 3, games.Action{Type: "wager", Amount: 3})
	for s := 1; s <= 3; s++ {
		act(t, g, s, games.Action{Type: "roll"})
		act(t, g, s, games.Action{Type: "stand"})
	}
	act(t, g, 0, games.Action{Type: "roll"})
	if w := g.Waiting(); len(w) != 3 {
		t.Fatalf("waiting %v, want all three players", w)
	}
	if err := g.Act(3, games.Action{Type: "double"}); err == nil {
		t.Fatal("seat 3 has 5 chips and can't cover doubling 3")
	}
	for _, a := range g.Legal(3) {
		if a.Type == "double" {
			t.Fatal("double listed as legal without the chips")
		}
	}
	act(t, g, 1, games.Action{Type: "double"})
	// Choices stay hidden from others until everyone has decided.
	if v := g.View(2).(bankerView); v.Seats[1].Choice != "" || !v.Seats[1].Decided {
		t.Fatalf("seat 1's choice leaked: %+v", v.Seats[1])
	}
	act(t, g, 2, games.Action{Type: "stay"})
	act(t, g, 3, games.Action{Type: "fold"})
	// Fold: seat 3 pays half of 3, rounded up = 2, straight away.
	if g.phase != bkBank || g.chips[3] != 3 || g.chips[0] != 22 || g.wager[1] != 6 {
		t.Fatalf("phase %s chips %v wagers %v", g.phase, g.chips, g.wager)
	}
	act(t, g, 0, games.Action{Type: "reroll", Dice: []int{3, 4}})
	// Full house beats three 6s (seat 1 loses 6) but not a low straight
	// (seat 2 wins 4).
	if g.chips[1] != 14 || g.chips[2] != 24 || g.chips[3] != 3 || g.chips[0] != 24 {
		t.Fatalf("chips %v, want [24 14 24 3]", g.chips)
	}
	if g.last.Net != 4 {
		t.Fatalf("bank net %d, want 4", g.last.Net)
	}
}

func TestKlondikeEveryoneFolds(t *testing.T) {
	g := newKlondike(games.NewTestEnv(1, 1, 2, 3, 4, 6, 6, 6, 6, 6), 2, true, 20, 1)
	act(t, g, 1, games.Action{Type: "wager", Amount: 5})
	act(t, g, 1, games.Action{Type: "roll"})
	act(t, g, 1, games.Action{Type: "stand"})
	act(t, g, 0, games.Action{Type: "roll"})
	act(t, g, 1, games.Action{Type: "fold"})
	if g.chips[1] != 17 || g.chips[0] != 23 || g.banker != 1 {
		t.Fatalf("chips %v banker %d", g.chips, g.banker)
	}
}

func TestKlondikeBustAndRotationEnd(t *testing.T) {
	env := games.NewTestEnv(6, 6, 6, 6, 6, 1, 2, 3, 4, 6)
	g := newKlondike(env, 2, false, 20, 1)
	g.chips[1] = 3
	act(t, g, 1, games.Action{Type: "wager", Amount: 3})
	if l := g.Legal(1); len(l) != 0 {
		t.Fatalf("seat 1 already bet, legal %v", l)
	}
	act(t, g, 0, games.Action{Type: "roll"})
	act(t, g, 1, games.Action{Type: "roll"})
	if !g.Over() || len(g.Winners()) != 1 || g.Winners()[0] != 0 {
		t.Fatalf("expected seat 0 to win after seat 1 busted; over=%v winners=%v chips=%v", g.Over(), g.Winners(), g.chips)
	}
}

func TestKlondikeRemoveBankerVoidsRound(t *testing.T) {
	g := newKlondike(games.NewTestEnv(), 3, false, 20, 1)
	act(t, g, 1, games.Action{Type: "wager", Amount: 2})
	g.Remove(0)
	if g.banker != 1 || g.phase != bkWager || g.chips[1] != 20 {
		t.Fatalf("banker %d phase %s chips %v", g.banker, g.phase, g.chips)
	}
	// Removing the last player we wait on for a bet moves the round on.
	g2 := newKlondike(games.NewTestEnv(), 3, false, 20, 1)
	act(t, g2, 1, games.Action{Type: "wager", Amount: 2})
	g2.Remove(2)
	if g2.phase != bkPoint {
		t.Fatalf("phase %s, want point", g2.phase)
	}
}

// ── Hundreds ────────────────────────────────────────────────────────────────

func playHundredsRound(t *testing.T, g *hundreds, squares map[int][]int) {
	t.Helper()
	next := map[int]int{}
	for i := 0; i < 3*len(g.order); i++ {
		s := g.cur()
		act(t, g, s, games.Action{Type: "roll"})
		act(t, g, s, games.Action{Type: "place", Index: squares[s][next[s]]})
		next[s]++
	}
}

func TestHundredsClassicHighAndLow(t *testing.T) {
	// Seat 0 builds 6 5 4 = 654, seat 1 builds 2 3 1 = 231 (rolls alternate).
	env := games.NewTestEnv(6, 2, 5, 3, 4, 1)
	g := newHundreds(env, 2, false, 0)
	order := map[int][]int{0: {0, 1, 2}, 1: {0, 1, 2}}
	playHundredsRound(t, g, order)
	if g.tokens[0] != 2 || g.tokens[1] != 3 || g.last.Values[0] != 654 || g.last.Values[1] != 231 {
		t.Fatalf("tokens %v last %+v", g.tokens, g.last)
	}
	if g.starter != 1 || g.cur() != 1 {
		t.Fatalf("next round should start with seat 1, got starter %d", g.starter)
	}
	// Round 2: seat 1 declares low; seat 1 builds 1 1 1, seat 0 builds 222.
	env2 := games.NewTestEnv(1, 2, 1, 2, 1, 2)
	g.env = env2
	act(t, g, 1, games.Action{Type: "low"})
	if err := g.Act(1, games.Action{Type: "low"}); err == nil {
		t.Fatal("declaring low twice should fail")
	}
	act(t, g, 1, games.Action{Type: "place", Index: 0})
	if l := g.Legal(0); len(l) != 1 || l[0].Type != "roll" {
		t.Fatalf("low already declared, legal %v", l)
	}
	if err := g.Act(0, games.Action{Type: "place", Index: 0}); err == nil {
		t.Fatal("placing before rolling should fail")
	}
	act(t, g, 0, games.Action{Type: "roll"})
	act(t, g, 0, games.Action{Type: "place", Index: 0})
	if err := g.Act(1, games.Action{Type: "roll"}); err != nil {
		t.Fatal(err)
	}
	if err := g.Act(1, games.Action{Type: "place", Index: 0}); err == nil {
		t.Fatal("filled square should be rejected")
	}
	act(t, g, 1, games.Action{Type: "place", Index: 1})
	for _, s := range []int{0, 1, 0} {
		act(t, g, s, games.Action{Type: "roll"})
		act(t, g, s, games.Action{Type: "place", Index: g.Legal(s)[0].Index})
	}
	if g.tokens[1] != 2 || g.tokens[0] != 2 || !g.last.Low {
		t.Fatalf("low round: tokens %v last %+v", g.tokens, g.last)
	}
}

func TestHundredsClassicWin(t *testing.T) {
	g := newHundreds(games.NewTestEnv(6, 1, 6, 1, 6, 1), 2, false, 0)
	g.tokens[0] = 1
	playHundredsRound(t, g, map[int][]int{0: {0, 1, 2}, 1: {0, 1, 2}})
	if !g.Over() || g.Winners()[0] != 0 {
		t.Fatalf("seat 0 should win; tokens %v", g.tokens)
	}
}

func TestHundredsPostman(t *testing.T) {
	// Seat 1 has the worst number and takes a token from the pool.
	g := newHundreds(games.NewTestEnv(6, 1, 6, 1, 6, 1), 2, true, 0)
	if g.pool != 6 || g.tokens[0] != 0 {
		t.Fatalf("postman start: pool %d tokens %v", g.pool, g.tokens)
	}
	playHundredsRound(t, g, map[int][]int{0: {0, 1, 2}, 1: {0, 1, 2}})
	if g.pool != 5 || g.tokens[1] != 1 || g.Over() {
		t.Fatalf("pool %d tokens %v", g.pool, g.tokens)
	}
	// The last token leaves the pool: seat 0 has none and wins at once.
	g.pool = 1
	g.env = games.NewTestEnv(1, 6, 1, 6, 1, 6)
	playHundredsRound(t, g, map[int][]int{0: {0, 1, 2}, 1: {0, 1, 2}})
	if !g.Over() || len(g.Winners()) != 1 || g.Winners()[0] != 0 {
		t.Fatalf("pool %d tokens %v winners %v", g.pool, g.tokens, g.Winners())
	}
}

func TestHundredsTieRollsOff(t *testing.T) {
	// Both build 666; roll-off 2 vs 5 → seat 1 discards.
	g := newHundreds(games.NewTestEnv(6, 6, 6, 6, 6, 6, 2, 5), 2, false, 0)
	playHundredsRound(t, g, map[int][]int{0: {0, 1, 2}, 1: {0, 1, 2}})
	if g.tokens[1] != 2 || g.tokens[0] != 3 {
		t.Fatalf("tokens %v", g.tokens)
	}
}

func TestHundredsRemoveCurrent(t *testing.T) {
	g := newHundreds(games.NewTestEnv(4), 3, false, 0)
	act(t, g, 0, games.Action{Type: "roll"})
	g.Remove(0)
	if g.cur() != 1 || g.die != 0 {
		t.Fatalf("cur %d die %d", g.cur(), g.die)
	}
}

// ── Three Hands ─────────────────────────────────────────────────────────────

func TestThreeHandsRound(t *testing.T) {
	// Seat 0 fills hand 1 with 6 6 6, hand 2 with 1 2 4, hand 3 with 5 5 1.
	// Seat 1 fills hand 1 with 2 3 4, hand 2 with 1 2 5, hand 3 with 5 5 1.
	p0 := []int{6, 6, 6, 1, 2, 4, 5, 5, 1}
	p1 := []int{2, 3, 4, 1, 2, 5, 5, 5, 1}
	faces := []int{}
	for i := range p0 {
		faces = append(faces, p0[i], p1[i])
	}
	g := newThreeHands(games.NewTestEnv(faces...), 3, 0)
	for i := 0; i < 9; i++ {
		for s := 0; s < 2; s++ {
			if g.cur != s {
				t.Fatalf("turn %d: cur %d want %d", i, g.cur, s)
			}
			act(t, g, s, games.Action{Type: "roll"})
			act(t, g, s, games.Action{Type: "place", Index: i / 3})
		}
	}
	// Hand 1: trips beats straight (0). Hand 2: 5-high beats 4-high (1).
	// Hand 3: tie.  1–1 → drawn round, replayed with seat 1 starting.
	if g.last.Results != [3]int{0, 1, -1} || g.last.Winner != -1 || g.wins != [2]int{0, 0} {
		t.Fatalf("results %v winner %d wins %v", g.last.Results, g.last.Winner, g.wins)
	}
	if g.round != 2 || g.cur != 1 {
		t.Fatalf("round %d cur %d", g.round, g.cur)
	}
}

func TestThreeHandsMatch(t *testing.T) {
	g := newThreeHands(games.NewTestEnv(), 3, 0)
	g.wins[0] = 1
	for a := 0; a < 3; a++ {
		g.areas[0][a] = []int{6, 6, 6}
		g.areas[1][a] = []int{1, 2, 4}
	}
	g.areas[0][2] = []int{6, 6, 0}
	g.die = 6
	if err := g.Act(0, games.Action{Type: "place", Index: 0}); err == nil {
		t.Fatal("placing in a full hand should fail")
	}
	act(t, g, 0, games.Action{Type: "place", Index: 2})
	if !g.Over() || g.Winners()[0] != 0 {
		t.Fatalf("seat 0 should win the match; wins %v", g.wins)
	}
}

func TestThreeHandsFiveDice(t *testing.T) {
	g := newThreeHands(games.NewTestEnv(), 5, 1)
	if len(g.areas[0][0]) != 5 || g.cur != 1 {
		t.Fatalf("five-dice areas %v cur %d", g.areas[0][0], g.cur)
	}
	if err := g.Act(0, games.Action{Type: "roll"}); err != games.ErrNotYourTurn {
		t.Fatalf("got %v", err)
	}
}

// ── Rule Maker ──────────────────────────────────────────────────────────────

func TestRuleMakerRoundAndVote(t *testing.T) {
	g := newRuleMaker(games.NewTestEnv(3, 6, 1), 3)
	act(t, g, 0, games.Action{Type: "roll"})
	act(t, g, 1, games.Action{Type: "roll"})
	act(t, g, 2, games.Action{Type: "roll"})
	if g.phase != rmPropose || g.proposer != 1 || g.scores[1] != 6 {
		t.Fatalf("phase %s proposer %d scores %v", g.phase, g.proposer, g.scores)
	}
	if err := g.Act(1, games.Action{Type: "propose", Text: "   "}); err == nil {
		t.Fatal("blank rule should be rejected")
	}
	long := "<b>{P0}</b>\n" + strings.Repeat("x", 300)
	act(t, g, 1, games.Action{Type: "propose", Text: long})
	if n := len([]rune(g.proposal)); n != ruleMaxText || strings.ContainsAny(g.proposal, "{}\n") {
		t.Fatalf("proposal not cleaned: %q (%d)", g.proposal, n)
	}
	act(t, g, 0, games.Action{Type: "vote", Yes: true})
	if v := g.View(2).(ruleMakerView); v.VotesCast != 1 || v.MyVote != voteNone || v.LastVote != nil {
		t.Fatalf("vote view leaked or wrong: %+v", v)
	}
	if err := g.Act(0, games.Action{Type: "vote", Yes: false}); err == nil {
		t.Fatal("double vote should fail")
	}
	act(t, g, 1, games.Action{Type: "vote", Yes: false})
	act(t, g, 2, games.Action{Type: "vote", Yes: true})
	if len(g.rules) != 1 || !g.lastVote.Adopted || g.phase != rmRoll || g.cur() != 1 {
		t.Fatalf("rules %v last %+v phase %s cur %d", g.rules, g.lastVote, g.phase, g.cur())
	}
}

func TestRuleMakerTieVoteFailsAndWin(t *testing.T) {
	g := newRuleMaker(games.NewTestEnv(2, 6), 2)
	act(t, g, 0, games.Action{Type: "roll"})
	act(t, g, 1, games.Action{Type: "roll"})
	act(t, g, 1, games.Action{Type: "propose", Text: "Sixes are wild"})
	act(t, g, 0, games.Action{Type: "vote", Yes: false})
	act(t, g, 1, games.Action{Type: "vote", Yes: true})
	if len(g.rules) != 0 || g.lastVote.Adopted {
		t.Fatal("a 1–1 vote is not a majority")
	}
	g.scores[1] = 45
	g.env = games.NewTestEnv(5)
	act(t, g, 1, games.Action{Type: "roll"})
	if !g.Over() || g.Winners()[0] != 1 {
		t.Fatalf("seat 1 should win at %d", g.scores[1])
	}
}

func TestRuleMakerRemovals(t *testing.T) {
	g := newRuleMaker(games.NewTestEnv(1, 6, 2, 3), 4)
	for s := 0; s < 4; s++ {
		act(t, g, s, games.Action{Type: "roll"})
	}
	g.Remove(1) // the proposer
	if g.phase != rmRoll || g.round != 2 {
		t.Fatalf("phase %s round %d", g.phase, g.round)
	}
	g.phase, g.proposer, g.proposal = rmVote, 0, "x"
	act(t, g, 0, games.Action{Type: "vote", Yes: true})
	act(t, g, 3, games.Action{Type: "vote", Yes: true})
	g.Remove(2) // the only voter left
	if g.phase != rmRoll || len(g.rules) != 1 {
		t.Fatalf("phase %s rules %v", g.phase, g.rules)
	}
}
