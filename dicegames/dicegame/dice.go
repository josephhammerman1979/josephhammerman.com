package game

import (
	"fmt"
	"image/color"
	"math"
	"math/rand"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/text"
)

// Dice handles animation and value for a standard six-sided die.
type Dice struct {
	Value            int  // Displayed die face (1-6)
	Animating        bool // Is the die currently animating?
	animationTick    int
	animationFrames  int // How many frames does the animation run?
	faceChangeRate   int
	finalValue       int // The value to land on at end of animation
	OffsetX, OffsetY int
	X, Y             float64 // location in play area
	VX, VY           float64 // velocity
	Size             float64
	Selected         bool // Dice is kept between rounds
	Locked           bool // Previously kepy dice cannot usually ne rethrown
}

// NewDice creates a new Dice instance with default values.
func NewDice() *Dice {
	return &Dice{Value: 1}
}

// StartRoll begins a dice rolling animation. "finalValue" is the value to end on.
func (d *Dice) StartRoll() {
	d.Animating = true
	d.animationFrames = 90
	d.faceChangeRate = 3
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

	// Change face if tick aligns with speed
	if d.animationTick%d.faceChangeRate == 0 {
		d.Value = rand.Intn(6) + 1
	}

	// Apply rattling effect during animation
	d.OffsetX, d.OffsetY = 0, 0

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
	dx, dy := x, y
	if d.Animating {
		dx += d.OffsetX
		dy += d.OffsetY
	}
	// Draw die body
	ebitenutil.DrawRect(screen, float64(dx), float64(dy), float64(size), float64(size), color.White)
	// Black border
	borderColor := color.RGBA{80, 80, 80, 255}
	ebitenutil.DrawRect(screen, float64(dx), float64(dy), float64(size), 2, borderColor)        // Top
	ebitenutil.DrawRect(screen, float64(dx), float64(dy+size-2), float64(size), 2, borderColor) // Bottom
	ebitenutil.DrawRect(screen, float64(dx), float64(dy), 2, float64(size), borderColor)        // Left
	ebitenutil.DrawRect(screen, float64(dx+size-2), float64(dy), 2, float64(size), borderColor) // Right

	// Dot (pip) positions
	offset := size / 4
	r := float64(size / 10)
	pip := func(dxDot, dyDot int) {
		ebitenutil.DrawCircle(screen, float64(dx+dxDot), float64(dy+dyDot), r, color.Black)
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

type DicePool struct {
	Dice                                   []*Dice
	CupX, CupY                             float64 // Where dice will launch from
	PlayMinX, PlayMinY, PlayMaxX, PlayMaxY float64 // Play area bounds
	Rolling                                bool
	Released                               bool         // Have dice "landed"?
	Kept                                   map[int]bool // Which dice are kept for next roll
	DiceSize                               float64
}

func NewDicePool(count int, cupX, cupY, playMinX, playMinY, playMaxX, playMaxY float64) *DicePool {
	playAreaWidth := playMaxX - playMinX
	margin := 20.0
	maxDiceInRow := float64(count)
	diceSize := math.Min(80, (playAreaWidth-margin)/(maxDiceInRow+0.5))
	dice := make([]*Dice, count)
	for i := 0; i < count; i++ {
		dice[i] = NewDice()
		dice[i].Size = diceSize
	}

	return &DicePool{
		Dice: dice,
		CupX: cupX, CupY: cupY,
		PlayMinX: playMinX, PlayMinY: playMinY,
		PlayMaxX: playMaxX, PlayMaxY: playMaxY,
		Kept:     make(map[int]bool),
		DiceSize: diceSize,
	}
}

func (pool *DicePool) StartRoll() {
	pool.Rolling = true
	pool.Released = false

	// Launch ALL dice with a similar horizontal velocity and minor random scatter
	collectiveSpeed := 11 + rand.Float64()*2 // Base horizontal throw
	verticalSpeedBase := -14.0               // Slight lift, mostly horizontal

	angleRange := 0.25 // +/- ~15° cone of scatter

	for i, d := range pool.Dice {
		d.X = pool.CupX + float64(i)*d.Size*1.1 - (float64(len(pool.Dice)-1)/2)*d.Size*1.1
		d.Y = pool.CupY

		// Dice are launched together: strong rightward (+X) velocity, slight upward (-Y), mild angular scatter
		baseAngle := 0.0 // 0 radians = due right
		scatter := (rand.Float64() - 0.5) * angleRange
		angle := baseAngle + scatter

		d.VX = math.Cos(angle) * collectiveSpeed
		d.VY = verticalSpeedBase + math.Sin(angle)*2.5 // minor vertical randomness
		d.Size = pool.DiceSize
		d.StartRoll()
	}
}

func (pool *DicePool) Update() {
	if !pool.Rolling {
		return
	}

	allDone := true
	// Move dice
	for _, d := range pool.Dice {
		d.Update()
		if d.Animating {
			allDone = false
			d.X += d.VX
			d.Y += d.VY

			// Simple gravity
			d.VY += 0.08

			// Friction/drag
			d.VX *= 0.994
			d.VY *= 0.994

			// Wall bounce
			if d.X <= pool.PlayMinX {
				d.X = pool.PlayMinX
				d.VX = -d.VX * 0.8
			}
			if d.X+d.Size >= pool.PlayMaxX {
				d.X = pool.PlayMaxX - d.Size
				d.VX = -d.VX * 0.8
			}
			if d.Y+d.Size >= pool.PlayMaxY {
				d.Y = pool.PlayMaxY - d.Size
				d.VY = -d.VY * 0.7
				d.VX *= 0.96
			}
			if d.Y <= pool.PlayMinY {
				d.Y = pool.PlayMinY
				d.VY = -d.VY * 0.8
			}

			// Optionally, stop animating and bouncing after settled
			if math.Abs(d.VX)+math.Abs(d.VY) < 0.2 && !d.Animating {
				d.Animating = false
			}
		}
	}

	// Dice–dice collisions
	for i := 0; i < len(pool.Dice); i++ {
		for j := i + 1; j < len(pool.Dice); j++ {
			if diceCollide(pool.Dice[i], pool.Dice[j]) {
				resolveDiceCollision(pool.Dice[i], pool.Dice[j])
			}
		}
	}

	if allDone {
		pool.Rolling = false
	}
}

func drawRedOutline(screen *ebiten.Image, x, y, size int) {
	col := color.RGBA{220, 60, 60, 255}
	thickness := 3
	// Top
	ebitenutil.DrawRect(screen, float64(x-thickness), float64(y-thickness), float64(size+2*thickness), float64(thickness), col)
	// Bottom
	ebitenutil.DrawRect(screen, float64(x-thickness), float64(y+size), float64(size+2*thickness), float64(thickness), col)
	// Left
	ebitenutil.DrawRect(screen, float64(x-thickness), float64(y-thickness), float64(thickness), float64(size+2*thickness), col)
	// Right
	ebitenutil.DrawRect(screen, float64(x+size), float64(y-thickness), float64(thickness), float64(size+2*thickness), col)
}

func (pool *DicePool) DrawResults(screen *ebiten.Image) {
	size := int(pool.DiceSize * 1.2)
	count := len(pool.Dice)
	startX := pool.PlayMinX + 40
	startY := pool.PlayMinY + 110
	endX := pool.PlayMaxX - 40
	endY := pool.PlayMinY + pool.DiceSize*2 + 140

	positions := makeDiceInRow(count, startX, startY, endX, endY, float64(size))
	for i, d := range pool.Dice {
		x, y := positions[i][0], positions[i][1]
		d.Draw(screen, int(x), int(y), size)
		if d.Selected {
			drawRedOutline(screen, int(x), int(y), size)
		}
	}
}

func (p *DicePool) ResizeDiceToFit() {
	playAreaWidth := p.PlayMaxX - p.PlayMinX
	margin := 20.0
	diceSize := math.Min(80, (playAreaWidth-margin)/(float64(len(p.Dice))+0.5))
	for _, d := range p.Dice {
		d.Size = diceSize
	}
	p.DiceSize = diceSize
}

// PresentFinalResults draws all dice in a neat row, highlighting selected dice.
func (pool *DicePool) PresentFinalResults(screen *ebiten.Image) {
	count := len(pool.Dice)

	// Define row layout: full play area
	startX := pool.PlayMinX
	startY := pool.PlayMinY + 40
	endX := pool.PlayMaxX
	endY := pool.PlayMinY + pool.DiceSize*2 + 40 // Row near top

	positions := makeDiceInRow(count, startX, startY, endX, endY, pool.DiceSize*1.2)

	for i, d := range pool.Dice {
		x, y := positions[i][0], positions[i][1]

		size := int(pool.DiceSize * 1.2)
		if d.Selected {
			// If kept, shift upward and add border
			y -= float64(size) * 0.1
			// borderCol := color.RGBA{220, 80, 80, 255}
			//ebitenutil.DrawRect(screen, x-4, y-4, float64(size)+8, float64(size)+8, borderCol)
		}
		d.Draw(screen, int(x), int(y), size)
		if d.Selected {
			drawRedOutline(screen, int(x), int(y), size)
		}
	}

	resultY := int(endY + pool.DiceSize*1.3)
	for i, d := range pool.Dice {
		var col color.RGBA
		if d.Selected {
			col = color.RGBA{220, 80, 80, 255} // Highlight kept
		}
		// Center the number under the die: use dice size to offset for aesthetics if needed
		valStr := fmt.Sprintf("%d", d.Value)
		textX := int(positions[i][0] + pool.DiceSize*0.40)
		text.Draw(screen, valStr, RegularFont, textX, resultY, col)
	}
}

func resolveDiceCollision(a, b *Dice) {
	ax, ay := a.X+a.Size/2, a.Y+a.Size/2
	bx, by := b.X+b.Size/2, b.Y+b.Size/2
	dx, dy := bx-ax, by-ay
	dist := math.Hypot(dx, dy)
	minDist := (a.Size + b.Size) / 2

	if dist == 0 || dist >= minDist {
		return
	}

	// Always push apart (separate) the dice if they overlap
	overlap := minDist - dist
	nx, ny := dx/dist, dy/dist
	a.X -= nx * (overlap / 2)
	a.Y -= ny * (overlap / 2)
	b.X += nx * (overlap / 2)
	b.Y += ny * (overlap / 2)

	// Now compute relative velocity along the collision axis
	rvx := b.VX - a.VX
	rvy := b.VY - a.VY
	velAlongNormal := rvx*nx + rvy*ny

	// Only apply bounce impulse if moving toward each other
	if velAlongNormal < 0 {
		restitution := 0.8 // bounciness
		impulse := -(1 + restitution) * velAlongNormal / 2
		ix := impulse * nx
		iy := impulse * ny
		a.VX -= ix
		a.VY -= iy
		b.VX += ix
		b.VY += iy
	}
}

// makeDiceInRow lays out dice evenly in a horizontal row.
// Returns a slice of (x, y) positions for each die.
func makeDiceInRow(
	count int, // number of dice
	areaMinX, areaMinY, areaMaxX, areaMaxY float64, // bounds of play area
	diceSize float64, // (calculated based on area and count)
) [][2]float64 {
	var positions [][2]float64

	rowWidth := areaMaxX - areaMinX
	rowHeight := areaMaxY - areaMinY

	// Compute spacing
	totalDiceWidth := float64(count) * diceSize
	gap := (rowWidth - totalDiceWidth) / float64(count+1)

	y := areaMinY + rowHeight/2 - diceSize/2 // Centered vertically

	for i := 0; i < count; i++ {
		// Dice will be laid out: gap | die | gap | die | ...etc
		x := areaMinX + gap + float64(i)*(diceSize+gap)
		positions = append(positions, [2]float64{x, y})
	}
	return positions
}

func (pool *DicePool) ToggleKeep(idx int) {
	if idx < 0 || idx >= len(pool.Dice) || pool.Dice[idx].Locked {
		return
	}
	pool.Dice[idx].Selected = !pool.Dice[idx].Selected
}

func (pool *DicePool) RerollUnkeptFromCup() {
	for _, d := range pool.Dice {
		if !d.Selected {
			d.X = pool.CupX
			d.Y = pool.CupY
			d.VX = (rand.Float64()*2 - 1) * 5
			d.VY = -rand.Float64()*6 - 3
			d.StartRoll()
		}
	}
	pool.Rolling = true
	for _, d := range pool.Dice {
		if d.Selected {
			d.Locked = true
		}
	}

}

func (pool *DicePool) ResetAll() {
	for _, d := range pool.Dice {
		d.Selected = false
	}
}
