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
	finalValue       int // The value to land on at end of animation
	OffsetX, OffsetY int
	X, Y             float64 // location in play area
	VX, VY           float64 // velocity
	Size             float64
	Selected         bool // Dice is kept between rounds
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

	for i, d := range pool.Dice {
		d.X = pool.CupX + float64(i)*d.Size*1.1 - (float64(len(pool.Dice)-1)/2)*d.Size*1.1
		d.Y = pool.CupY

		angle := math.Pi/4 + rand.Float64()*(math.Pi/2) // 45°–135° upward
		speed := 5 + rand.Float64()*3
		d.VX = math.Cos(angle) * speed
		d.VY = -math.Sin(angle) * speed // Upwards
		d.Size = pool.DiceSize
		d.StartRoll() // Animates the die face
	}
}

func (pool *DicePool) Update() {
	if !pool.Rolling {
		return
	}

	// Move dice
	for _, d := range pool.Dice {
		d.Update()
		if d.Animating {
			d.X += d.VX
			d.Y += d.VY

			// Simple gravity
			d.VY += 0.2

			// Friction/drag
			d.VX *= 0.98
			d.VY *= 0.98

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

	pool.Rolling = false
}

func (pool *DicePool) DrawResults(screen *ebiten.Image) {
	x := ScreenWidth/2 - pool.DiceSize*float64(len(pool.Dice))/2
	y := 100
	for i, d := range pool.Dice {
		if d.Selected {
			d.Draw(screen, int(x)+i*int(pool.DiceSize*1.2), int(y), int(pool.DiceSize*1.5))
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
			borderCol := color.RGBA{220, 80, 80, 255}
			ebitenutil.DrawRect(screen, x-4, y-4, float64(size)+8, float64(size)+8, borderCol)
		} else {
			// Optionally dim or gray out the not-kept dice if desired
			// (Or just draw as normal)
		}

		d.Draw(screen, int(x), int(y), size)
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
	if idx < 0 || idx >= len(pool.Dice) {
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
}

func (pool *DicePool) ResetAll() {
	for _, d := range pool.Dice {
		d.Selected = false
	}
}
