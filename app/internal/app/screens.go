package app

import (
	"fmt"
	"image/color"
	"math"

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

// drawGameScreen shows the game list of the selected console with a side
// panel detailing the selected game (boxart or placeholder + metadata).
func (a *App) drawGameScreen(screen *ebiten.Image) {
	fillRect(screen, 0, 0, 960, 720, colBG)
	con := a.consoles[a.selCon]

	// Header: console name + back hint.
	drawText(screen, con.Name, 60, 50, 3, consoleColor(con.ID))
	drawText(screen, "Échap  retour consoles", 640, 66, 1, colTextDim)

	// Game list (left).
	listX, listY, listW, rowH := 70, 140, 470, 54
	for i, g := range con.Games {
		y := listY + i*rowH
		fill := colPanel
		if i == a.selGame {
			fill = colPanelHi
		}
		drawPanel(screen, listX, y, listW, rowH-8, fill, colPanelHi)
		if i == a.selGame {
			// Left accent bar + arrow.
			fillRect(screen, listX, y, 6, rowH-8, consoleColor(con.ID))
			drawText(screen, ">", listX-28, y+16, 2, colAccent2)
		}
		drawText(screen, g.Name, listX+24, y+18, 2, colText)
		drawText(screen, fmt.Sprintf("%s · %d Ko", g.Ext, g.Size/1024), listX+24, y+40, 1, colTextDim)
	}

	// Side panel with the selected game's boxart + details.
	sel := con.Games[a.selGame]
	panelX, panelY, panelW, panelH := 580, 140, 330, 440
	drawPanel(screen, panelX, panelY, panelW, panelH, colPanel, consoleColor(con.ID))

	// Boxart scaled to fit the tile box, preserving aspect, centered.
	const tileW, tileH = 160.0, 220.0
	img := a.boxartFor(con.ID, sel.Name)
	iw, ih := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
	scale := math.Min(tileW/iw, tileH/ih)
	op := &ebiten.DrawImageOptions{}
	op.Filter = ebiten.FilterNearest
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(float64(panelX)+(float64(panelW)-tileW)/2+(tileW-iw*scale)/2,
		float64(panelY)+24+(tileH-ih*scale)/2)
	screen.DrawImage(img, op)

	// Details.
	dy := panelY + 24 + tileH + 20
	drawText(screen, sel.Name, panelX+20, dy, 1, colAccent2)
	dy += 26
	drawText(screen, fmt.Sprintf("Console  %s", con.Name), panelX+20, dy, 1, colTextDim)
	dy += 22
	drawText(screen, fmt.Sprintf("Format   %s", sel.Ext), panelX+20, dy, 1, colTextDim)
	dy += 22
	drawText(screen, fmt.Sprintf("Taille   %d Ko", sel.Size/1024), panelX+20, dy, 1, colTextDim)
	drawText(screen, fmt.Sprintf("MD5      %s…", sel.MD5[:8]), panelX+20, dy+22, 1, colTextDim)

	// Controls hint.
	drawPanel(screen, 40, 620, 880, 50, colPanel, colAccent)
	drawText(screen, "↑ ↓  jeu     Entrée  jouer     Échap  retour", 250, 634, 2, colText)

	drawScanlines(screen)
}
