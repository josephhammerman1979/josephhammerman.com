package game

import "github.com/hajimehoshi/ebiten/v2"

type GameState int

const (
	StateLauncher GameState = iota
	StatePlayerCount
	StatePigGame
)

type Game interface {
	Update() error
	Draw(screen *ebiten.Image)
	Layout(outsideWidth, outsideHeight int) (int, int)
}

type GameManager struct {
	currentState GameState
	launcher     *Launcher
	playerCount  *PlayerCount
	pigGame      *PigGame
}

func NewGameManager() *GameManager {
	return &GameManager{
		currentState: StateLauncher,
		launcher:     NewLauncher(),
	}
}

func (gm *GameManager) Update() error {
	switch gm.currentState {
	case StateLauncher:
		return gm.launcher.Update(gm)
	case StatePlayerCount:
		return gm.playerCount.Update(gm)
	case StatePigGame:
		return gm.pigGame.Update()
	}
	return nil
}

func (gm *GameManager) Draw(screen *ebiten.Image) {
	switch gm.currentState {
	case StateLauncher:
		gm.launcher.Draw(screen)
	case StatePlayerCount:
		gm.playerCount.Draw(screen)
	case StatePigGame:
		gm.pigGame.Draw(screen)
	}
}

func (gm *GameManager) Layout(outsideWidth, outsideHeight int) (int, int) {
	return 800, 600
}
