package games

import "testing"

func TestStacksNeverGoNegative(t *testing.T) {
	s := NewStacks(2, 5)
	paid, allIn := s.Pay(0, 8)
	if paid != 5 || !allIn || s.Chips[0] != 0 {
		t.Fatalf("paid=%d allIn=%v chips=%d, want 5, true, 0", paid, allIn, s.Chips[0])
	}
	if paid, allIn := s.Pay(1, 5); paid != 5 || allIn {
		t.Fatalf("paying exactly your stack isn't all-in: paid=%d allIn=%v", paid, allIn)
	}
	if !s.Busted(0) || s.Funded(0) {
		t.Fatal("seat 0 should be busted")
	}
	s.Rebuy(0)
	if s.Chips[0] != 5 || s.Net(0) != -5 {
		t.Fatalf("after rebuy chips=%d net=%d, want 5, -5", s.Chips[0], s.Net(0))
	}
}

func TestAllInWinnerOnlyWinsTheirStakeFromEach(t *testing.T) {
	s := NewStacks(3, 20)
	p := NewPot(3)
	s.Chips[0] = 2
	s.PayInto(&p, 0, 6) // short: 2 in, all-in
	s.PayInto(&p, 1, 6)
	s.PayInto(&p, 2, 6)
	won := p.Award(&s, 0)
	if won != 6 || s.Chips[0] != 6 {
		t.Fatalf("all-in winner took %d (chips %d), want 2 from each = 6", won, s.Chips[0])
	}
	if p.Total() != 8 || p.Contrib[1] != 4 || p.Contrib[2] != 4 {
		t.Fatalf("leftover pot = %v, want 4 and 4 left", p.Contrib)
	}
	// A covered winner takes the rest.
	won = p.Award(&s, 1)
	if won != 8 || p.Total() != 0 {
		t.Fatalf("second award took %d, pot %d", won, p.Total())
	}
}

func TestPotRefund(t *testing.T) {
	s := NewStacks(2, 10)
	p := NewPot(2)
	s.PayInto(&p, 0, 3)
	s.PayInto(&p, 1, 4)
	p.Refund(&s)
	if s.Chips[0] != 10 || s.Chips[1] != 10 || p.Total() != 0 {
		t.Fatalf("chips=%v pot=%d", s.Chips, p.Total())
	}
}
