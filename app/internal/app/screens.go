package app

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// consoleColor returns the arcade color for a console.
func consoleColor(id string) color.Color {
	switch id {
	case "nes":
		return colNES
	case "snes":
		return colSNES
	case "gb":
		return colGB
	case "genesis":
		return colGenesis
	}
	return colAccent
}

// drawTitleScreen shows the arcade boot screen with blinking PRESS START.
func (a *App) drawTitleScreen(screen *ebiten.Image, frameCount int) {
	fillRect(screen, 0, 0, 960, 720, colBG)

	// Glow strip behind the logo.
	drawPanel(screen, 140, 180, 680, 160, colPanel, colAccent)
	fillRect(screen, 140, 250, 680, 4, colAccent2)

	// Logo, big pixel style.
	drawText(screen, "RETRO", 300, 210, 8, colAccent)
	drawText(screen, "RACE", 330, 310, 8, colAccent2)

	drawText(screen, "la communauté du jeu rétro", 300, 430, 2, colTextDim)

	// Blinking press start (60-frame cycle).
	if (frameCount/30)%2 == 0 {
		drawText(screen, "PRESS START", 350, 540, 3, colText)
	}
	drawText(screen, "Entrée / Start pour commencer", 320, 600, 1, colTextDim)

	drawScanlines(screen)
}

// drawGameScreen shows the game list of the selected console.
func (a *App) drawGameScreen(screen *ebiten.Image) {
	fillRect(screen, 0, 0, 960, 720, colBG)
	con := a.consoles[a.selCon]

	// Header: console name + back hint.
	drawText(screen, con.Name, 60, 50, 3, consoleColor(con.ID))
	drawText(screen, "Échap  retour consoles", 620, 66, 1, colTextDim)

	// Game list.
	listX, listY := 160, 140
	rowH := 56
	for i, g := range con.Games {
		y := listY + i*rowH
		fill := colPanel
		if i == a.selGame {
			fill = colPanelHi
		}
		drawPanel(screen, listX, y, 640, rowH-8, fill, colPanelHi)
		if i == a.selGame {
			// Left accent bar + arrow.
			fillRect(screen, listX, y, 6, rowH-8, consoleColor(con.ID))
			drawText(screen, ">", listX-28, y+16, 2, colAccent2)
		}
		drawText(screen, g.Name, listX+24, y+18, 2, colText)
		drawText(screen, fmt.Sprintf("%s · %d Ko", g.Ext, g.Size/1024), listX+24, y+40, 1, colTextDim)
	}

	// Controls hint.
	drawPanel(screen, 40, 620, 880, 50, colPanel, colAccent)
	drawText(screen, "↑ ↓  jeu     Entrée  jouer     Échap  retour", 250, 634, 2, colText)

	drawScanlines(screen)
}
