package main

import (
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io/fs"
	"log"
	"math"
	"sync"

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
	prevMouseX     int
	prevMouseY     int
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
	erasingToggle  bool
	sliderDragging bool
	sliderValue    int
}

var keyActions = map[ebiten.Key]func(int){}
var pressedKeys = map[ebiten.Key]bool{}

const (
	handleArea        = 8
	handleDisplaySize = 4
	handleNone        = 0
	handleTopLeft     = 1
	handleTopRight    = 2
	handleBottomLeft  = 3
	handleBottomRight = 4
	buttonSize        = 32
	sliderMin         = 5
	sliderMax         = 145
	sliderWidth       = 150
	sliderHeight      = 8
	sliderYOffset     = 18
	erasingOOBOffset  = 80
)

var (
	colorEraser = color.RGBA{255, 32, 78, 200}
)

const cursorDebug = false
const actionDebug = false
const fpsDebug = true

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

				img, _, err := image.Decode(f)
				if err != nil {
					log.Printf("Failed to decode the image file: %v", err)
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

		if g.erasingToggle {
			// if out of bounds
			if x < v.x-erasingOOBOffset || x > v.x+v.w+erasingOOBOffset || y < v.y-erasingOOBOffset || y > v.y+v.h+erasingOOBOffset {
				cursor = ebiten.CursorShapePointer
			} else {
				cursor = ebiten.CursorShapeCrosshair
			}

		}

		for i, button := range g.buttons { // Button Hover Cursor
			yOffset := v.y + (buttonSize * i)
			if x >= v.x+button.xOffset && x <= v.x+button.xOffset+buttonSize && y >= yOffset && y <= yOffset+buttonSize {
				if g.erasingToggle && !containsIndex([]int{1, 2, 3}, i) {
					cursor = ebiten.CursorShapeNotAllowed
				} else {
					cursor = ebiten.CursorShapePointer
				}
			}
		}

		if g.dragging {
			cursor = ebiten.CursorShapeMove
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

	if g.panning {
		cursor = ebiten.CursorShapeMove
	}

	if g.cursor != cursor {
		ebiten.SetCursorShape(cursor)
		g.cursor = cursor
	}
}

func (g *Game) handleKeybinds() {
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
		if g.resizing {
			g.handleResizeMouseRelease()
		}

		g.handleMouseRelease()
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

func getPixelCoordinates(v *Visage, x, y int) (int, int) { // Needed for erasing resized images
	return int(float64(x-v.x) * (float64(v.image.Bounds().Dx()) / float64(v.w))), int(float64(y-v.y) * (float64(v.image.Bounds().Dy()) / float64(v.h)))
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// Bresenham's line algorithm with thickness
func drawLine(img *ebiten.Image, x0, y0, x1, y1, thickness int, col color.Color) {
	dx := abs(x1 - x0)
	dy := abs(y1 - y0)
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx - dy

	// TODO: Fix this
	drawCircle(img, x0, y0, thickness/2, color.RGBA{0, 0, 0, 0})

	for {
		for t := -thickness / 2; t <= thickness/2; t++ {
			if dx > dy {
				setPixel(img, x0, y0+t, col)
			} else {
				setPixel(img, x0+t, y0, col)
			}
		}

		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
	}

	// TODO: Fix this
	drawCircle(img, x1, y1, thickness/2, color.RGBA{0, 0, 0, 0})
}

// Bresenham's circle algorithm
func drawCircle(img *ebiten.Image, x0, y0, radius int, col color.Color) {
	x := radius
	y := 0
	err := 1 - x

	for x >= y {
		drawHorizontalLine(img, x0-y, x0+y, y0+x, col)
		drawHorizontalLine(img, x0-y, x0+y, y0-x, col)
		drawHorizontalLine(img, x0-x, x0+x, y0+y, col)
		drawHorizontalLine(img, x0-x, x0+x, y0-y, col)

		y++
		if err < 0 {
			err += 2*y + 1
		} else {
			x--
			err += 2*(y-x) + 1
		}
	}
}

func drawHorizontalLine(img *ebiten.Image, x1, x2, y int, col color.Color) {
	for x := x1; x <= x2; x++ {
		setPixel(img, x, y, col)
	}
}

// Function to set a pixel in the image, checking bounds
func setPixel(img *ebiten.Image, x, y int, col color.Color) {
	if x >= 0 && x < img.Bounds().Max.X && y >= 0 && y < img.Bounds().Max.Y {
		img.Set(x, y, col)
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
	if g.clicking { // Prevents erasing when clicking on buttons
		return
	}

	v := &g.visages[g.selectedIndex]

	// Slider dragging
	sliderMouseOffset := 14
	if x >= v.x+(v.w/2)-(sliderWidth/2)-sliderMouseOffset && x <= v.x+(v.w/2)-(sliderWidth/2)+sliderWidth+sliderMouseOffset && y >= v.y+v.h+sliderYOffset-sliderMouseOffset && y <= v.y+v.h+sliderYOffset+sliderHeight+sliderMouseOffset {
		g.sliderDragging = true
		g.updateSliderValue(x - (v.x + (v.w / 2) - (sliderWidth / 2)))
		return
	}

	// Check if outofbounds + an offset for deselecting the eraser
	if x < v.x-erasingOOBOffset || x > v.x+v.w+erasingOOBOffset || y < v.y-erasingOOBOffset || y > v.y+v.h+erasingOOBOffset {
		g.erasingToggle = false
		return
	}

	// Any outofbounds don't erase
	if x < v.x || x > v.x+v.w || y < v.y || y > v.y+v.h+sliderYOffset {
		return
	}

	if g.resizing { // Prevent erasing when resizing
		return
	}

	if g.prevMouseX == x && g.prevMouseY == y { // Prevent erasing when mouse is not moving
		return
	}

	px, py := getPixelCoordinates(v, x, y)
	if g.prevMouseX == 0 && g.prevMouseY == 0 {
		g.prevMouseX = px
		g.prevMouseY = py
	}

	// draw transparent line
	drawLine(v.image, g.prevMouseX, g.prevMouseY, px, py, g.sliderValue, color.RGBA{0, 0, 0, 0})
	g.prevMouseX = px
	g.prevMouseY = py
}

func containsIndex(arr []int, val int) bool {
	for _, v := range arr {
		if v == val {
			return true
		}
	}
	return false
}

func (g *Game) checkButtonClicks(x, y int) {
	v := g.visages[g.selectedIndex]

	for i, button := range g.buttons {
		yOffset := v.y + (buttonSize * i)
		if g.erasingToggle && !containsIndex([]int{1, 2, 3}, i) {
			continue
		}

		if x >= v.x+button.xOffset && x <= v.x+button.xOffset+buttonSize && y >= yOffset && y <= yOffset+buttonSize {
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
			if g.erasingToggle {
				g.handleErasing(x, y)
			}
		}

		if !g.resizing && !g.clicking && !g.erasingToggle {
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

	if !g.dragging { // Clicked outside of visage
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
}

func (g *Game) handleResizeMouseRelease() {
	v := &g.visages[g.selectedIndex]

	if v.w < 0 {
		v.x += v.w
		v.w = -v.w
		// flip image horizontally
		flippedImage := ebiten.NewImage(v.image.Bounds().Dx(), v.image.Bounds().Dy())
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(-1, 1)
		op.GeoM.Translate(float64(v.image.Bounds().Dx()), 0)
		flippedImage.DrawImage(v.image, op)
		v.image = flippedImage
	}

	if v.h < 0 {
		v.y += v.h
		v.h = -v.h
		// flip image vertically
		flippedImage := ebiten.NewImage(v.image.Bounds().Dx(), v.image.Bounds().Dy())
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(1, -1)
		op.GeoM.Translate(0, float64(v.image.Bounds().Dy()))
		flippedImage.DrawImage(v.image, op)
		v.image = flippedImage
	}

	g.resizing = false
	g.resizeHandle = handleNone
}

func (g *Game) handleMouseRelease() {
	g.dragging = false
	g.clicking = false
	g.panning = false
	g.sliderDragging = false
	g.prevMouseX = 0
	g.prevMouseY = 0
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

func (g *Game) drawVisages(screen *ebiten.Image) {
	for _, visage := range g.visages {
		op := &ebiten.DrawImageOptions{}
		op.Filter = ebiten.FilterLinear
		op.GeoM.Scale(float64(visage.w)/float64(visage.image.Bounds().Dx()), float64(visage.h)/float64(visage.image.Bounds().Dy()))
		op.GeoM.Translate(float64(visage.x), float64(visage.y))
		screen.DrawImage(visage.image, op)
	}

	if g.selected {
		v := g.visages[g.selectedIndex]
		g.drawVisageBorder(screen, v)
		g.drawResizeHandles(screen, v)
		g.drawButtons(screen, v)
		if g.erasingToggle {
			g.drawEraser(screen, v)
		}
	}
}

func (g *Game) drawVisageBorder(screen *ebiten.Image, v Visage) {
	var colorWhite = color.RGBA{0, 0, 0, 255}
	var borderThickness float32 = 2
	vector.DrawFilledRect(screen, float32(v.x), float32(v.y), float32(v.w), borderThickness, colorWhite, false)
	vector.DrawFilledRect(screen, float32(v.x), float32(v.y+v.h), float32(v.w)+borderThickness, borderThickness, colorWhite, false)
	vector.DrawFilledRect(screen, float32(v.x), float32(v.y), borderThickness, float32(v.h), colorWhite, false)
	vector.DrawFilledRect(screen, float32(v.x+v.w), float32(v.y), borderThickness, float32(v.h)+borderThickness, colorWhite, false)
}

func (g *Game) drawResizeHandles(screen *ebiten.Image, v Visage) {
	colorWhite := color.RGBA{255, 255, 255, 255}
	colorBlack := color.RGBA{0, 0, 0, 255}
	vector.DrawFilledCircle(screen, float32(v.x), float32(v.y), float32(handleDisplaySize)+1, colorWhite, false)
	vector.DrawFilledCircle(screen, float32(v.x+v.w), float32(v.y), float32(handleDisplaySize)+1, colorWhite, false)
	vector.DrawFilledCircle(screen, float32(v.x), float32(v.y+v.h), float32(handleDisplaySize)+1, colorWhite, false)
	vector.DrawFilledCircle(screen, float32(v.x+v.w), float32(v.y+v.h), float32(handleDisplaySize)+1, colorWhite, false)
	vector.DrawFilledCircle(screen, float32(v.x), float32(v.y), float32(handleDisplaySize), colorBlack, false)
	vector.DrawFilledCircle(screen, float32(v.x+v.w), float32(v.y), float32(handleDisplaySize), colorBlack, false)
	vector.DrawFilledCircle(screen, float32(v.x), float32(v.y+v.h), float32(handleDisplaySize), colorBlack, false)
	vector.DrawFilledCircle(screen, float32(v.x+v.w), float32(v.y+v.h), float32(handleDisplaySize), colorBlack, false)
}

func (g *Game) drawButtons(screen *ebiten.Image, v Visage) {
	for i, button := range g.buttons {
		xOffset := v.x + button.xOffset
		yOffset := v.y + button.yOffset + (buttonSize * i)
		padding := 2
		colorBlack := color.RGBA{0, 0, 0, 255}

		vector.DrawFilledRect(screen, float32(xOffset), float32(yOffset), float32(buttonSize), float32(buttonSize), colorBlack, false)

		if g.erasingToggle && containsIndex([]int{1, 2, 3}, i) {
			vector.DrawFilledRect(screen, float32(xOffset), float32(yOffset), float32(buttonSize), float32(buttonSize), colorEraser, true)
		}

		op := &ebiten.DrawImageOptions{}
		op.Filter = ebiten.FilterLinear
		op.GeoM.Scale(float64(button.w)/float64(button.image.Bounds().Dx()), float64(button.h)/float64(button.image.Bounds().Dy()))
		op.GeoM.Translate(float64(xOffset+padding), float64(yOffset+padding))
		screen.DrawImage(button.image, op)
	}
}

func (g *Game) drawEraser(screen *ebiten.Image, v Visage) {
	x, y := ebiten.CursorPosition()
	// if out of bounds don't draw
	if x < v.x-erasingOOBOffset || x > v.x+v.w+erasingOOBOffset || y < v.y-erasingOOBOffset || y > v.y+v.h+erasingOOBOffset {
		return
	}

	// Eraser cursor
	vector.DrawFilledCircle(screen, float32(x), float32(y), float32(g.sliderValue)/2, colorEraser, false)

	colorWhite := color.RGBA{255, 255, 255, 255}
	colorBlack := color.RGBA{0, 0, 0, 255}
	// Eraser slider
	vector.DrawFilledRect(screen, float32(v.x+(v.w/2)-(sliderWidth/2)), float32(v.y+v.h+sliderYOffset), sliderWidth, sliderHeight, colorBlack, false)
	vector.DrawFilledCircle(screen, float32(v.x+(v.w/2)-(sliderWidth/2)+g.sliderValue), float32(v.y+v.h+sliderYOffset+4), 12, colorBlack, false)
	vector.DrawFilledCircle(screen, float32(v.x+(v.w/2)-(sliderWidth/2)+g.sliderValue), float32(v.y+v.h+sliderYOffset+4), 10, colorWhite, false)
}

func (g *Game) drawDebugInfo(screen *ebiten.Image) {
	if fpsDebug {
		vector.DrawFilledRect(screen, 0, 0, 140, 20, color.RGBA{100, 100, 100, 200}, false)
		ebitenutil.DebugPrint(screen, "TPS: "+fmt.Sprintf("%.2f", ebiten.ActualTPS())+" FPS: "+fmt.Sprintf("%.2f", ebiten.ActualFPS()))
	}

	if cursorDebug {
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

	if actionDebug {
		vector.DrawFilledRect(screen, 0, 0, 120, 20, color.RGBA{100, 100, 100, 200}, false)
		switch {
		case g.dragging:
			ebitenutil.DebugPrint(screen, "Action: Dragging")
		case g.resizing:
			ebitenutil.DebugPrint(screen, "Action: Resizing")
		case g.panning:
			ebitenutil.DebugPrint(screen, "Action: Panning")
		case g.clicking:
			ebitenutil.DebugPrint(screen, "Action: Clicking")
		case g.erasingToggle:
			ebitenutil.DebugPrint(screen, "Action: Erasing")
		default:
			ebitenutil.DebugPrint(screen, "Action: None")
		}
	}
}

func (g *Game) moveAction(selectedIndex int) {
	if len(g.visages) == 0 || !g.selected || g.erasingToggle {
		return
	}

	visage := g.visages[selectedIndex]
	if g.selectedIndex == len(g.visages)-1 {
		g.visages = append([]Visage{visage}, g.visages[:g.selectedIndex]...)
		g.selectedIndex = 0
	} else {
		g.visages = append(g.visages[:g.selectedIndex], g.visages[g.selectedIndex+1:]...)
		g.visages = append(g.visages, visage)
		g.selectedIndex = len(g.visages) - 1
	}
}

func (g *Game) flipAction(selectedIndex int) {
	if len(g.visages) == 0 || !g.selected {
		return
	}

	visage := &g.visages[selectedIndex]
	flippedImage := ebiten.NewImage(visage.image.Bounds().Dx(), visage.image.Bounds().Dy())
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(-1, 1)
	op.GeoM.Translate(float64(visage.image.Bounds().Dx()), 0)
	flippedImage.DrawImage(visage.image, op)
	visage.image = flippedImage
}

func (g *Game) rotateAction(selectedIndex int) {
	if len(g.visages) == 0 || !g.selected {
		return
	}

	visage := &g.visages[selectedIndex]
	rotatedImage := ebiten.NewImage(visage.image.Bounds().Dy(), visage.image.Bounds().Dx())
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(-float64(visage.image.Bounds().Dx())/2, -float64(visage.image.Bounds().Dy())/2)
	op.GeoM.Rotate(math.Pi / 2)
	op.GeoM.Translate(float64(visage.image.Bounds().Dy())/2, float64(visage.image.Bounds().Dx())/2)
	rotatedImage.DrawImage(visage.image, op)
	visage.image = rotatedImage
	visage.w = rotatedImage.Bounds().Dx()
	visage.h = rotatedImage.Bounds().Dy()
}

func (g *Game) deleteAction(selectedIndex int) {
	if len(g.visages) == 0 || !g.selected || g.erasingToggle {
		return
	}

	if len(g.visages) <= 1 {
		g.visages = nil
		g.selected = false
	} else {
		g.visages = append(g.visages[:selectedIndex], g.visages[selectedIndex+1:]...)
		g.selected = true
		g.selectedIndex = len(g.visages) - 1
	}
}

func (g *Game) copyAction(selectedIndex int) {
	if len(g.visages) == 0 || !g.selected || g.erasingToggle {
		return
	}

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
}

func (g *Game) eraseAction(selectedIndex int) {
	g.erasingToggle = !g.erasingToggle
	log.Println("Erasing: ", g.erasingToggle)
}

func loadAssets(g *Game) {
	icons := []struct {
		path   string
		action func(selectedIndex int)
	}{
		{"assets/move.png", g.moveAction},
		{"assets/flip.png", g.flipAction},
		{"assets/rotate.png", g.rotateAction},
		{"assets/erase.png", g.eraseAction},
		{"assets/delete.png", g.deleteAction},
		{"assets/copy.png", g.copyAction},
	}

	for _, icon := range icons {
		img, _, err := ebitenutil.NewImageFromFile(icon.path)
		if err != nil {
			log.Fatal(err)
		}

		button := Button{
			w:       28,
			h:       28,
			xOffset: -38,
			yOffset: 10,
			image:   img,
			action:  icon.action,
		}
		g.buttons = append(g.buttons, button)
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
	screen.Fill(color.RGBA{240, 235, 230, 255}) // Background color
	screen.Fill(color.RGBA{120, 120, 120, 200})

	g.m.Lock()
	defer g.m.Unlock()

	g.drawDebugInfo(screen)
	g.drawVisages(screen)
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func main() {
	g := &Game{
		sliderValue: 30,
	}

	loadAssets(g)

	keyActions = map[ebiten.Key]func(int){
		ebiten.KeyW: g.buttons[0].action,
		ebiten.KeyF: g.buttons[1].action,
		ebiten.KeyR: g.buttons[2].action,
		ebiten.KeyE: g.buttons[3].action,
		ebiten.KeyD: g.buttons[4].action,
		ebiten.KeyC: g.buttons[5].action,
	}

	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowTitle("visage")
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
