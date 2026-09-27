//go:build ignore

// gen draws the Gwen app icon — a rounded square in #10b981 with a white
// stopwatch — as gwen.svg and as 48 and 128 px PNGs, all from one geometry.
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

// The geometry, in a 128-unit square.
const (
	size       = 128.0
	corner     = 28.0
	cx, cy     = 64.0, 70.0 // dial centre
	dialR      = 33.0       // dial ring radius, to the middle of the stroke
	ringW      = 9.0
	handW      = 8.0
	handTipY   = 47.0
	hubR       = 6.5
	crownX0    = 55.0
	crownX1    = 73.0
	crownY0    = 17.0
	crownY1    = 27.0
	stemX0     = 60.0
	stemX1     = 68.0
	stemY1     = 38.0
	crownRound = 3.0
)

var green = color.NRGBA{0x10, 0xb9, 0x81, 0xff}

func main() {
	if err := os.WriteFile("gwen.svg", []byte(svg()), 0o644); err != nil {
		log.Fatal(err)
	}
	for _, px := range []int{48, 128} {
		if err := writePNG(fmt.Sprintf("gwen-%d.png", px), px); err != nil {
			log.Fatal(err)
		}
	}
}

func svg() string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="128" height="128" viewBox="0 0 128 128">
  <rect width="128" height="128" rx="%g" fill="#10b981"/>
  <g fill="none" stroke="#fff" stroke-linecap="round">
    <circle cx="%g" cy="%g" r="%g" stroke-width="%g"/>
    <line x1="%g" y1="%g" x2="%g" y2="%g" stroke-width="%g"/>
  </g>
  <g fill="#fff">
    <rect x="%g" y="%g" width="%g" height="%g" rx="%g"/>
    <rect x="%g" y="%g" width="%g" height="%g"/>
    <circle cx="%g" cy="%g" r="%g"/>
  </g>
</svg>
`, corner, cx, cy, dialR, ringW, cx, cy, cx, handTipY, handW,
		crownX0, crownY0, crownX1-crownX0, crownY1-crownY0, crownRound,
		stemX0, crownY1-1, stemX1-stemX0, stemY1-crownY1+1, cx, cy, hubR)
}

// shade reports whether (x, y) is inside the rounded square, and if so
// whether it is part of the white stopwatch.
func shade(x, y float64) (inside bool, white bool) {
	if !inRoundRect(x, y, 0, 0, size, size, corner) {
		return false, false
	}
	d := math.Hypot(x-cx, y-cy)
	switch {
	case math.Abs(d-dialR) <= ringW/2,
		d <= hubR,
		segmentDist(x, y, cx, cy, cx, handTipY) <= handW/2,
		inRoundRect(x, y, crownX0, crownY0, crownX1, crownY1, crownRound),
		x >= stemX0 && x <= stemX1 && y >= crownY1-1 && y <= stemY1:
		return true, true
	}
	return true, false
}

func inRoundRect(x, y, x0, y0, x1, y1, r float64) bool {
	if x < x0 || x > x1 || y < y0 || y > y1 {
		return false
	}
	qx := math.Max(math.Max(x0+r-x, x-(x1-r)), 0)
	qy := math.Max(math.Max(y0+r-y, y-(y1-r)), 0)
	return qx*qx+qy*qy <= r*r
}

func segmentDist(x, y, x0, y0, x1, y1 float64) float64 {
	dx, dy := x1-x0, y1-y0
	t := math.Max(0, math.Min(1, ((x-x0)*dx+(y-y0)*dy)/(dx*dx+dy*dy)))
	return math.Hypot(x-(x0+t*dx), y-(y0+t*dy))
}

// writePNG renders px×px with 8×8 supersampling for smooth edges.
func writePNG(name string, px int) error {
	const ss = 8
	img := image.NewNRGBA(image.Rect(0, 0, px, px))
	scale := size / float64(px)
	for py := range px {
		for pxx := range px {
			var in, wh float64
			for sy := range ss {
				for sx := range ss {
					x := (float64(pxx) + (float64(sx)+0.5)/ss) * scale
					y := (float64(py) + (float64(sy)+0.5)/ss) * scale
					inside, white := shade(x, y)
					if inside {
						in++
						if white {
							wh++
						}
					}
				}
			}
			if in == 0 {
				continue
			}
			w := wh / in // white share of the covered area
			mix := func(g uint8) uint8 { return uint8(math.Round(float64(g)*(1-w) + 255*w)) }
			img.SetNRGBA(pxx, py, color.NRGBA{mix(green.R), mix(green.G), mix(green.B),
				uint8(math.Round(255 * in / (ss * ss)))})
		}
	}
	f, err := os.Create(name)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
