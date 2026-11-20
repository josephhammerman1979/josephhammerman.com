package games

import "fmt"

// Pig and Big Pig: push-your-luck to 100.
//
//   Pig:     one die; a 1 ends the turn and loses the turn's points.
//   Big Pig: two dice; a 1 on either die ends the turn and loses the turn's
//            points, but double ones score 25 and you keep rolling.
//
// Actions: {type:"roll"}, {type:"hold"} (after at least one roll).

func init() {
	Register(&Def{
		ID: "pig", Name: "Pig", Order: 10,
		Summary:    "One die, first to 100. Roll as long as you dare — a 1 wipes your turn.",
		MinPlayers: 2, MaxPlayers: 6,
		Rules: []string{
			"On your turn, roll as many times as you like; each roll adds to your turn total.",
			"Hold to bank your turn total.",
			"Roll a 1 and you lose the turn total and your turn ends.",
			"First to 100 banked points wins.",
		},
		Facts: []string{"Number of dice: 1", "Player count: 2-6", "Object: First to 100"},
		Sheet: []string{
			"Starting with the first player and proceeding to their left, each player takes their by throwing the die one or more times. After each throw their running total for the turn is increased as long as no one is thrown. If the player chooses to hold before throwing a one, they bank their current score. Throwing a one causes a player’s turn to end and any points they accumulated that turn to be lost.",
			"When a player reaches 100 or more points, the game ends.",
		},
		New: func(env *Env, n int, _ map[string]string) Game { return newPig(env, n, 1) },
	})
	Register(&Def{
		ID: "bigpig", Name: "Big Pig", Order: 20,
		Summary:    "Pig with two dice. A 1 on either die busts — but double ones are worth 25.",
		MinPlayers: 2, MaxPlayers: 6,
		Rules: []string{
			"Played like Pig, but you roll two dice and add them.",
			"A 1 on either die loses your turn total and ends your turn.",
			"Double ones score 25 and you keep rolling.",
			"First to 100 banked points wins.",
		},
		Facts: []string{"Number of dice: 2", "Player count: 2-6", "Object: First to 100"},
		Sheet: []string{
			"Played like Pig but with two dice. A one on either die causes a loss of your turn. Double ones are worth 25.",
		},
		New: func(env *Env, n int, _ map[string]string) Game { return newPig(env, n, 2) },
	})
}

const pigGoal = 100

type pig struct {
	env       *Env
	seats     Seats
	numDice   int
	cur       int
	totals    []int
	turnTotal int
	rolled    bool // current player has rolled this turn
	dice      []int
	winner    int
}

func newPig(env *Env, n, numDice int) *pig {
	return &pig{env: env, seats: NewSeats(n), numDice: numDice, totals: make([]int, n), winner: -1}
}

func (g *pig) Act(seat int, a Action) error {
	if g.winner >= 0 {
		return ErrGameOver
	}
	if seat != g.cur {
		return ErrNotYourTurn
	}
	switch a.Type {
	case "roll":
		g.roll()
	case "hold":
		if !g.rolled {
			return Errorf("Roll at least once before holding.")
		}
		g.hold()
	default:
		return ErrUnknown
	}
	return nil
}

func (g *pig) roll() {
	g.dice = g.env.Roll(g.numDice)
	g.rolled = true
	ones := Counts(g.dice)[1]
	who := P(g.cur)
	switch {
	case g.numDice == 2 && ones == 2:
		g.turnTotal += 25
		g.env.Log(LogGood, "%s rolled double ones: +25 (turn %d)", who, g.turnTotal)
	case ones > 0:
		g.env.Log(LogBust, "%s rolled %s — BUST, lost %d", who, DiceText(g.dice), g.turnTotal)
		g.turnTotal = 0
		g.nextTurn()
	default:
		gain := Sum(g.dice)
		g.turnTotal += gain
		g.env.Log(LogRoll, "%s rolled %s: +%d (turn %d)", who, DiceText(g.dice), gain, g.turnTotal)
	}
}

func (g *pig) hold() {
	g.totals[g.cur] += g.turnTotal
	if g.totals[g.cur] >= pigGoal {
		g.winner = g.cur
		g.env.Log(LogWin, "%s held +%d → %d and wins!", P(g.cur), g.turnTotal, g.totals[g.cur])
		g.turnTotal = 0
		return
	}
	g.env.Log(LogInfo, "%s held +%d → %d", P(g.cur), g.turnTotal, g.totals[g.cur])
	g.turnTotal = 0
	g.nextTurn()
}

func (g *pig) nextTurn() {
	g.cur = g.seats.Next(g.cur)
	g.turnTotal = 0
	g.rolled = false
}

func (g *pig) Legal(seat int) []Action {
	if g.winner >= 0 || seat != g.cur {
		return nil
	}
	out := []Action{{Type: "roll"}}
	if g.rolled {
		out = append(out, Action{Type: "hold"})
	}
	return out
}

func (g *pig) Remove(seat int) {
	g.seats.Out[seat] = true
	if seat == g.cur {
		g.nextTurn()
	}
}

type pigView struct {
	NumDice   int   `json:"numDice"`
	Dice      []int `json:"dice"`
	Current   int   `json:"current"`
	TurnTotal int   `json:"turnTotal"`
	Rolled    bool  `json:"rolled"`
	Totals    []int `json:"totals"`
	Goal      int   `json:"goal"`
}

func (g *pig) View(seat int) interface{} {
	return pigView{
		NumDice: g.numDice, Dice: Ints(g.dice), Current: g.cur, TurnTotal: g.turnTotal,
		Rolled: g.rolled, Totals: Ints(g.totals), Goal: pigGoal,
	}
}

func (g *pig) Prompt(seat int) string {
	if g.winner >= 0 {
		return fmt.Sprintf("%s wins with %d!", P(g.winner), g.totals[g.winner])
	}
	if seat != g.cur {
		if g.turnTotal > 0 {
			return fmt.Sprintf("%s has %d at risk.", P(g.cur), g.turnTotal)
		}
		return fmt.Sprintf("Waiting for %s to roll.", P(g.cur))
	}
	if !g.rolled {
		return "Roll to start your turn."
	}
	return fmt.Sprintf("%d at risk — roll again or hold.", g.turnTotal)
}

func (g *pig) Waiting() []int {
	if g.winner >= 0 {
		return nil
	}
	return []int{g.cur}
}

func (g *pig) Badges() []string {
	out := make([]string, len(g.totals))
	for i, t := range g.totals {
		out[i] = fmt.Sprint(t)
		if i == g.cur && g.turnTotal > 0 && g.winner < 0 {
			out[i] += fmt.Sprintf(" (+%d)", g.turnTotal)
		}
	}
	return out
}

func (g *pig) Over() bool { return g.winner >= 0 }

func (g *pig) Winners() []int {
	if g.winner < 0 {
		return nil
	}
	return []int{g.winner}
}
