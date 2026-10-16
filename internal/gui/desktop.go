package gui

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"yati/internal/localfile"
)

const applicationID = "io.github.kristof.yati"

var desktopMu sync.Mutex

//go:embed assets/yati.svg
var applicationIcon []byte

//go:embed assets/yati-idle.svg
var idleIcon []byte

func desktopIconName(running bool) string {
	if running {
		return applicationID + "-tracking"
	}
	return applicationID + "-idle"
}

func installIconFiles(data string) error {
	for name, icon := range map[string][]byte{applicationID: applicationIcon, desktopIconName(true): applicationIcon, desktopIconName(false): idleIcon} {
		path := filepath.Join(data, "icons", "hicolor", "scalable", "apps", name+".svg")
		if previous, err := os.ReadFile(path); err == nil && bytes.Equal(previous, icon) {
			continue
		}
		if err := writeDesktopFile(path, icon); err != nil {
			return err
		}
	}
	return nil
}

func desktopDirectories() (string, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		data = filepath.Join(home, ".local", "share")
	}
	configuration, err := os.UserConfigDir()
	return data, configuration, err
}

// InstallDesktop writes a launcher/icon for this executable, without requiring root.
// Autostart is only enabled when explicitly requested.
func InstallDesktop(autostart bool) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return installDesktopAt(executable, autostart)
}

func installDesktopAt(executable string, autostart bool) error {
	desktopMu.Lock()
	defer desktopMu.Unlock()
	data, configuration, err := desktopDirectories()
	if err != nil {
		return err
	}
	quoted := desktopQuote(executable)
	path := filepath.Join(data, "applications", applicationID+".desktop")
	icon := desktopIconName(false)
	if previous, err := os.ReadFile(path); err == nil && strings.Contains(string(previous), "\nIcon="+desktopIconName(true)+"\n") {
		icon = desktopIconName(true)
	}
	entry := fmt.Sprintf("[Desktop Entry]\nType=Application\nName=Yati\nComment=Start, stop, and continue Toggl entries\nExec=%s gui\nIcon=%s\nTerminal=false\nCategories=Office;Utility;\nStartupNotify=true\n", quoted, icon)
	if err := writeDesktopFile(path, []byte(entry)); err != nil {
		return err
	}
	if err := installIconFiles(data); err != nil {
		return err
	}
	if autostart {
		entry = strings.Replace(entry, " gui\n", " gui --background\n", 1)
		return writeDesktopFile(filepath.Join(configuration, "autostart", applicationID+".desktop"), []byte(entry))
	}
	return nil
}

// syncDesktopIcon updates an installed launcher, preserving its executable and
// other settings. Distinct icon names let desktop shells invalidate cached art.
func syncDesktopIcon(running bool) error {
	desktopMu.Lock()
	defer desktopMu.Unlock()
	data, _, err := desktopDirectories()
	if err != nil {
		return err
	}
	path := filepath.Join(data, "applications", applicationID+".desktop")
	contents, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil // The user has not installed a desktop launcher.
	}
	if err != nil {
		return err
	}
	if err := installIconFiles(data); err != nil {
		return err
	}
	lines := strings.Split(string(contents), "\n")
	inside := false
	for i, line := range lines {
		if strings.HasPrefix(line, "[") {
			inside = line == "[Desktop Entry]"
		}
		if inside && strings.HasPrefix(line, "Icon=") {
			lines[i] = "Icon=" + desktopIconName(running)
			updated := []byte(strings.Join(lines, "\n"))
			if bytes.Equal(contents, updated) {
				return nil
			}
			return localfile.WritePrivate(path, updated)
		}
	}
	return nil
}

// AutostartEnabled reports whether Yati has its per-user login launcher installed.
func AutostartEnabled() bool {
	_, configuration, err := desktopDirectories()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(configuration, "autostart", applicationID+".desktop"))
	return err == nil
}

// SetAutostart changes only Yati's own login launcher.
func SetAutostart(enabled bool) error {
	if enabled {
		return InstallDesktop(true)
	}
	_, configuration, err := desktopDirectories()
	if err != nil {
		return err
	}
	err = os.Remove(filepath.Join(configuration, "autostart", applicationID+".desktop"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func writeDesktopFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func desktopQuote(value string) string {
	value = strings.ReplaceAll(value, "%", "%%")
	value = strings.NewReplacer("\\", "\\\\\\\\", "\"", "\\\\\"", "`", "\\\\`", "$", "\\\\$").Replace(value)
	return "\"" + value + "\""
}
