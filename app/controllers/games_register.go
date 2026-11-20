package controllers

// The dice games register themselves with app/games when imported.
import (
	_ "github.com/josephhammerman1979/josephhammerman.com/app/games/bluff"
	_ "github.com/josephhammerman1979/josephhammerman.com/app/games/boards"
	_ "github.com/josephhammerman1979/josephhammerman.com/app/games/category"
	_ "github.com/josephhammerman1979/josephhammerman.com/app/games/pushluck"
)
