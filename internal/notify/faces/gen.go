//go:build ignore

// gen shrinks the dashboard's mood sprites (ui/src/assets/gwen) to the 128 px
// faces desktop notifications show. Run from this directory with: go run gen.go
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const maxSide = 128

func main() {
	srcs, err := filepath.Glob("../../../ui/src/assets/gwen/*.png")
	if err != nil || len(srcs) == 0 {
		log.Fatalf("no sprites found: %v", err)
	}
	for _, src := range srcs {
		if err := shrink(src, strings.TrimSuffix(filepath.Base(src), ".png")+".png"); err != nil {
			log.Fatal(err)
		}
	}
}

func shrink(src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	in, err := png.Decode(f)
	if err != nil {
		return err
	}
	b := in.Bounds()
	scale := float64(max(b.Dx(), b.Dy())) / maxSide
	w, h := int(float64(b.Dx())/scale+0.5), int(float64(b.Dy())/scale+0.5)
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	// Area-average each output pixel over its source box, in premultiplied
	// alpha so transparent edges do not darken.
	for y := range h {
		y0, y1 := int(float64(y)*scale), int(float64(y+1)*scale)
		for x := range w {
			x0, x1 := int(float64(x)*scale), int(float64(x+1)*scale)
			var r, g, bl, a, n float64
			for sy := y0; sy < max(y1, y0+1); sy++ {
				for sx := x0; sx < max(x1, x0+1); sx++ {
					c := color.NRGBAModel.Convert(in.At(b.Min.X+sx, b.Min.Y+sy)).(color.NRGBA)
					al := float64(c.A)
					r, g, bl, a, n = r+float64(c.R)*al, g+float64(c.G)*al, bl+float64(c.B)*al, a+al, n+1
				}
			}
			if a == 0 {
				continue
			}
			out.SetNRGBA(x, y, color.NRGBA{uint8(r/a + 0.5), uint8(g/a + 0.5), uint8(bl/a + 0.5), uint8(a/n + 0.5)})
		}
	}
	o, err := os.Create(dst)
	if err != nil {
		return err
	}
	if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(o, out); err != nil {
		o.Close()
		return err
	}
	return o.Close()
}
