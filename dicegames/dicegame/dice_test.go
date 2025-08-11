package game

import (
	"math/rand"
	"testing"
)

// For dice randomness seed stability in tests
func init() {
	rand.Seed(42)
}

// Test that a single die produces a random value in 1..6
func TestDiceRollRange(t *testing.T) {
	die := NewDice()
	die.StartRoll()
	maxFrames := 1000
	for frame := 0; die.Animating && frame < maxFrames; frame++ {
		if frame%10 == 0 {
			t.Logf("Frame %d, die value=%d animating=%v", frame, die.Value, die.Animating)
		}
		die.Update()
	}
	if die.Animating {
		t.Fatalf("Die still animating after %d frames", maxFrames)
	}
	if die.Value < 1 || die.Value > 6 {
		t.Errorf("Die value out of range: got %d, want 1..6", die.Value)
	}
}

// Test that a DicePool can roll all dice and they land with values in range
func TestDicePoolRolls(t *testing.T) {
	pool := NewDicePool(5, 400, 580, 400, 0, 800, 600)
	pool.StartRoll()
	maxFrames := 1000
	for frame := 0; pool.Rolling && frame < maxFrames; frame++ {
		if frame%10 == 0 {
			t.Logf("Frame %d: checking dice states", frame)
			for i, d := range pool.Dice {
				t.Logf("  Die %d: Value=%d Selected=%t Animating=%v Pos=(%.1f,%.1f)", i, d.Value, d.Selected, d.Animating, d.X, d.Y)
			}
		}
		pool.Update()
	}
	if pool.Rolling {
		t.Fatalf("Pool still rolling after %d frames, possible hang", maxFrames)
	}
	for i, die := range pool.Dice {
		if die.Value < 1 || die.Value > 6 {
			t.Errorf("Die %d: value out of range: got %d", i, die.Value)
		}
	}
}

// Test dice collision by putting two dice overlapping and updating pool
func TestDiceCollisionResolution(t *testing.T) {
	d1 := NewDice()
	d2 := NewDice()
	d1.X, d1.Y, d1.Size = 110, 110, 40
	d2.X, d2.Y, d2.Size = 90, 110, 40 // overlap
	d1.VX, d1.VY = -1.0, 0            // move toward left
	d2.VX, d2.VY = 1.0, 0             // move toward right

	pool := &DicePool{Dice: []*Dice{d1, d2}, Rolling: true}
	pool.Update()

	if d1.VX > 0 && d2.VX < 0 { // swapped relative to initial
		// Bounced correctly
	} else {
		t.Errorf("Dice did not bounce: d1.VX=%f, d2.VX=%f", d1.VX, d2.VX)
	}
}

// Test dice selection and keeping mechanics
func TestDicePoolSelection(t *testing.T) {
	pool := NewDicePool(3, 400, 580, 400, 0, 800, 600)
	pool.StartRoll()
	maxFrames := 1000
	for frame := 0; pool.Rolling && frame < maxFrames; frame++ {
		if frame%10 == 0 {
			t.Logf("Frame %d: checking dice states", frame)
			for i, d := range pool.Dice {
				t.Logf("  Die %d: Value=%d Selected=%t Animating=%v Pos=(%.1f,%.1f)", i, d.Value, d.Selected, d.Animating, d.X, d.Y)
			}
		}
		pool.Update()
	}
	if pool.Rolling {
		t.Fatalf("Pool still rolling after %d frames, possible hang", maxFrames)
	}
	pool.ToggleKeep(1)
	if !pool.Dice[1].Selected {
		t.Errorf("Die 1 should be marked as kept.")
	}
	pool.Dice[0].Selected = true
	notKept := 0
	for _, d := range pool.Dice {
		if !d.Selected {
			notKept++
		}
	}
	if notKept != 1 {
		t.Errorf("Unexpected kept dice count: got %d not kept, want 1", notKept)
	}
}
