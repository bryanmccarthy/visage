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
	image *ebiten.Image
}

type Button struct {
	w, h    int
	xOffset int
	yOffset int
	image   *ebiten.Image
	action  func(selectedIndex int)
}

type Game struct {
	visages        []Visage
	buttons        []Button
	err            error
	m              sync.Mutex
	cursor         ebiten.CursorShapeType
	selected       bool
	selectedIndex  int
	dragging       bool
	dragOffsetX    int
	dragOffsetY    int
	resizing       bool
	resizeHandle   int
	panning        bool
	panStartX      int
	panStartY      int
	clicking       bool
	erasing        bool
	sliderDragging bool
	sliderValue    int
}

var pressedKeys = map[ebiten.Key]bool{}

const (
	handleArea        = 8
	handleDisplaySize = 4
	handleNone        = 0
	handleTopLeft     = 1
	handleTopRight    = 2
	handleBottomLeft  = 3
	handleBottomRight = 4
	buttonSize        = 30
	sliderMin         = 5
	sliderMax         = 145
	sliderWidth       = 150
	sliderHeight      = 8
	sliderYOffset     = 18
)

var (
	colorNeonRed = color.RGBA{255, 32, 78, 255}
	// colorDarkRed = color.RGBA{160, 21, 62, 255}
	// colorMaroon  = color.RGBA{93, 14, 65, 255}
	colorNavy   = color.RGBA{0, 34, 77, 255}
	colorEraser = color.RGBA{255, 32, 78, 200}
)

const debug = true

func (g *Game) handleErrors() error {
	if err := func() error {
		g.m.Lock()
		defer g.m.Unlock()
		return g.err
	}(); err != nil {
		return err
	}

	return nil
}

func (g *Game) handleDroppedFiles() {
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
					x:     40,
					y:     40,
					w:     eimg.Bounds().Dx(),
					h:     eimg.Bounds().Dy(),
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
}

func (g *Game) handleCursor(x, y int) {
	cursor := ebiten.CursorShapeDefault

	if g.selected {
		v := g.visages[g.selectedIndex]

		for _, button := range g.buttons { // Button Hover Cursor
			if x >= v.x+button.xOffset && x <= v.x+button.xOffset+buttonSize && y >= v.y+button.yOffset && y <= v.y+button.yOffset+buttonSize {
				cursor = ebiten.CursorShapePointer
			}
		}

		if x >= v.x-handleArea && x <= v.x+handleArea && y >= v.y-handleArea && y <= v.y+handleArea {
			cursor = ebiten.CursorShapeNWSEResize
		} else if x >= v.x+v.w-handleArea && x <= v.x+v.w+handleArea && y >= v.y-handleArea && y <= v.y+handleArea {
			cursor = ebiten.CursorShapeNESWResize
		} else if x >= v.x-handleArea && x <= v.x+handleArea && y >= v.y+v.h-handleArea && y <= v.y+v.h+handleArea {
			cursor = ebiten.CursorShapeNESWResize
		} else if x >= v.x+v.w-handleArea && x <= v.x+v.w+handleArea && y >= v.y+v.h-handleArea && y <= v.y+v.h+handleArea {
			cursor = ebiten.CursorShapeNWSEResize
		}
	}

	if g.dragging || g.panning {
		cursor = ebiten.CursorShapeMove
	}

	if g.erasing {
		cursor = ebiten.CursorShapeCrosshair
	}

	if g.cursor != cursor {
		ebiten.SetCursorShape(cursor)
		g.cursor = cursor
	}
}

func (g *Game) handleKeybinds() {
	keyActions := map[ebiten.Key]func(int){
		ebiten.KeyE: g.buttons[0].action,
		ebiten.KeyF: g.buttons[1].action,
		ebiten.KeyR: g.buttons[2].action,
		ebiten.KeyD: g.buttons[3].action,
		ebiten.KeyC: g.buttons[4].action,
	}

	for key, action := range keyActions {
		if ebiten.IsKeyPressed(key) {
			if !pressedKeys[key] {
				action(g.selectedIndex)
			}
			pressedKeys[key] = true
		} else {
			pressedKeys[key] = false
		}
	}
}

func (g *Game) handleMouseActions(x, y int) {
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		g.handleLeftMouseButton(x, y)
	} else if ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight) {
		g.handlePanning(x, y)
	} else {
		g.resetActions()
	}
}

func (g *Game) checkResizeHandles(x, y int) {
	v := g.visages[g.selectedIndex]
	if x >= v.x-handleArea && x <= v.x+handleArea && y >= v.y-handleArea && y <= v.y+handleArea {
		g.resizing = true
		g.resizeHandle = handleTopLeft
	} else if x >= v.x+v.w-handleArea && x <= v.x+v.w+handleArea && y >= v.y-handleArea && y <= v.y+handleArea {
		g.resizing = true
		g.resizeHandle = handleTopRight
	} else if x >= v.x-handleArea && x <= v.x+handleArea && y >= v.y+v.h-handleArea && y <= v.y+v.h+handleArea {
		g.resizing = true
		g.resizeHandle = handleBottomLeft
	} else if x >= v.x+v.w-handleArea && x <= v.x+v.w+handleArea && y >= v.y+v.h-handleArea && y <= v.y+v.h+handleArea {
		g.resizing = true
		g.resizeHandle = handleBottomRight
	}
}

func (g *Game) getPixelCoordinates(v *Visage, x, y int) (int, int) {
	return int(float64(x-v.x) * (float64(v.image.Bounds().Dx()) / float64(v.w))), int(float64(y-v.y) * (float64(v.image.Bounds().Dy()) / float64(v.h)))
}

func (g *Game) erasePixels(v *Visage, px, py int) {
	w, h := v.image.Bounds().Dx(), v.image.Bounds().Dy()
	radiusSquared := g.sliderValue * g.sliderValue

	for dx := -g.sliderValue; dx <= g.sliderValue; dx++ {
		for dy := -g.sliderValue; dy <= g.sliderValue; dy++ {
			if dx*dx+dy*dy <= radiusSquared {
				ex := px + dx
				ey := py + dy
				if ex >= 0 && ey >= 0 && ex < w && ey < h {
					v.image.Set(ex, ey, color.RGBA{0, 0, 0, 0})
				}
			}
		}
	}
}

func (g *Game) updateSliderValue(value int) {
	g.sliderValue = value
	if g.sliderValue < sliderMin {
		g.sliderValue = sliderMin
	} else if g.sliderValue > sliderMax {
		g.sliderValue = sliderMax
	}
}

func (g *Game) handleErasing(x, y int) {
	v := &g.visages[g.selectedIndex]
	sliderMouseOffset := 14
	if x >= v.x+(v.w/2)-(sliderWidth/2)-sliderMouseOffset && x <= v.x+(v.w/2)-(sliderWidth/2)+sliderWidth+sliderMouseOffset && y >= v.y+v.h+sliderYOffset-sliderMouseOffset && y <= v.y+v.h+sliderYOffset+sliderHeight+sliderMouseOffset {
		g.sliderDragging = true
		g.updateSliderValue(x - (v.x + (v.w / 2) - (sliderWidth / 2)))
	}

	px, py := g.getPixelCoordinates(v, x, y)
	g.erasePixels(v, px, py)
}

func (g *Game) checkButtonClicks(x, y int) {
	v := g.visages[g.selectedIndex]
	for _, button := range g.buttons {
		if x >= v.x+button.xOffset && x <= v.x+button.xOffset+buttonSize && y >= v.y+button.yOffset && y <= v.y+button.yOffset+buttonSize {
			button.action(g.selectedIndex)
			g.clicking = true
		}
	}
}

func (g *Game) handleLeftMouseButton(x, y int) {
	if !g.dragging && !g.resizing {
		if g.selected {
			g.checkResizeHandles(x, y)
			if !g.clicking {
				g.checkButtonClicks(x, y)
			}
			if g.erasing {
				g.handleErasing(x, y)
			}
		}

		if !g.resizing && !g.clicking && !g.erasing {
			g.checkVisageDrag(x, y)
		}
	} else if g.dragging {
		g.dragSelectedVisage(x, y)
	} else if g.resizing {
		g.resizeSelectedVisage(x, y)
	}
}

func (g *Game) checkVisageDrag(x, y int) {
	for i := len(g.visages) - 1; i >= 0; i-- {
		v := g.visages[i]
		if x >= v.x && x <= v.x+v.w && y >= v.y && y <= v.y+v.h {
			g.dragging = true
			g.selected = true
			g.selectedIndex = i
			g.dragOffsetX = x - v.x
			g.dragOffsetY = y - v.y
			break
		}
	}

	if !g.dragging {
		g.selected = false
	}
}

func (g *Game) dragSelectedVisage(x, y int) {
	v := &g.visages[g.selectedIndex]
	v.x = x - g.dragOffsetX
	v.y = y - g.dragOffsetY
}

func (g *Game) resizeSelectedVisage(x, y int) {
	v := &g.visages[g.selectedIndex]
	switch g.resizeHandle {
	case handleTopLeft:
		v.w += v.x - x
		v.h += v.y - y
		v.x = x
		v.y = y
	case handleTopRight:
		v.w = x - v.x
		v.h += v.y - y
		v.y = y
	case handleBottomLeft:
		v.w += v.x - x
		v.h = y - v.y
		v.x = x
	case handleBottomRight:
		v.w = x - v.x
		v.h = y - v.y
	}
	if v.w < 1 {
		v.w = 1
	}
	if v.h < 1 {
		v.h = 1
	}
}

func (g *Game) resetActions() {
	g.dragging = false
	g.resizing = false
	g.clicking = false
	g.panning = false
	g.sliderDragging = false
	g.resizeHandle = handleNone
}

func (g *Game) handlePanning(x, y int) {
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
}

func (g *Game) Update() error {

	g.handleErrors()
	g.handleDroppedFiles()
	g.handleKeybinds()

	x, y := ebiten.CursorPosition()
	g.handleMouseActions(x, y)
	g.handleCursor(x, y)

	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{240, 235, 230, 255})

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

		var borderThickness float32 = 2

		// Draw border
		vector.DrawFilledRect(screen, float32(v.x), float32(v.y), float32(v.w), borderThickness, colorNavy, false)
		vector.DrawFilledRect(screen, float32(v.x), float32(v.y+v.h), float32(v.w)+borderThickness, borderThickness, colorNavy, false)
		vector.DrawFilledRect(screen, float32(v.x), float32(v.y), borderThickness, float32(v.h), colorNavy, false)
		vector.DrawFilledRect(screen, float32(v.x+v.w), float32(v.y), borderThickness, float32(v.h)+borderThickness, colorNavy, false)

		// Draw resize handles outer
		vector.DrawFilledCircle(screen, float32(v.x), float32(v.y), float32(handleDisplaySize)+1, colorNavy, false)
		vector.DrawFilledCircle(screen, float32(v.x+v.w), float32(v.y), float32(handleDisplaySize)+1, colorNavy, false)
		vector.DrawFilledCircle(screen, float32(v.x), float32(v.y+v.h), float32(handleDisplaySize)+1, colorNavy, false)
		vector.DrawFilledCircle(screen, float32(v.x+v.w), float32(v.y+v.h), float32(handleDisplaySize)+1, colorNavy, false)
		// Draw resize handles inner
		vector.DrawFilledCircle(screen, float32(v.x), float32(v.y), float32(handleDisplaySize), colorNeonRed, false)
		vector.DrawFilledCircle(screen, float32(v.x+v.w), float32(v.y), float32(handleDisplaySize), colorNeonRed, false)
		vector.DrawFilledCircle(screen, float32(v.x), float32(v.y+v.h), float32(handleDisplaySize), colorNeonRed, false)
		vector.DrawFilledCircle(screen, float32(v.x+v.w), float32(v.y+v.h), float32(handleDisplaySize), colorNeonRed, false)

		// Draw buttons
		for i, button := range g.buttons {
			vector.DrawFilledRect(screen, float32(v.x+button.xOffset), float32(v.y+button.yOffset), float32(buttonSize), float32(buttonSize), colorNavy, false)
			if g.erasing && i == 5 {
				vector.DrawFilledRect(screen, float32(v.x+button.xOffset), float32(v.y+button.yOffset), float32(buttonSize), float32(buttonSize), colorNeonRed, true)
			}

			op := &ebiten.DrawImageOptions{}
			op.Filter = ebiten.FilterLinear
			op.GeoM.Scale(float64(button.w)/float64(button.image.Bounds().Dx()), float64(button.h)/float64(button.image.Bounds().Dy()))
			op.GeoM.Translate(float64(v.x+button.xOffset+2), float64(v.y+button.yOffset+2)) // +2 for icon padding
			screen.DrawImage(button.image, op)
		}

		// Draw eraser size slider
		if g.erasing {
			x, y := ebiten.CursorPosition()
			// Eraser cursor at mouse position
			vector.DrawFilledCircle(screen, float32(x), float32(y), float32(g.sliderValue), colorEraser, false)
			// Slide bar
			vector.DrawFilledRect(screen, float32(v.x+(v.w/2)-(sliderWidth/2)), float32(v.y+v.h+sliderYOffset), sliderWidth, sliderHeight, colorNavy, false)
			// Slider ball
			vector.DrawFilledCircle(screen, float32(v.x+(v.w/2)-(sliderWidth/2)+g.sliderValue), float32(v.y+v.h+sliderYOffset+4), 12, colorNavy, false)
			vector.DrawFilledCircle(screen, float32(v.x+(v.w/2)-(sliderWidth/2)+g.sliderValue), float32(v.y+v.h+sliderYOffset+4), 10, colorNeonRed, false)
		}
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func main() {
	g := &Game{
		sliderValue: 30,
	}

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

	eraseIcon, _, err := ebitenutil.NewImageFromFile("assets/erase.png")
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
				g.erasing = false
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
					image: newImage,
				}
				g.visages = append(g.visages, newVisage)
				g.selectedIndex = len(g.visages) - 1
			},
		},
		{
			w:       26,
			h:       26,
			xOffset: -36,
			yOffset: 190,
			image:   eraseIcon,
			action: func(selectedIndex int) {
				g.erasing = !g.erasing
				log.Println("Erasing: ", g.erasing)
			},
		},
	}

	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowTitle("visage")
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
