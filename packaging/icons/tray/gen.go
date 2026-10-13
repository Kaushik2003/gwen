//go:build ignore

// gen draws the four tray icons — the app logo at assets/branding/logo.jpg,
// identically for every state — at 22 and 44 px.
// Run from this directory with: go run gen.go
package main

import (
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"log"
	"os"

	"golang.org/x/image/draw"
)

const sourcePath = "../../../assets/branding/logo.jpg"

var states = []string{"off", "working", "idle", "break"}

func main() {
	src := loadSource()
	for _, name := range states {
		for _, px := range []int{22, 44} {
			if err := write(fmt.Sprintf("%s-%d.png", name, px), src, px); err != nil {
				log.Fatal(err)
			}
		}
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

func write(name string, src image.Image, px int) error {
	dst := image.NewNRGBA(image.Rect(0, 0, px, px))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	f, err := os.Create(name)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, dst)
}
