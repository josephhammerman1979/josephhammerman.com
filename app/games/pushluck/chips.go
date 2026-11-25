package pushluck

import (
	"fmt"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

// chipTable applies the shared chip rules (app/games/chips.go) to the
// hand-by-hand pot games, Threes and 1-4-24: stacks, a pot that tracks who
// put in what, and the between-hands rebuy decision for busted players.
type chipTable struct {
	St    games.Stacks
	Pot   games.Pot
	Rebuy []bool // busted players still deciding to buy back in or leave
}

var chipsOption = games.Option{Key: "chips", Label: "Starting chips", Choices: []games.Choice{
	{Value: "20", Label: "20 chips"}, {Value: "50", Label: "50 chips"},
}}

func newChipTable(n int, opts map[string]string) chipTable {
	return chipTable{
		St:    games.NewStacks(n, atoiDefault(opts["chips"], 20, 20, 50)),
		Pot:   games.NewPot(n),
		Rebuy: make([]bool, n),
	}
}

// canPlay reports whether seat is at the table with chips to play.
func (c *chipTable) canPlay(seat int, s *games.Seats) bool {
	return s.In(seat) && c.St.Funded(seat)
}

// playersFrom lists the seats that can play, going round from first.
func (c *chipTable) playersFrom(first int, s *games.Seats) []int {
	out := []int{}
	for _, seat := range s.OrderFrom(first) {
		if c.canPlay(seat, s) {
			out = append(out, seat)
		}
	}
	return out
}

// ante puts amount in the pot from each seat; anyone short goes all-in.
func (c *chipTable) ante(seats []int, amount int) {
	for _, s := range seats {
		c.St.PayInto(&c.Pot, s, amount)
	}
}

// award gives winner the pot and logs anything an all-in winner can't take.
func (c *chipTable) award(env *games.Env, winner int) int {
	won := c.Pot.Award(&c.St, winner)
	if left := c.Pot.Total(); left > 0 {
		env.Log(games.LogInfo, "%s was all-in, so %d stays in the pot.", games.P(winner), left)
	}
	return won
}

// markBusted flags every busted player for the rebuy decision and reports
// whether there are any.
func (c *chipTable) markBusted(env *games.Env, s *games.Seats) bool {
	any := false
	for _, seat := range s.Active() {
		if c.St.Busted(seat) {
			c.Rebuy[seat] = true
			any = true
			env.Log(games.LogBust, "%s is busted.", games.P(seat))
		}
	}
	return any
}

func (c *chipTable) rebuying() bool {
	for _, r := range c.Rebuy {
		if r {
			return true
		}
	}
	return false
}

func (c *chipTable) waiting() []int {
	out := []int{}
	for s, r := range c.Rebuy {
		if r {
			out = append(out, s)
		}
	}
	return out
}

// decide applies a busted player's rebuy / leave choice.
func (c *chipTable) decide(env *games.Env, seat int, a games.Action) error {
	if !c.Rebuy[seat] {
		return games.Errorf("Waiting for busted players to buy back in or sit out.")
	}
	switch a.Type {
	case "rebuy":
		c.St.Rebuy(seat)
		env.Log(games.LogInfo, "%s bought back in for %d.", games.P(seat), c.St.Start)
	case "leave":
		c.St.Gone[seat] = true
		env.Log(games.LogInfo, "%s is sitting out the rest of the game.", games.P(seat))
	default:
		return games.ErrUnknown
	}
	c.Rebuy[seat] = false
	return nil
}

func (c *chipTable) legal(seat int) []games.Action {
	if !c.Rebuy[seat] {
		return nil
	}
	return append([]games.Action{}, games.RebuyActions...)
}

// finish returns any unclaimed pot and picks the winner on net results.
func (c *chipTable) finish(env *games.Env, s *games.Seats) int {
	if left := c.Pot.Total(); left > 0 {
		env.Log(games.LogInfo, "The unclaimed pot of %d is returned.", left)
		c.Pot.Refund(&c.St)
	}
	w := pickWinner(env, games.BestSeats(s, c.St.Nets(), false))
	env.Log(games.LogWin, "%s wins the game, %s net!", games.P(w), signed(c.St.Net(w)))
	return w
}

// badge is a seat's chip status for its video tile.
func (c *chipTable) badge(seat int) string {
	switch {
	case c.St.Gone[seat]:
		return "sat out"
	case c.St.Busted(seat):
		return "busted"
	}
	return fmt.Sprintf("%d chips", c.St.Chips[seat])
}

func (c *chipTable) rebuyPrompt(seat int) string {
	if seat >= 0 && c.Rebuy[seat] {
		return fmt.Sprintf("You're busted. Buy back in for %d chips, or sit out the rest of the game?", c.St.Start)
	}
	return "Waiting for busted players to buy back in or sit out."
}

// chipView is the chip part of a game's view.
type chipView struct {
	Start  int    `json:"start"`
	Chips  []int  `json:"chips"`
	BuyIns []int  `json:"buyIns"`
	Net    []int  `json:"net"`
	AllIn  []bool `json:"allIn"`
	Gone   []bool `json:"gone"`
	Rebuy  []bool `json:"rebuy"`
}

func (c *chipTable) view() chipView {
	return chipView{
		Start: c.St.Start, Chips: games.Ints(c.St.Chips), BuyIns: games.Ints(c.St.BuyIns),
		Net: c.St.Nets(), AllIn: append([]bool{}, c.Pot.AllIn...),
		Gone: append([]bool{}, c.St.Gone...), Rebuy: append([]bool{}, c.Rebuy...),
	}
}
