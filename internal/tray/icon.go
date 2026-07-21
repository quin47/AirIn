package tray

import (
	"image"
	"image/color"
)

type pixmap struct {
	Width  int32
	Height int32
	Pixels []byte
}

type SNITooltip struct {
	IconName string
	IconData []pixmap
	Title    string
	Desc     string
}

func generateCirclePixels(c color.Color, size int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	center := size / 2
	radius := size/2 - 3

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := x - center
			dy := y - center
			if dx*dx+dy*dy <= radius*radius {
				img.Set(x, y, c)
			}
		}
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	out := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			idx := (y*w + x) * 4
			r, g, b, a := img.At(x, y).RGBA()
			out[idx+0] = byte(a >> 8)
			out[idx+1] = byte(r >> 8)
			out[idx+2] = byte(g >> 8)
			out[idx+3] = byte(b >> 8)
		}
	}
	return out
}

func generateSNIIconPixmap() []pixmap {
	gray := generateCirclePixels(color.NRGBA{R: 128, G: 128, B: 128, A: 255}, 32)
	return []pixmap{{Width: 32, Height: 32, Pixels: gray}}
}

func buildTooltip(status string) SNITooltip {
	return SNITooltip{
		IconName: "",
		IconData: []pixmap{},
		Title:    "AirIn",
		Desc:     status,
	}
}
