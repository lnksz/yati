//go:build !linux || !cgo || nogui

package gui

import (
	"fmt"
	"io"

	"yati/internal/config"
)

func runDesktop(_ *config.Config, _ Options, _ io.Writer) error {
	return fmt.Errorf("this binary has no GTK desktop support; on Linux, install libgtk-4-dev and libadwaita-1-dev and rebuild with CGO_ENABLED=1")
}

func detachDesktop(_ Options, _, _ string) (string, error) {
	return "", runDesktop(nil, Options{}, nil)
}
