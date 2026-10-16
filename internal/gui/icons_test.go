//go:build linux && cgo && !nogui

package gui

import "testing"

func TestTrayPixelsMatchTimerState(t *testing.T) {
	for _, running := range []bool{false, true} {
		pixmaps, err := trayPixmaps(running)
		if err != nil {
			t.Fatal(err)
		}
		if len(pixmaps) != 5 {
			t.Fatalf("expected five icon sizes, got %d", len(pixmaps))
		}
		for _, pixmap := range pixmaps {
			if len(pixmap.Data) != int(pixmap.Width*pixmap.Height*4) {
				t.Fatal("invalid ARGB payload length")
			}
			colored, opaque := false, false
			for i := 0; i < len(pixmap.Data); i += 4 {
				if pixmap.Data[i] > 200 {
					opaque = true
					colored = colored || pixmap.Data[i+1] != pixmap.Data[i+2] || pixmap.Data[i+2] != pixmap.Data[i+3]
				}
			}
			if !opaque || colored != running {
				t.Fatalf("%dpx icon: running=%v, colored=%v, opaque=%v", pixmap.Width, running, colored, opaque)
			}
		}
	}
}
