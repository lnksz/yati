package gui

import (
	"fmt"
	"os"
	"strconv"
)

const startupFDEnvironment = "YATI_GUI_STARTUP_FD"

type startupNotifier struct{ file *os.File }

func newStartupNotifier() (*startupNotifier, error) {
	s := &startupNotifier{}
	value, exists := os.LookupEnv(startupFDEnvironment)
	if !exists {
		return s, nil
	}
	if err := os.Unsetenv(startupFDEnvironment); err != nil {
		return nil, err
	}
	fd, err := strconv.ParseUint(value, 10, 32)
	if err != nil || fd < 3 {
		return nil, fmt.Errorf("invalid desktop startup descriptor")
	}
	s.file = os.NewFile(uintptr(fd), "yati-desktop-startup")
	if s.file == nil {
		return nil, fmt.Errorf("desktop startup descriptor is unavailable")
	}
	return s, nil
}

func (s *startupNotifier) Ready() {
	if s.file != nil {
		_, _ = s.file.WriteString("ready\n") // A launcher that already exited needs no acknowledgement.
		s.Close()
	}
}

func (s *startupNotifier) Close() {
	if s.file != nil {
		_ = s.file.Close() // Best effort when the launcher has disconnected.
		s.file = nil
	}
}
