//go:build ignore

// gen draws the four tray icons — the stopwatch of the app icon in the state's
// colour on a transparent background — at 22 and 44 px.
// Run from this directory with: go run gen.go
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
)

var states = map[string]color.NRGBA{
	"off":     {0x6b, 0x72, 0x80, 0xff}, // grey
	"working": {0x10, 0xb9, 0x81, 0xff}, // green
	"idle":    {0xf5, 0x9e, 0x0b, 0xff}, // amber
	"break":   {0x3b, 0x82, 0xf6, 0xff}, // blue
}

// The geometry, in a 22-unit square: a filled dial with a white hand, and a
// crown on top.
const (
	size        = 22.0
	cx, cy      = 11.0, 12.5
	dialR       = 8.5
	handW       = 1.8
	handTipY    = 7.5
	crownX0     = 9.0
	crownX1     = 13.0
	crownY0     = 1.5
	crownY1     = 3.8
	stemX0      = 10.2
	stemX1      = 11.8
	stemBottomY = 5.0
	hubR        = 1.4
)

func main() {
	for name, c := range states {
		for _, px := range []int{22, 44} {
			if err := write(fmt.Sprintf("%s-%d.png", name, px), px, c); err != nil {
				log.Fatal(err)
			}
		}
	}
}

// sample reports whether (x, y) is covered, and whether it is the white hand.
func sample(x, y float64) (covered, white bool) {
	d := math.Hypot(x-cx, y-cy)
	if d <= dialR {
		return true, d <= hubR || segDist(x, y, cx, cy, cx, handTipY) <= handW/2
	}
	if x >= crownX0 && x <= crownX1 && y >= crownY0 && y <= crownY1 {
		return true, false
	}
	if x >= stemX0 && x <= stemX1 && y >= crownY1 && y <= stemBottomY {
		return true, false
	}
	return false, false
}

func segDist(x, y, x0, y0, x1, y1 float64) float64 {
	dx, dy := x1-x0, y1-y0
	t := math.Max(0, math.Min(1, ((x-x0)*dx+(y-y0)*dy)/(dx*dx+dy*dy)))
	return math.Hypot(x-(x0+t*dx), y-(y0+t*dy))
}

func write(name string, px int, c color.NRGBA) error {
	const ss = 8
	img := image.NewNRGBA(image.Rect(0, 0, px, px))
	scale := size / float64(px)
	for py := range px {
		for pxx := range px {
			var cov, wh float64
			for sy := range ss {
				for sx := range ss {
					covered, white := sample((float64(pxx)+(float64(sx)+0.5)/ss)*scale, (float64(py)+(float64(sy)+0.5)/ss)*scale)
					if covered {
						cov++
						if white {
							wh++
						}
					}
				}
			}
			if cov == 0 {
				continue
			}
			w := wh / cov
			mix := func(v uint8) uint8 { return uint8(math.Round(float64(v)*(1-w) + 255*w)) }
			img.SetNRGBA(pxx, py, color.NRGBA{mix(c.R), mix(c.G), mix(c.B), uint8(math.Round(255 * cov / (ss * ss)))})
		}
	}
	f, err := os.Create(name)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
