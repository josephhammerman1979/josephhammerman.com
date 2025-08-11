package game

import "github.com/hajimehoshi/ebiten/v2"

// GameState tracks what screen the manager is displaying.
type GameState int

const (
	StateLauncher GameState = iota
	StatePlayerCount
	StateInGame
)

// Game is a generic interface for any ebiten game.
type Game interface {
	Update() error
	Draw(screen *ebiten.Image)
	Layout(outsideWidth, outsideHeight int) (int, int)
}

type GameManager struct {
	currentState GameState
	launcher     *Launcher
	playerCount  *PlayerCount

	// Track which game the user selected.
	selectedGame     string
	activeGame       Game // Holds the running game (MockGame, PigGame, etc.)
	requestedPlayers int  // For games needing a player count, e.g. Pig
}

// Construct test menu: add more games as needed.
func NewGameManager() *GameManager {
	return &GameManager{
		currentState: StateLauncher,
		launcher:     NewLauncher(),
	}
}

// Top-level update: dispatches to the current state's logic.
func (gm *GameManager) Update() error {
	switch gm.currentState {
	case StateLauncher:
		return gm.launcher.Update(gm)
	case StatePlayerCount:
		err := gm.playerCount.Update(gm)
		// Did we finish selection? Launch game
		if gm.playerCount.Done {
			gm.requestedPlayers = gm.playerCount.Value
			switch gm.selectedGame {
			case "Pig Dice Game":
				gm.activeGame = NewPigGame(gm.requestedPlayers)
			}
			gm.currentState = StateInGame
		}
		return err
	case StateInGame:
		if gm.activeGame != nil {
			return gm.activeGame.Update()
		}
	}
	return nil
}

func (gm *GameManager) Draw(screen *ebiten.Image) {
	switch gm.currentState {
	case StateLauncher:
		gm.launcher.Draw(screen)
	case StatePlayerCount:
		gm.playerCount.Draw(screen)
	case StateInGame:
		if gm.activeGame != nil {
			gm.activeGame.Draw(screen)
		}
	}
}

func (gm *GameManager) Layout(outsideWidth, outsideHeight int) (int, int) {
	if gm.activeGame != nil {
		return gm.activeGame.Layout(outsideWidth, outsideHeight)
	}
	return 800, 600
}
