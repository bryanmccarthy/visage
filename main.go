package main

import (
	"io/fs"
	"log"
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
	err           error
	m             sync.Mutex
	selected      bool
	selectedIndex int
	dragging      bool
	dragOffsetX   int
	dragOffsetY   int
	resizing      bool
	resizeHandle  int
}

const (
	handleSize        = 8
	handleNone        = 0
	handleTopLeft     = 1
	handleTopRight    = 2
	handleBottomLeft  = 3
	handleBottomRight = 4
)

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
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		log.Printf("Mouse position: %d, %d", x, y)

		if !g.dragging && !g.resizing {
			if g.selected { // Check for resize on selected visage
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
			}

			if !g.resizing { // Check for drag after resize to prevent overlap reselection
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
	} else {
		g.dragging = false
		g.resizing = false
		g.resizeHandle = handleNone
	}

	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{188, 184, 191, 255})

	g.m.Lock()
	defer g.m.Unlock()

	if len(g.visages) == 0 {
		ebitenutil.DebugPrint(screen, "Drop PNG files onto this window!")
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
		borderColor := color.RGBA{52, 13, 79, 240}
		vector.DrawFilledRect(screen, float32(v.x), float32(v.y), float32(v.w), 4, borderColor, false)
		vector.DrawFilledRect(screen, float32(v.x), float32(v.y+v.h), float32(v.w), 4, borderColor, false)
		vector.DrawFilledRect(screen, float32(v.x), float32(v.y), 4, float32(v.h), borderColor, false)
		vector.DrawFilledRect(screen, float32(v.x+v.w), float32(v.y), 4, float32(v.h), borderColor, false)

		// Draw resize handles
		handleColor := color.RGBA{52, 13, 79, 240}
		vector.DrawFilledRect(screen, float32(v.x-handleSize), float32(v.y-handleSize), float32(handleSize*2), float32(handleSize*2), handleColor, false)
		vector.DrawFilledRect(screen, float32(v.x+v.w-handleSize), float32(v.y-handleSize), float32(handleSize*2), float32(handleSize*2), handleColor, false)
		vector.DrawFilledRect(screen, float32(v.x-handleSize), float32(v.y+v.h-handleSize), float32(handleSize*2), float32(handleSize*2), handleColor, false)
		vector.DrawFilledRect(screen, float32(v.x+v.w-handleSize), float32(v.y+v.h-handleSize), float32(handleSize*2), float32(handleSize*2), handleColor, false)
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func main() {

	g := &Game{}

	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowTitle("visage")
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
