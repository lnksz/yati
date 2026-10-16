//go:build linux && cgo && !nogui

package gui

import (
	"bytes"
	"embed"
	"fmt"
	"image/color"
	"image/png"
)

//go:embed assets/toggl-*.png
var trayImages embed.FS

func trayPixmaps(running bool) ([]iconPixmap, error) {
	state := "idle"
	if running {
		state = "tracking"
	}
	var pixmaps []iconPixmap
	for _, size := range []int{16, 24, 32, 48, 64} {
		data, err := trayImages.ReadFile(fmt.Sprintf("assets/toggl-%s-%d.png", state, size))
		if err != nil {
			return nil, err
		}
		bitmap, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		// SNI uses unpremultiplied ARGB32 in network byte order.
		pixels := make([]byte, 0, size*size*4)
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				pixel := color.NRGBAModel.Convert(bitmap.At(x, y)).(color.NRGBA)
				pixels = append(pixels, pixel.A, pixel.R, pixel.G, pixel.B)
			}
		}
		pixmaps = append(pixmaps, iconPixmap{int32(size), int32(size), pixels})
	}
	return pixmaps, nil
}
