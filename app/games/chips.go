package games

// Chip rules shared by the betting games (Klondike, Double or Quits, Threes,
// 1-4-24):
//
//   - Nobody goes negative.  A player who can't cover a payment pays what
//     they have and is all-in.
//   - An all-in player can only win, from each other player's share of a
//     pot, as much as they put in themselves; the rest stays in the pot.
//   - A player on zero chips is busted: they sit out the rest of the round,
//     then either buy back in for a fresh starting stack or sit out the
//     rest of the game.
//   - Standings are net of buy-ins: chips minus everything bought in.

// Stacks tracks each player's chips and buy-ins.
type Stacks struct {
	Start  int    `json:"start"`
	Chips  []int  `json:"chips"`
	BuyIns []int  `json:"buyIns"`
	Gone   []bool `json:"gone"` // chose to sit out the rest of the game after busting
}

// NewStacks gives n players start chips each (their first buy-in).
func NewStacks(n, start int) Stacks {
	s := Stacks{Start: start, Chips: make([]int, n), BuyIns: make([]int, n), Gone: make([]bool, n)}
	for i := range s.Chips {
		s.Chips[i] = start
		s.BuyIns[i] = 1
	}
	return s
}

// Pay takes up to amount from seat.  It returns what was actually paid and
// whether the player was short (all-in).
func (s *Stacks) Pay(seat, amount int) (paid int, allIn bool) {
	if amount <= 0 {
		return 0, false
	}
	paid = amount
	if paid > s.Chips[seat] {
		paid = s.Chips[seat]
	}
	s.Chips[seat] -= paid
	return paid, paid < amount
}

// PayInto pays up to amount from seat into pot, marking them all-in if short.
func (s *Stacks) PayInto(p *Pot, seat, amount int) int {
	paid, allIn := s.Pay(seat, amount)
	p.Add(seat, paid, allIn)
	return paid
}

// Busted reports whether seat has no chips and hasn't left the game.
func (s *Stacks) Busted(seat int) bool { return s.Chips[seat] <= 0 && !s.Gone[seat] }

// Funded reports whether seat can play: has chips and hasn't left.
func (s *Stacks) Funded(seat int) bool { return s.Chips[seat] > 0 && !s.Gone[seat] }

// Rebuy gives a busted seat a fresh starting stack.
func (s *Stacks) Rebuy(seat int) {
	s.Chips[seat] += s.Start
	s.BuyIns[seat]++
}

// Net is seat's result so far: chips minus everything bought in.
func (s *Stacks) Net(seat int) int { return s.Chips[seat] - s.Start*s.BuyIns[seat] }

// Nets lists Net for every seat.
func (s *Stacks) Nets() []int {
	out := make([]int, len(s.Chips))
	for i := range out {
		out[i] = s.Net(i)
	}
	return out
}

// Pot tracks what each player has put in, and who went all-in.
type Pot struct {
	Contrib []int  `json:"contrib"`
	AllIn   []bool `json:"allIn"`
}

func NewPot(n int) Pot { return Pot{Contrib: make([]int, n), AllIn: make([]bool, n)} }

// Add records seat putting amount in; allIn marks them as short.
func (p *Pot) Add(seat, amount int, allIn bool) {
	p.Contrib[seat] += amount
	if allIn {
		p.AllIn[seat] = true
	}
}

// Total is the size of the pot.
func (p *Pot) Total() int { return Sum(p.Contrib) }

// Award gives the pot to winner and returns what they took.  An all-in
// winner takes at most their own stake from each player's share; whatever
// they can't take stays in the pot (carried to the next round, or returned
// by the caller).
func (p *Pot) Award(s *Stacks, winner int) int {
	cap := p.Contrib[winner]
	won := 0
	for i, c := range p.Contrib {
		take := c
		if p.AllIn[winner] && take > cap {
			take = cap
		}
		p.Contrib[i] -= take
		won += take
	}
	s.Chips[winner] += won
	p.AllIn[winner] = false
	if p.Total() == 0 {
		for i := range p.AllIn {
			p.AllIn[i] = false
		}
	}
	return won
}

// Refund returns every player's remaining share and empties the pot.
func (p *Pot) Refund(s *Stacks) {
	for i, c := range p.Contrib {
		s.Chips[i] += c
		p.Contrib[i] = 0
		p.AllIn[i] = false
	}
}

// RebuyActions are the choices offered to a busted player between rounds:
// {type:"rebuy"} buys back in, {type:"leave"} sits out the rest of the game.
var RebuyActions = []Action{{Type: "rebuy"}, {Type: "leave"}}
