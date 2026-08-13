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

// drawTitleScreen shows the arcade boot screen with blinking PRESS START and
// a console marquee.
func (a *App) drawTitleScreen(screen *ebiten.Image, frameCount int) {
	fillRect(screen, 0, 0, 960, 720, colBG)

	// Warm glow behind the logo.
	drawGlow(screen, 480, 210, 720, 300, glowColor(colAccent))

	// Logo with drop shadow for depth.
	const logo = "RETRO RACE"
	logoW := textWidth(logo) * 6
	lx := (960 - logoW) / 2
	drawText(screen, logo, lx+7, 187, 6, color.RGBA{0x00, 0x00, 0x00, 0xb0})
	drawText(screen, logo, lx, 180, 6, colAccent)
	fillRect(screen, lx+40, 180+6*13+6, logoW-80, 6, colAccent2)

	// Tagline.
	drawText(screen, "la communauté du jeu rétro", 384, 292, 2, colTextDim)

	// Console marquee: all systems side by side.
	n := len(a.consoles)
	if n > 0 {
		const gw = 170
		total := n * gw
		startX := (960 - total) / 2
		for i := range a.consoles {
			con := a.consoles[i]
			art := consoleArt(con.ID)
			sc := 0.34
			aw := float64(artW) * sc
			ah := float64(artH) * sc
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Scale(sc, sc)
			op.GeoM.Translate(float64(startX+i*gw)+float64(gw)/2-aw/2, 380-ah/2)
			op.Filter = ebiten.FilterNearest
			screen.DrawImage(art, op)
			// Console-colored underline below the console.
			cc := consoleColor(con.ID)
			fillRect(screen, startX+i*gw+30, 432, gw-60, 4, cc)
		}
	}

	// Blinking press start.
	if (frameCount/30)%2 == 0 {
		drawGlow(screen, 480, 520, 360, 60, glowColor(colAccent2))
		drawText(screen, "PRESS START", 384, 508, 3, colText)
	}

	// Footer hints.
	drawPanel(screen, 240, 610, 480, 40, colPanel, colAccent)
	drawText(screen, "Entrée / Espace pour commencer", 296, 622, 1, colTextDim)

	drawScanlines(screen)
	drawVignette(screen)
}

// drawGameScreen shows the game list of the selected console with a side
// panel detailing the selected game (boxart + metadata).
func (a *App) drawGameScreen(screen *ebiten.Image) {
	fillRect(screen, 0, 0, 960, 720, colBG)
	con := a.consoles[a.selCon]
	cc := consoleColor(con.ID)

	// Header: console name + back hint.
	drawGlow(screen, 200, 60, 420, 90, glowColor(cc))
	drawText(screen, con.Name, 60, 40, 3, cc)
	drawText(screen, "Échap  retour consoles", 700, 66, 1, colTextDim)
	fillRect(screen, 60, 92, 320, 4, cc)

	// Game list (left).
	listX, listY, listW, rowH := 40, 132, 480, 58
	for i, g := range con.Games {
		y := listY + i*rowH
		sel := i == a.selGame

		var fill color.Color = colPanel
		var border color.Color = colPanelHi
		if sel {
			fill = colPanelHi
			border = cc
		}
		drawPanel(screen, listX, y, listW, rowH-10, fill, border)
		if sel {
			// Left accent bar + arrow marker.
			fillRect(screen, listX, y, 6, rowH-10, cc)
			drawText(screen, "▶", listX-30, y+18, 1, cc)
		}
		nameCol := colText
		if !sel {
			nameCol = colTextDim
		}
		drawText(screen, g.Name, listX+26, y+14, 2, nameCol)
		drawText(screen, fmt.Sprintf("%s · %d Ko", g.Ext, g.Size/1024), listX+26, y+38, 1, colTextDim)
	}

	// Side panel with the selected game's boxart + details.
	sel := con.Games[a.selGame]
	panelX, panelY, panelW, panelH := 560, 132, 360, 480
	drawPanel(screen, panelX, panelY, panelW, panelH, colPanel, cc)
	// Top accent strip inside the panel.
	fillRect(screen, panelX+8, panelY+8, panelW-16, 4, cc)

	// Boxart scaled to fit the tile box, preserving aspect, centered.
	const tileW, tileH = 170.0, 230.0
	img := a.boxartFor(con.ID, sel.Name)
	iw, ih := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
	scale := math.Min(tileW/iw, tileH/ih)
	op := &ebiten.DrawImageOptions{}
	op.Filter = ebiten.FilterNearest
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(float64(panelX)+(float64(panelW)-tileW)/2+(tileW-iw*scale)/2,
		float64(panelY)+20+(tileH-ih*scale)/2)
	screen.DrawImage(img, op)

	// Details.
	dy := panelY + 20 + tileH + 22
	drawText(screen, sel.Name, panelX+24, dy, 1, colAccent2)
	dy += 28
	drawText(screen, fmt.Sprintf("Console  %s", con.Name), panelX+24, dy, 1, colTextDim)
	dy += 24
	drawText(screen, fmt.Sprintf("Format   %s", sel.Ext), panelX+24, dy, 1, colTextDim)
	dy += 24
	drawText(screen, fmt.Sprintf("Taille   %d Ko", sel.Size/1024), panelX+24, dy, 1, colTextDim)
	drawText(screen, fmt.Sprintf("MD5      %s…", sel.MD5[:8]), panelX+24, dy+24, 1, colTextDim)

	// Controls hint.
	drawPanel(screen, 40, 636, 880, 44, colPanel, colAccent)
	drawText(screen, "↑ ↓  jeu     Entrée  jouer     Échap  retour", 276, 650, 2, colText)

	drawScanlines(screen)
	drawVignette(screen)
}
