package game

import (
	"image/color"
	"math"
	"math/rand"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

// Dice handles animation and value for a standard six-sided die.
type Dice struct {
	Value            int  // Displayed die face (1-6)
	Animating        bool // Is the die currently animating?
	animationTick    int
	animationFrames  int // How many frames does the animation run?
	finalValue       int // The value to land on at end of animation
	OffsetX, OffsetY int
	X, Y             float64 // location in play area
	VX, VY           float64 // velocity
	Size             float64
}

// NewDice creates a new Dice instance with default values.
func NewDice() *Dice {
	return &Dice{Value: 1}
}

// StartRoll begins a dice rolling animation. "finalValue" is the value to end on.
func (d *Dice) StartRoll() {
	d.Animating = true
	d.animationFrames = 40
	d.animationTick = 0
	d.finalValue = rand.Intn(6) + 1
}

func diceCollide(a, b *Dice) bool {
	dx := (a.X + a.Size/2) - (b.X + b.Size/2)
	dy := (a.Y + a.Size/2) - (b.Y + b.Size/2)
	distance := math.Hypot(dx, dy)
	minDist := a.Size/2 + b.Size/2
	return distance < minDist
}

// Update advances the animation if necessary and ends with the final value.
func (d *Dice) Update() {
	if !d.Animating {
		d.OffsetX, d.OffsetY = 0, 0
		return
	}

	updateSpeed := 1
	switch progress := float64(d.animationTick) / float64(d.animationFrames); {
	case progress < 0.33:
		updateSpeed = 1 // Fast updates initially
	case progress < 0.66:
		updateSpeed = 2 // Slowing down
	default:
		updateSpeed = 3 // Final few face changes slowest
	}

	// Change face if tick aligns with speed
	if d.animationTick%updateSpeed == 0 {
		d.Value = rand.Intn(6) + 1
	}

	// Apply rattling effect during animation
	maxOffset := 10 // You can fine-tune this
	d.OffsetX = rand.Intn(maxOffset*2+1) - maxOffset
	d.OffsetY = rand.Intn(maxOffset*2+1) - maxOffset

	// Advance animation tick
	d.animationTick++

	// End of animation
	if d.animationTick >= d.animationFrames {
		d.Value = d.finalValue
		d.Animating = false
		d.OffsetX, d.OffsetY = 0, 0
	}
}

// Draw renders the die at (x, y) with given size.
func (d *Dice) Draw(screen *ebiten.Image, x, y, size int) {
	// Draw die body
	ebitenutil.DrawRect(screen, float64(x), float64(y), float64(size), float64(size), color.White)
	// Black border
	borderColor := color.RGBA{80, 80, 80, 255}
	ebitenutil.DrawRect(screen, float64(x), float64(y), float64(size), 2, borderColor)        // Top
	ebitenutil.DrawRect(screen, float64(x), float64(y+size-2), float64(size), 2, borderColor) // Bottom
	ebitenutil.DrawRect(screen, float64(x), float64(y), 2, float64(size), borderColor)        // Left
	ebitenutil.DrawRect(screen, float64(x+size-2), float64(y), 2, float64(size), borderColor) // Right

	// Dot (pip) positions
	offset := size / 4
	r := float64(size / 10)
	pip := func(dx, dy int) {
		ebitenutil.DrawCircle(screen, float64(x+dx), float64(y+dy), r, color.Black)
	}
	center := size / 2
	// Standard die pip layouts
	switch d.Value {
	case 1:
		pip(center, center)
	case 2:
		pip(offset, offset)
		pip(size-offset, size-offset)
	case 3:
		pip(center, center)
		pip(offset, offset)
		pip(size-offset, size-offset)
	case 4:
		pip(offset, offset)
		pip(size-offset, offset)
		pip(offset, size-offset)
		pip(size-offset, size-offset)
	case 5:
		pip(center, center)
		pip(offset, offset)
		pip(size-offset, offset)
		pip(offset, size-offset)
		pip(size-offset, size-offset)
	case 6:
		pip(offset, offset)
		pip(size-offset, offset)
		pip(offset, size-offset)
		pip(size-offset, size-offset)
		pip(offset, center)
		pip(size-offset, center)
	}
}
