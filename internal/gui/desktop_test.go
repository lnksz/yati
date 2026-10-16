package gui

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopRegistrationAndAutostart(t *testing.T) {
	data, configuration := t.TempDir(), t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", configuration)
	executable := filepath.Join(t.TempDir(), "my yati")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := installDesktopAt(executable, true); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(data, "applications", applicationID+".desktop")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "Exec=\""+executable+"\" gui\n") {
		t.Fatalf("bad executable quoting: %s", contents)
	}
	if validator, err := exec.LookPath("desktop-file-validate"); err == nil {
		if output, err := exec.Command(validator, path).CombinedOutput(); err != nil {
			t.Fatalf("invalid desktop file: %v\n%s", err, output)
		}
	}
	if !AutostartEnabled() {
		t.Fatal("autostart was not installed")
	}
	if err := SetAutostart(false); err != nil {
		t.Fatal(err)
	}
	if AutostartEnabled() {
		t.Fatal("autostart was not removed")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("disabling autostart removed the application launcher")
	}
}

func TestDesktopIconTracksStateWithoutChangingLauncher(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := syncDesktopIcon(true); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(data, "applications", applicationID+".desktop")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("state updates should not install a launcher")
	}
	executable := "/example/my yati"
	if err := installDesktopAt(executable, false); err != nil {
		t.Fatal(err)
	}
	for _, running := range []bool{true, false} {
		if err := syncDesktopIcon(running); err != nil {
			t.Fatal(err)
		}
		if err := installDesktopAt(executable, false); err != nil {
			t.Fatal(err)
		}
		entry, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(entry), "\nIcon="+desktopIconName(running)+"\n") || !strings.Contains(string(entry), "Exec=\""+executable+"\" gui\n") {
			t.Fatalf("launcher did not preserve executable and timer state: %s", entry)
		}
		icon, err := os.ReadFile(filepath.Join(data, "icons", "hicolor", "scalable", "apps", desktopIconName(running)+".svg"))
		if err != nil || !bytes.Contains(icon, []byte("Toggl Track")) {
			t.Fatalf("missing Toggl icon: %v", err)
		}
	}
}

func TestTokenSaveIsPrivateAndPreservesOtherSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("YATI_CONFIG", path)
	if err := os.WriteFile(path, []byte(`{"pass_path":"toggl/token","future_setting":true}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := saveToken("a-secret-test-token"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]any
	if err := json.Unmarshal(data, &values); err != nil {
		t.Fatal(err)
	}
	if values["pass_path"] != "toggl/token" || values["future_setting"] != true || values["token"] != "a-secret-test-token" {
		t.Fatal("lost configuration settings")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("unsafe token-file permissions: %v", info.Mode())
	}
}

func TestDesktopHelpAndInvalidOptionsNeedNoCredentials(t *testing.T) {
	for _, test := range []struct {
		args []string
		code int
	}{{[]string{"--help"}, 0}, {[]string{"unexpected"}, 1}, {[]string{"--background", "--no-tray"}, 1},
		{[]string{"-d", "--quit"}, 1}, {[]string{"--detach", "--install"}, 1}, {[]string{"-d", "--autostart"}, 1}} {
		var stdout, stderr bytes.Buffer
		if code := Execute(test.args, &stdout, &stderr); code != test.code {
			t.Fatalf("%v: got %d, want %d", test.args, code, test.code)
		}
	}
}
