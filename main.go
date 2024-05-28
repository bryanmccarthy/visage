package main

import (
	"io/fs"
	"log"
	"math"
	"sync"

	"image/color"
	"image/png"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

type Visage struct {
	x, y  int
	w, h  int
	scale float64
	image *ebiten.Image
}

type Game struct {
	visages       []Visage
	buttons       []Button
	err           error
	m             sync.Mutex
	cursor        ebiten.CursorShapeType
	selected      bool
	selectedIndex int
	dragging      bool
	dragOffsetX   int
	dragOffsetY   int
	resizing      bool
	resizeHandle  int
	panning       bool
	panStartX     int
	panStartY     int
	clicking      bool
}

type Button struct {
	w, h    int
	xOffset int
	yOffset int
	image   *ebiten.Image
	action  func(selectedIndex int)
}

var pressedKeys = map[ebiten.Key]bool{}

const (
	handleSize        = 6
	handleNone        = 0
	handleTopLeft     = 1
	handleTopRight    = 2
	handleBottomLeft  = 3
	handleBottomRight = 4
	buttonSize        = 30
)

const debug = true

func (g *Game) Update() error {
	if err := func() error {
		g.m.Lock()
		defer g.m.Unlock()
		return g.err
	}(); err != nil {
		return err
	}

	if files := ebiten.DroppedFiles(); files != nil {
		go func() {
			if err := fs.WalkDir(files, ".", func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}

				fi, err := d.Info()
				if err != nil {
					return err
				}
				log.Printf("Name: %s, Size: %d, IsDir: %t, ModTime: %v", fi.Name(), fi.Size(), fi.IsDir(), fi.ModTime())

				f, err := files.Open(path)
				if err != nil {
					return err
				}
				defer func() {
					_ = f.Close()
				}()

				img, err := png.Decode(f)
				if err != nil {
					log.Printf("Failed to decode the PNG file: %v", err)
					return nil
				}

				eimg := ebiten.NewImageFromImage(img)

				g.m.Lock()
				newVisage := Visage{
					x:     10,
					y:     10,
					w:     eimg.Bounds().Dx(),
					h:     eimg.Bounds().Dy(),
					scale: 1,
					image: eimg,
				}
				g.visages = append(g.visages, newVisage)
				g.m.Unlock()

				return nil
			}); err != nil {
				g.m.Lock()
				if g.err == nil {
					g.err = err
				}
				g.m.Unlock()
			}
		}()
	}

	x, y := ebiten.CursorPosition()
	cursor := ebiten.CursorShapeDefault

	if g.selected { // Button Hover Cursor
		v := g.visages[g.selectedIndex]
		for _, button := range g.buttons {
			if x >= v.x+button.xOffset && x <= v.x+button.xOffset+buttonSize && y >= v.y+button.yOffset && y <= v.y+button.yOffset+buttonSize {
				cursor = ebiten.CursorShapePointer
			}
		}
	}

	if g.selected { // Resize Hover Cursor
		v := g.visages[g.selectedIndex]
		imgX := v.x
		imgY := v.y
		imgW := v.w
		imgH := v.h

		if x >= imgX-handleSize && x <= imgX+handleSize && y >= imgY-handleSize && y <= imgY+handleSize {
			cursor = ebiten.CursorShapeNWSEResize
		} else if x >= imgX+imgW-handleSize && x <= imgX+imgW+handleSize && y >= imgY-handleSize && y <= imgY+handleSize {
			cursor = ebiten.CursorShapeNESWResize
		} else if x >= imgX-handleSize && x <= imgX+handleSize && y >= imgY+imgH-handleSize && y <= imgY+imgH+handleSize {
			cursor = ebiten.CursorShapeNESWResize
		} else if x >= imgX+imgW-handleSize && x <= imgX+imgW+handleSize && y >= imgY+imgH-handleSize && y <= imgY+imgH+handleSize {
			cursor = ebiten.CursorShapeNWSEResize
		}
	}

	if g.dragging || g.panning { // Drag or Pan Cursor
		cursor = ebiten.CursorShapeMove
	}

	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		if !g.dragging && !g.resizing {
			if g.selected { // Check for resize and button clicks on selected visage
				v := g.visages[g.selectedIndex]
				imgX := v.x
				imgY := v.y
				imgW := v.w
				imgH := v.h

				if x >= imgX-handleSize && x <= imgX+handleSize && y >= imgY-handleSize && y <= imgY+handleSize {
					g.resizing = true
					g.resizeHandle = handleTopLeft
				} else if x >= imgX+imgW-handleSize && x <= imgX+imgW+handleSize && y >= imgY-handleSize && y <= imgY+handleSize {
					g.resizing = true
					g.resizeHandle = handleTopRight
				} else if x >= imgX-handleSize && x <= imgX+handleSize && y >= imgY+imgH-handleSize && y <= imgY+imgH+handleSize {
					g.resizing = true
					g.resizeHandle = handleBottomLeft
				} else if x >= imgX+imgW-handleSize && x <= imgX+imgW+handleSize && y >= imgY+imgH-handleSize && y <= imgY+imgH+handleSize {
					g.resizing = true
					g.resizeHandle = handleBottomRight
				}

				if !g.clicking {
					for _, button := range g.buttons {
						if x >= imgX+button.xOffset && x <= imgX+button.xOffset+buttonSize && y >= imgY+button.yOffset && y <= imgY+button.yOffset+buttonSize {
							button.action(g.selectedIndex)
							g.clicking = true
						}
					}
				}
			}

			if !g.resizing && !g.clicking { // Check for drag after resize or click to prevent overlap reselection
				for i := len(g.visages) - 1; i >= 0; i-- {
					v := g.visages[i]
					imgX := v.x
					imgY := v.y
					imgW := v.w
					imgH := v.h

					if x >= imgX && x <= imgX+imgW && y >= imgY && y <= imgY+imgH {
						g.dragging = true
						g.selected = true
						g.selectedIndex = i
						g.dragOffsetX = x - imgX
						g.dragOffsetY = y - imgY
						break
					}
				}

				if !g.dragging { // Deselect if no visage is dragged (click outside)
					g.selected = false
				}
			}
		} else if g.dragging {
			v := &g.visages[g.selectedIndex]
			v.x = x - g.dragOffsetX
			v.y = y - g.dragOffsetY
		} else if g.resizing {
			v := &g.visages[g.selectedIndex]
			switch g.resizeHandle {
			case handleTopLeft:
				v.w += v.x - x
				v.h += v.y - y
				v.x = x
				v.y = y
				cursor = ebiten.CursorShapeNWSEResize
			case handleTopRight:
				v.w = x - v.x
				v.h += v.y - y
				v.y = y
				cursor = ebiten.CursorShapeNESWResize
			case handleBottomLeft:
				v.w += v.x - x
				v.h = y - v.y
				v.x = x
				cursor = ebiten.CursorShapeNESWResize
			case handleBottomRight:
				v.w = x - v.x
				v.h = y - v.y
				cursor = ebiten.CursorShapeNWSEResize
			}
		}
	} else {
		g.dragging = false
		g.resizing = false
		g.clicking = false
		g.resizeHandle = handleNone
	}

	// Handle panning
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight) {
		if !g.panning {
			g.panStartX = x
			g.panStartY = y
			g.panning = true
		} else {
			dx := x - g.panStartX
			dy := y - g.panStartY

			for i := range g.visages {
				g.visages[i].x += dx
				g.visages[i].y += dy
			}

			g.panStartX = x
			g.panStartY = y
		}
	} else {
		g.panning = false
	}

	if g.selected {
		if ebiten.IsKeyPressed(ebiten.KeyE) {
			if !pressedKeys[ebiten.KeyE] {
				g.buttons[0].action(g.selectedIndex)
			}
			pressedKeys[ebiten.KeyE] = true
		} else {
			pressedKeys[ebiten.KeyE] = false
		}

		if ebiten.IsKeyPressed(ebiten.KeyF) {
			if !pressedKeys[ebiten.KeyF] {
				g.buttons[1].action(g.selectedIndex)
			}
			pressedKeys[ebiten.KeyF] = true
		} else {
			pressedKeys[ebiten.KeyF] = false
		}

		if ebiten.IsKeyPressed(ebiten.KeyR) {
			if !pressedKeys[ebiten.KeyR] {
				g.buttons[2].action(g.selectedIndex)
			}
			pressedKeys[ebiten.KeyR] = true
		} else {
			pressedKeys[ebiten.KeyR] = false
		}

		if ebiten.IsKeyPressed(ebiten.KeyD) {
			if !pressedKeys[ebiten.KeyD] {
				g.buttons[3].action(g.selectedIndex)
			}
			pressedKeys[ebiten.KeyD] = true
		} else {
			pressedKeys[ebiten.KeyD] = false
		}

		if ebiten.IsKeyPressed(ebiten.KeyC) {
			if !pressedKeys[ebiten.KeyC] {
				g.buttons[4].action(g.selectedIndex)
			}
			pressedKeys[ebiten.KeyC] = true
		} else {
			pressedKeys[ebiten.KeyC] = false
		}
	}

	if g.cursor != cursor { // Only set cursor if it has changed
		ebiten.SetCursorShape(cursor)
		g.cursor = cursor
	}

	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{188, 184, 191, 255})

	g.m.Lock()
	defer g.m.Unlock()

	if debug {
		vector.DrawFilledRect(screen, 0, 0, 120, 20, color.RGBA{100, 100, 100, 200}, false)

		switch ebiten.CursorShape() {
		case ebiten.CursorShapeDefault:
			ebitenutil.DebugPrint(screen, "Cursor: Default")
		case ebiten.CursorShapeMove:
			ebitenutil.DebugPrint(screen, "Cursor: Move")
		case ebiten.CursorShapeNESWResize:
			ebitenutil.DebugPrint(screen, "Cursor: NESW Resize")
		case ebiten.CursorShapeNWSEResize:
			ebitenutil.DebugPrint(screen, "Cursor: NWSE Resize")
		case ebiten.CursorShapePointer:
			ebitenutil.DebugPrint(screen, "Cursor: Pointer")
		}
	}

	if len(g.visages) == 0 {
		return
	}

	for _, visage := range g.visages {
		op := &ebiten.DrawImageOptions{}
		op.Filter = ebiten.FilterLinear
		op.GeoM.Scale(float64(visage.w)/float64(visage.image.Bounds().Dx()), float64(visage.h)/float64(visage.image.Bounds().Dy()))
		op.GeoM.Translate(float64(visage.x), float64(visage.y))
		screen.DrawImage(visage.image, op)
	}

	if g.selected {
		v := g.visages[g.selectedIndex]
		uiColor := color.RGBA{52, 13, 79, 210}

		// Draw border
		vector.DrawFilledRect(screen, float32(v.x), float32(v.y), float32(v.w), 1, uiColor, false)
		vector.DrawFilledRect(screen, float32(v.x), float32(v.y+v.h), float32(v.w), 1, uiColor, false)
		vector.DrawFilledRect(screen, float32(v.x), float32(v.y), 1, float32(v.h), uiColor, false)
		vector.DrawFilledRect(screen, float32(v.x+v.w), float32(v.y), 1, float32(v.h), uiColor, false)

		// Draw resize handles
		vector.DrawFilledRect(screen, float32(v.x-handleSize), float32(v.y-handleSize), float32(handleSize*2), float32(handleSize*2), uiColor, false)
		vector.DrawFilledRect(screen, float32(v.x+v.w-handleSize), float32(v.y-handleSize), float32(handleSize*2), float32(handleSize*2), uiColor, false)
		vector.DrawFilledRect(screen, float32(v.x-handleSize), float32(v.y+v.h-handleSize), float32(handleSize*2), float32(handleSize*2), uiColor, false)
		vector.DrawFilledRect(screen, float32(v.x+v.w-handleSize), float32(v.y+v.h-handleSize), float32(handleSize*2), float32(handleSize*2), uiColor, false)

		// Draw buttons
		for _, button := range g.buttons {
			vector.DrawFilledRect(screen, float32(v.x+button.xOffset), float32(v.y+button.yOffset), float32(buttonSize), float32(buttonSize), uiColor, false)

			op := &ebiten.DrawImageOptions{}
			op.Filter = ebiten.FilterLinear
			op.GeoM.Scale(float64(button.w)/float64(button.image.Bounds().Dx()), float64(button.h)/float64(button.image.Bounds().Dy()))
			op.GeoM.Translate(float64(v.x+button.xOffset+2), float64(v.y+button.yOffset+2)) // +2 for icon padding
			screen.DrawImage(button.image, op)
		}
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func main() {
	g := &Game{}

	moveIcon, _, err := ebitenutil.NewImageFromFile("assets/move.png")
	if err != nil {
		log.Fatal(err)
	}

	flipIcon, _, err := ebitenutil.NewImageFromFile("assets/flip.png")
	if err != nil {
		log.Fatal(err)
	}

	rotateIcon, _, err := ebitenutil.NewImageFromFile("assets/rotate.png")
	if err != nil {
		log.Fatal(err)
	}

	deleteIcon, _, err := ebitenutil.NewImageFromFile("assets/delete.png")
	if err != nil {
		log.Fatal(err)
	}

	copyIcon, _, err := ebitenutil.NewImageFromFile("assets/copy.png")
	if err != nil {
		log.Fatal(err)
	}

	g.buttons = []Button{
		{
			w:       26,
			h:       26,
			xOffset: -36,
			yOffset: 10,
			image:   moveIcon,
			action: func(selectedIndex int) {
				visage := g.visages[selectedIndex]

				if g.selectedIndex == len(g.visages)-1 { // Move to back if already at the top
					g.visages = append([]Visage{visage}, g.visages[:g.selectedIndex]...)
					g.selectedIndex = 0
				} else { // Move to top
					g.visages = append(g.visages[:g.selectedIndex], g.visages[g.selectedIndex+1:]...)
					g.visages = append(g.visages, visage)
					g.selectedIndex = len(g.visages) - 1
				}
			},
		},
		{
			w:       26,
			h:       26,
			xOffset: -36,
			yOffset: 46,
			image:   flipIcon,
			action: func(selectedIndex int) {
				visage := g.visages[selectedIndex]
				flippedImage := ebiten.NewImage(visage.image.Bounds().Dx(), visage.image.Bounds().Dy())
				op := &ebiten.DrawImageOptions{}
				op.GeoM.Scale(-1, 1)
				op.GeoM.Translate(float64(visage.image.Bounds().Dx()), 0)
				flippedImage.DrawImage(visage.image, op)
				visage.image = flippedImage
				g.visages[selectedIndex] = visage
			},
		},
		{
			w:       26,
			h:       26,
			xOffset: -36,
			yOffset: 82,
			image:   rotateIcon,
			action: func(selectedIndex int) {
				visage := g.visages[selectedIndex]
				rotatedImage := ebiten.NewImage(visage.image.Bounds().Dy(), visage.image.Bounds().Dx())
				op := &ebiten.DrawImageOptions{}

				// Translate to origin, rotate, translate back
				op.GeoM.Translate(-float64(visage.image.Bounds().Dx())/2, -float64(visage.image.Bounds().Dy())/2)
				op.GeoM.Rotate(math.Pi / 2)
				op.GeoM.Translate(float64(visage.image.Bounds().Dy())/2, float64(visage.image.Bounds().Dx())/2)

				rotatedImage.DrawImage(visage.image, op)
				visage.image = rotatedImage
				visage.w = rotatedImage.Bounds().Dx()
				visage.h = rotatedImage.Bounds().Dy()
				g.visages[selectedIndex] = visage
			},
		},
		{
			w:       26,
			h:       26,
			xOffset: -36,
			yOffset: 118,
			image:   deleteIcon,
			action: func(selectedIndex int) {
				g.visages = append(g.visages[:selectedIndex], g.visages[selectedIndex+1:]...)
				g.selected = false
				g.selectedIndex = 0
			},
		},
		{
			w:       26,
			h:       26,
			xOffset: -36,
			yOffset: 154,
			image:   copyIcon,
			action: func(selectedIndex int) {
				visage := g.visages[selectedIndex]
				newImage := ebiten.NewImage(visage.image.Bounds().Dx(), visage.image.Bounds().Dy())
				newImage.DrawImage(visage.image, nil)

				newVisage := Visage{
					x:     visage.x + 30,
					y:     visage.y + 30,
					w:     visage.w,
					h:     visage.h,
					scale: visage.scale,
					image: newImage,
				}
				g.visages = append(g.visages, newVisage)
				g.selectedIndex = len(g.visages) - 1
			},
		},
	}

	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowTitle("visage")
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
