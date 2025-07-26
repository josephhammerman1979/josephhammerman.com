package game

import (
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
)

// Screen dimensions
const (
	ScreenWidth     = 800
	ScreenHeight    = 600
	ScorePanelWidth = 400 // Left side of screen
)

// Shared font
var RegularFont font.Face

func init() {
	tt, _ := opentype.Parse(goregular.TTF)
	RegularFont, _ = opentype.NewFace(tt, &opentype.FaceOptions{
		Size:    24,
		DPI:     72,
		Hinting: font.HintingFull,
	})
}
