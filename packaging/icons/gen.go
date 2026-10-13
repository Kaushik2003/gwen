//go:build ignore

// gen derives the Gwen app icon from the master artwork at
// assets/branding/logo.jpg, writing gwen.svg and 48/128 px PNGs here, the
// Wails build icon at cmd/gwen-ui/build/appicon.png, and the in-app brand
// mark at ui/src/assets/brand/logo.png.
// Run from this directory with: go run gen.go
package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"log"
	"os"

	"golang.org/x/image/draw"
)

const sourcePath = "../../assets/branding/logo.jpg"

func main() {
	src := loadSource()

	for _, px := range []int{48, 128} {
		resized := resize(src, px)
		if err := writePNG(fmt.Sprintf("gwen-%d.png", px), resized); err != nil {
			log.Fatal(err)
		}
	}

	appicon := resize(src, 128)
	if err := writePNG("../../cmd/gwen-ui/build/appicon.png", appicon); err != nil {
		log.Fatal(err)
	}
	if err := writePNG("../../ui/src/assets/brand/logo.png", appicon); err != nil {
		log.Fatal(err)
	}

	if err := os.WriteFile("gwen.svg", []byte(svg(appicon)), 0o644); err != nil {
		log.Fatal(err)
	}
}

// loadSource reads the master artwork and centre-crops it to a square.
func loadSource() image.Image {
	f, err := os.Open(sourcePath)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		log.Fatal(err)
	}
	b := img.Bounds()
	side := min(b.Dx(), b.Dy())
	x0 := b.Min.X + (b.Dx()-side)/2
	y0 := b.Min.Y + (b.Dy()-side)/2
	crop := image.Rect(x0, y0, x0+side, y0+side)
	square := image.NewRGBA(image.Rect(0, 0, side, side))
	draw.Draw(square, square.Bounds(), img, crop.Min, draw.Src)
	return square
}

// resize scales the square source down to px×px with high-quality
// interpolation.
func resize(src image.Image, px int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, px, px))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	return dst
}

func writePNG(name string, img image.Image) error {
	f, err := os.Create(name)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// svg wraps the 128 px PNG as a base64 data URI so the scalable hicolor
// slot still resolves to real artwork.
func svg(img image.Image) string {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		log.Fatal(err)
	}
	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="128" height="128" viewBox="0 0 128 128">
  <image width="128" height="128" href="data:image/png;base64,%s"/>
</svg>
`, encoded)
}
