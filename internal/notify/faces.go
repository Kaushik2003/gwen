package notify

import (
	"bytes"
	"embed"
	"image"
	"image/draw"
	"image/png"
	"sync"

	"github.com/godbus/dbus/v5"
)

// faces are Gwen's mood sprites at notification size; faces/gen.go makes them
// from the dashboard's.
//
//go:embed faces/*.png
var faces embed.FS

// imageData is the image-data hint: width, height, rowstride, has alpha,
// bits per sample, channels, and RGBA bytes (signature (iiibiiay)).
type imageData struct {
	Width, Height, Stride int32
	Alpha                 bool
	Bits, Channels        int32
	Data                  []byte
}

var (
	faceMu    sync.Mutex
	faceCache = map[string]*imageData{}
)

// face is the image-data hint for a mood, or false for a mood with no sprite.
func face(mood string) (dbus.Variant, bool) {
	faceMu.Lock()
	defer faceMu.Unlock()
	img, seen := faceCache[mood]
	if !seen {
		img = decodeFace(mood)
		faceCache[mood] = img
	}
	if img == nil {
		return dbus.Variant{}, false
	}
	return dbus.MakeVariant(*img), true
}

func decodeFace(mood string) *imageData {
	if mood == "" {
		return nil
	}
	b, err := faces.ReadFile("faces/" + mood + ".png")
	if err != nil {
		return nil
	}
	src, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return nil
	}
	rgba := image.NewNRGBA(src.Bounds())
	draw.Draw(rgba, rgba.Bounds(), src, src.Bounds().Min, draw.Src)
	r := rgba.Bounds()
	return &imageData{Width: int32(r.Dx()), Height: int32(r.Dy()), Stride: int32(rgba.Stride), Alpha: true,
		Bits: 8, Channels: 4, Data: rgba.Pix}
}
