//go:build linux && cgo && !nogui

package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"yati/internal/config"
	"yati/internal/toggl"
	"yati/internal/tracker"
)

type desktopFixture struct {
	mu           sync.Mutex
	entries      []toggl.TimeEntry
	current      *toggl.TimeEntry
	calls        []string
	app          *desktopApp
	startRelease chan struct{}
}

func (f *desktopFixture) Profile(context.Context) (*toggl.Me, error) {
	return &toggl.Me{ID: 1, DefaultWorkspace: 42}, nil
}
func (f *desktopFixture) Projects(context.Context, int64, []toggl.TimeEntry) ([]toggl.Project, error) {
	return []toggl.Project{{ID: 1, WorkspaceID: 42, Name: "Firmware", Active: true}, {ID: 2, WorkspaceID: 42, Name: "Platform / API", Active: true}, {ID: 3, WorkspaceID: 42, Name: "Archived project", Active: false}}, nil
}
func (f *desktopFixture) Entries(context.Context, time.Time, time.Time) ([]toggl.TimeEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]toggl.TimeEntry(nil), f.entries...), nil
}
func (f *desktopFixture) Current(context.Context) (*toggl.TimeEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.current, nil
}
func (f *desktopFixture) Start(ctx context.Context, workspace int64, project *int64, description string) (*toggl.TimeEntry, error) {
	f.mu.Lock()
	release := f.startRelease
	f.mu.Unlock()
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "start")
	f.current = &toggl.TimeEntry{ID: int64(100 + len(f.calls)), WorkspaceID: workspace, ProjectID: project, Description: description, Start: time.Now(), Duration: -1}
	f.entries = append(f.entries, *f.current)
	return f.current, nil
}

func (f *desktopFixture) BlockStart(block bool) *dbus.Error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if block && f.startRelease == nil {
		f.startRelease = make(chan struct{})
	} else if !block && f.startRelease != nil {
		close(f.startRelease)
		f.startRelease = nil
	}
	return nil
}
func (f *desktopFixture) Stop(_ context.Context, workspace, id int64) (*toggl.TimeEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.current == nil || f.current.ID != id {
		return nil, fmt.Errorf("fixture timer changed")
	}
	f.calls = append(f.calls, "stop")
	stopped := *f.current
	stopped.Duration = int64(time.Since(stopped.Start) / time.Second)
	f.entries = replaceFixtureEntry(f.entries, stopped)
	f.current = nil
	return &stopped, nil
}
func replaceFixtureEntry(entries []toggl.TimeEntry, entry toggl.TimeEntry) []toggl.TimeEntry {
	for i := range entries {
		if entries[i].ID == entry.ID {
			entries[i] = entry
			return entries
		}
	}
	return append(entries, entry)
}

type fixtureView struct {
	Busy             bool
	Page             string
	Query            string
	Current          *toggl.TimeEntry
	Calls            []string
	Error            string
	Width            int
	Height           int
	Transitioning    bool
	HistoryDetails   map[int64]string
	ShortcutsVisible bool
}

func (f *desktopFixture) View() (string, *dbus.Error) {
	result := make(chan string, 1)
	f.app.post(func() {
		f.mu.Lock()
		calls := append([]string(nil), f.calls...)
		f.mu.Unlock()
		width, height, transitioning := nativeFrame()
		details := make(map[int64]string)
		for i, entry := range f.app.history {
			details[entry.ID] = nativeHistoryDetail(i)
		}
		data, err := json.Marshal(fixtureView{Busy: f.app.busy, Page: f.app.page, Query: f.app.query,
			Current: f.app.state().Current, Calls: calls, Error: f.app.errorText,
			Width: width, Height: height, Transitioning: transitioning, HistoryDetails: details, ShortcutsVisible: nativeShortcutsVisible()})
		if err != nil {
			result <- "{}"
		} else {
			result <- string(data)
		}
	})
	select {
	case data := <-result:
		return data, nil
	case <-time.After(3 * time.Second):
		return "", dbus.MakeFailedError(fmt.Errorf("UI did not respond"))
	}
}

// TestDesktopFixture is only launched by the isolated integration harness. Its
// backend cannot contact Toggl, even if real credentials exist on the machine.
func TestDesktopFixture(t *testing.T) {
	if os.Getenv("YATI_GUI_FIXTURE") != "1" {
		t.Skip("subprocess fixture")
	}
	var connections []*dbus.Conn
	newBackend = func(_, _ string) tracker.Backend {
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		if err := installDesktopAt(executable, false); err != nil {
			t.Fatal(err)
		}
		one, two := int64(1), int64(2)
		now := time.Now()
		current := &toggl.TimeEntry{ID: 10, WorkspaceID: 42, ProjectID: &one, Description: "[Test data] Review sensor firmware", Start: now.Add(-37*time.Minute - 12*time.Second), Duration: -1}
		f := &desktopFixture{app: activeApp, current: current, entries: []toggl.TimeEntry{
			*current,
			{ID: 11, WorkspaceID: 42, ProjectID: &two, Description: "Investigate OAuth callback", Start: now.Add(-2 * time.Hour), Duration: 3720},
			{ID: 12, WorkspaceID: 42, ProjectID: &one, Description: "Überprüfen <API> & UI", Start: now.Add(-4 * time.Hour), Duration: 2520},
			{ID: 13, WorkspaceID: 42, Description: "Planning and notes", Start: now.Add(-25 * time.Hour), Duration: 1800},
			{ID: 14, WorkspaceID: 42, ProjectID: &two, Description: "Review sensor firmware", Start: now.Add(-48 * time.Hour), Duration: 5400},
		}}
		conn, err := dbus.ConnectSessionBus()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.RequestName(applicationID+".Fixture", dbus.NameFlagDoNotQueue); err != nil {
			t.Fatal(err)
		}
		if err := conn.Export(f, "/Fixture", applicationID+".Fixture"); err != nil {
			t.Fatal(err)
		}
		connections = append(connections, conn)
		return f
	}
	defer func() {
		for _, conn := range connections {
			_ = conn.Close() // Test fixture is exiting.
		}
	}()
	if os.Getenv("YATI_GUI_MODE") == "detached" {
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err = startDetached(ctx, executable, []string{"-test.run=^TestDesktopFixture$"}, append(os.Environ(), "YATI_GUI_MODE=open"), filepath.Join(os.Getenv("XDG_STATE_HOME"), "yati", "gui.log"))
		if err != nil {
			t.Fatal(err)
		}
		return
	}
	startup, err := newStartupNotifier()
	if err != nil {
		t.Fatal(err)
	}
	defer startup.Close()
	options := Options{Background: os.Getenv("YATI_GUI_MODE") == "background", Toggle: os.Getenv("YATI_GUI_MODE") == "toggle", Quit: os.Getenv("YATI_GUI_MODE") == "quit"}
	options.startupReady = startup.Ready
	if err := runDesktop(&config.Config{Token: "synthetic-test-token"}, options, os.Stderr); err != nil {
		t.Fatal(err)
	}
	startup.Ready()
}

func TestDesktopInteraction(t *testing.T) {
	if os.Getenv("YATI_GUI_INTEGRATION") != "1" {
		t.Skip("run scripts/test-gui.sh for native interaction tests")
	}
	for _, name := range []string{"xdotool", "import"} {
		if _, err := exec.LookPath(name); err != nil {
			t.Skip(name + " unavailable")
		}
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	log, err := os.Create(filepath.Join(directory, "desktop.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = log.Close() }() // Test output cleanup.
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	launch := func(mode string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, executable, "-test.run=^TestDesktopFixture$")
		cmd.Env = append(os.Environ(), "YATI_GUI_FIXTURE=1", "YATI_GUI_MODE="+mode, "XDG_CACHE_HOME="+filepath.Join(directory, "cache"), "XDG_CONFIG_HOME="+filepath.Join(directory, "config"), "XDG_DATA_HOME="+filepath.Join(directory, "data"))
		cmd.Stdout, cmd.Stderr = log, log
		return cmd
	}
	primary := launch("open")
	if err := primary.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = primary.Process.Kill() // It may already have exited via remote Quit.
		_ = primary.Wait()         // Cleanup also covers deliberately killed failed fixtures.
		if t.Failed() {
			data, _ := os.ReadFile(log.Name())
			t.Log(string(data))
		}
	}()
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }() // Private test bus cleanup.
	view := func() fixtureView {
		var data string
		call := conn.Object(applicationID+".Fixture", "/Fixture").CallWithContext(ctx, applicationID+".Fixture.View", 0)
		if call.Store(&data) != nil {
			return fixtureView{Busy: true}
		}
		var result fixtureView
		if json.Unmarshal([]byte(data), &result) != nil {
			return fixtureView{Busy: true}
		}
		return result
	}
	await := func(description string, predicate func() bool) {
		t.Helper()
		ticker := time.NewTicker(30 * time.Millisecond)
		defer ticker.Stop()
		deadline := time.NewTimer(10 * time.Second)
		defer deadline.Stop()
		for {
			if predicate() {
				return
			}
			select {
			case <-ticker.C:
			case <-deadline.C:
				t.Logf("last UI state: %+v", view())
				t.Fatalf("timed out: %s", description)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		}
	}
	await("initial state", func() bool { v := view(); return !v.Busy && v.Current != nil })
	var window string
	await("visible GTK window", func() bool {
		output, err := exec.CommandContext(ctx, "xdotool", "search", "--onlyvisible", "--name", "^Yati$").Output()
		window = strings.TrimSpace(string(output))
		return err == nil && window != ""
	})
	x := func(args ...string) {
		t.Helper()
		if output, err := exec.CommandContext(ctx, "xdotool", args...).CombinedOutput(); err != nil {
			t.Fatalf("xdotool %v: %v\n%s", args, err, output)
		}
	}
	x("windowfocus", "--sync", window)
	trayName := fmt.Sprintf("org.kde.StatusNotifierItem-%d-1", primary.Process.Pid)
	launcher := filepath.Join(directory, "data", "applications", applicationID+".desktop")
	awaitIcon := func(running bool) {
		t.Helper()
		defer func() {
			if t.Failed() {
				colored, err := trayIconColored(ctx, conn, trayName)
				data, readErr := os.ReadFile(launcher)
				t.Logf("icon state: colored=%v err=%v launcher=%s readErr=%v", colored, err, data, readErr)
			}
		}()
		await("tray and launcher timer state", func() bool {
			colored, err := trayIconColored(ctx, conn, trayName)
			data, readErr := os.ReadFile(launcher)
			return err == nil && readErr == nil && colored == running && strings.Contains(string(data), "\nIcon="+desktopIconName(running)+"\n")
		})
	}
	awaitIcon(true)
	initialWidth, initialHeight := view().Width, view().Height
	screenshot := func(name string) {
		t.Helper()
		path := os.Getenv("YATI_GUI_EVIDENCE")
		if path == "" {
			return
		}
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
		if output, err := exec.CommandContext(ctx, "import", "-screen", "-window", window, filepath.Join(path, name+".png")).CombinedOutput(); err != nil {
			t.Fatalf("screenshot: %v %s", err, output)
		}
	}
	screenshot("quick-entry")
	x("windowsize", window, "440", "650")
	await("narrow layout allocated", func() bool {
		v := view()
		return v.Width == initialWidth-200 && v.Height == initialHeight-40 && !v.Transitioning
	})
	// Exercise the scrollbar's hover state while the recent list overflows.
	x("mousemove", "--window", window, "399", "470")
	x("mousedown", "1")
	screenshot("narrow")
	x("mouseup", "1")
	x("mousemove", "1000", "900") // Leave the app so a header tooltip cannot mask later window searches.
	x("windowsize", window, "640", "690")
	await("normal layout allocated", func() bool {
		v := view()
		return v.Width == initialWidth && v.Height == initialHeight && !v.Transitioning
	})
	x("key", "--clearmodifiers", "question")
	await("question mark opens shortcuts", func() bool { return view().ShortcutsVisible })
	var helpWindow string
	await("shortcuts window allocated", func() bool {
		out, err := exec.CommandContext(ctx, "xdotool", "search", "--onlyvisible", "--name", "^Keyboard shortcuts$").Output()
		helpWindow = strings.TrimSpace(string(out))
		return err == nil && helpWindow != ""
	})
	x("windowfocus", "--sync", helpWindow)
	if path := os.Getenv("YATI_GUI_EVIDENCE"); path != "" {
		if out, err := exec.CommandContext(ctx, "import", "-window", helpWindow, filepath.Join(path, "shortcuts.png")).CombinedOutput(); err != nil {
			t.Fatalf("shortcuts screenshot: %v %s", err, out)
		}
	}
	x("key", "--clearmodifiers", "Escape")
	await("Escape closes shortcuts", func() bool { return !view().ShortcutsVisible })
	x("windowfocus", "--sync", window)
	x("type", "--clearmodifiers", "--delay", "0", "Why?")
	await("question marks remain usable in descriptions", func() bool { return view().Query == "Why?" })
	x("key", "--clearmodifiers", "ctrl+question")
	await("Ctrl question opens shortcuts while typing", func() bool { return view().ShortcutsVisible })
	x("windowfocus", "--sync", helpWindow)
	x("key", "--clearmodifiers", "ctrl+s")
	await("Ctrl S stops the timer while help is open", func() bool { v := view(); return !v.Busy && v.Current == nil && v.ShortcutsVisible })
	awaitIcon(false)
	x("key", "--clearmodifiers", "Escape")
	await("close shortcuts without losing draft", func() bool { v := view(); return !v.ShortcutsVisible && v.Query == "Why?" })
	x("windowfocus", "--sync", window)
	x("key", "--clearmodifiers", "ctrl+l", "ctrl+a", "BackSpace")
	x("key", "--clearmodifiers", "F10", "Tab", "Tab", "Return")
	await("app menu opens keyboard shortcuts", func() bool { return view().ShortcutsVisible })
	x("windowfocus", "--sync", helpWindow)
	x("key", "--clearmodifiers", "Escape")
	await("menu shortcuts dismissed", func() bool { return !view().ShortcutsVisible })
	x("windowfocus", "--sync", window)
	x("key", "--clearmodifiers", "ctrl+l")
	x("type", "--clearmodifiers", "--delay", "0", "OAuth")
	await("local autocomplete", func() bool { return view().Query == "OAuth" })
	x("key", "--clearmodifiers", "Return")
	await("resume preserves project", func() bool {
		v := view()
		return !v.Busy && v.Current != nil && v.Current.Description == "Investigate OAuth callback" && v.Current.ProjectID != nil && *v.Current.ProjectID == 2
	})
	if calls := view().Calls; strings.Join(calls, ",") != "stop,start" {
		t.Fatalf("switch ordering: %v", calls)
	}
	x("key", "--clearmodifiers", "ctrl+s")
	await("stop keyboard action", func() bool { v := view(); return !v.Busy && v.Current == nil })
	awaitIcon(false)
	x("key", "--clearmodifiers", "ctrl+l")
	x("type", "--clearmodifiers", "--delay", "0", "Unrelated search")
	x("key", "--clearmodifiers", "ctrl+shift+c")
	await("continue last shortcut preserves project independently of search", func() bool {
		v := view()
		return !v.Busy && v.Current != nil && v.Current.Description == "Investigate OAuth callback" && v.Current.ProjectID != nil && *v.Current.ProjectID == 2
	})
	x("key", "--clearmodifiers", "ctrl+s")
	await("stop continued entry", func() bool { v := view(); return !v.Busy && v.Current == nil })
	x("key", "--clearmodifiers", "ctrl+l")
	x("type", "--clearmodifiers", "--delay", "0", "Brand new task")
	fixture := conn.Object(applicationID+".Fixture", "/Fixture")
	if err := fixture.CallWithContext(ctx, applicationID+".Fixture.BlockStart", 0, true).Err; err != nil {
		t.Fatal(err)
	}
	x("key", "--clearmodifiers", "ctrl+Return")
	await("delayed start pending", func() bool { return view().Busy })
	x("key", "--clearmodifiers", "ctrl+l")
	x("key", "--clearmodifiers", "ctrl+a")
	x("type", "--clearmodifiers", "--delay", "0", "A newer draft")
	await("edit draft during pending start", func() bool { return view().Query == "A newer draft" })
	if err := fixture.CallWithContext(ctx, applicationID+".Fixture.BlockStart", 0, false).Err; err != nil {
		t.Fatal(err)
	}
	await("new entry", func() bool {
		v := view()
		return !v.Busy && v.Current != nil && v.Current.Description == "Brand new task" && v.Current.ProjectID == nil
	})
	awaitIcon(true)
	if view().Query != "A newer draft" {
		t.Fatal("start completion erased a newer draft")
	}
	x("key", "--clearmodifiers", "ctrl+l", "ctrl+a", "BackSpace")
	await("clear history filter", func() bool { return view().Query == "" })
	x("key", "--clearmodifiers", "ctrl+2")
	await("history view", func() bool { v := view(); return v.Page == "history" && !v.Transitioning })
	currentID := view().Current.ID
	detail := view().HistoryDetails[currentID]
	if detail == "" {
		t.Fatal("running history row has no duration")
	}
	await("running history row advances without rebuilding", func() bool {
		return view().HistoryDetails[currentID] != detail
	})
	screenshot("history")
	x("key", "--clearmodifiers", "Escape")
	await("Escape hides window", func() bool {
		out, _ := exec.CommandContext(ctx, "xdotool", "search", "--onlyvisible", "--name", "^Yati$").Output()
		return strings.TrimSpace(string(out)) == ""
	})
	if err := launch("open").Run(); err != nil {
		t.Fatalf("second activation: %v", err)
	}
	await("second launch presents existing window", func() bool {
		out, _ := exec.CommandContext(ctx, "xdotool", "search", "--onlyvisible", "--name", "^Yati$").Output()
		return strings.TrimSpace(string(out)) == window
	})
	// The tray uses the same controller, including disabled-state checks.
	if err := conn.Object(trayName, menuPath).CallWithContext(ctx, menuInterface+".Event", 0, int32(3), "clicked", dbus.MakeVariant(int32(0)), uint32(0)).Err; err != nil {
		t.Fatal(err)
	}
	await("tray stop", func() bool { v := view(); return !v.Busy && v.Current == nil })
	if err := conn.Object(trayName, menuPath).CallWithContext(ctx, menuInterface+".Event", 0, int32(4), "clicked", dbus.MakeVariant(int32(0)), uint32(0)).Err; err != nil {
		t.Fatal(err)
	}
	await("tray continue", func() bool {
		v := view()
		return !v.Busy && v.Current != nil && v.Current.Description == "Brand new task"
	})
	x("windowfocus", "--sync", window)
	x("key", "--clearmodifiers", "ctrl+s")
	await("stop before continue from help", func() bool { v := view(); return !v.Busy && v.Current == nil })
	x("key", "--clearmodifiers", "question")
	await("open help for continue shortcut", func() bool { return view().ShortcutsVisible })
	x("windowfocus", "--sync", helpWindow)
	x("key", "--clearmodifiers", "ctrl+shift+c")
	await("continue last shortcut works while help is open", func() bool {
		v := view()
		return !v.Busy && v.ShortcutsVisible && v.Current != nil && v.Current.Description == "Brand new task" && v.Current.ProjectID == nil
	})
	x("key", "--clearmodifiers", "Escape")
	await("close help before choosing project", func() bool { return !view().ShortcutsVisible })
	x("windowfocus", "--sync", window)
	x("key", "--clearmodifiers", "ctrl+1", "ctrl+l")
	x("type", "--clearmodifiers", "--delay", "0", "Project picker test")
	filterProjects := func(query string) {
		t.Helper()
		x("mousemove", "--window", window, "240", "267")
		x("click", "1")
		x("type", "--clearmodifiers", "--delay", "20", query)
		// GTK debounces the popup search before updating its filtered model.
		x("sleep", "0.3")
	}
	filterProjects("api")
	screenshot("project-filter")
	x("key", "--clearmodifiers", "Down", "Return")
	x("key", "--clearmodifiers", "ctrl+l", "ctrl+Return")
	await("project filter selects a case-insensitive substring match", func() bool {
		v := view()
		return !v.Busy && v.Current != nil && v.Current.Description == "Project picker test" &&
			v.Current.ProjectID != nil && *v.Current.ProjectID == 2
	})
	filterProjects("firm")
	x("mousemove", "--window", window, "240", "216")
	x("click", "1") // Dismiss the popup without choosing the matching project.
	x("key", "--clearmodifiers", "ctrl+l")
	x("type", "--clearmodifiers", "--delay", "0", "Keep selected project")
	x("key", "--clearmodifiers", "ctrl+Return")
	await("cancelling project search preserves the selection", func() bool {
		v := view()
		return !v.Busy && v.Current != nil && v.Current.Description == "Keep selected project" &&
			v.Current.ProjectID != nil && *v.Current.ProjectID == 2
	})
	filterProjects("no such project")
	screenshot("project-no-match")
	x("key", "--clearmodifiers", "ctrl+a", "BackSpace")
	x("sleep", "0.3")
	screenshot("project-filter-cleared")
	x("type", "--clearmodifiers", "--delay", "20", "no project")
	x("sleep", "0.3")
	x("key", "--clearmodifiers", "Down", "Return")
	x("key", "--clearmodifiers", "ctrl+l")
	x("type", "--clearmodifiers", "--delay", "0", "Without project")
	x("key", "--clearmodifiers", "ctrl+Return")
	await("clearing a no-match search restores project choices", func() bool {
		v := view()
		return !v.Busy && v.Current != nil && v.Current.Description == "Without project" && v.Current.ProjectID == nil
	})
	if err := launch("quit").Run(); err != nil {
		t.Fatalf("remote quit: %v", err)
	}
	await("application releases bus name", func() bool {
		var owned bool
		return conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, applicationID).Store(&owned) == nil && !owned
	})
}

func integrationEnabled() bool { return os.Getenv("YATI_GUI_INTEGRATION") == "1" }

func TestDesktopDetachment(t *testing.T) {
	if !integrationEnabled() {
		t.Skip("requires isolated display and session bus")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	launch := func(mode string) error {
		cmd := exec.CommandContext(ctx, executable, "-test.run=^TestDesktopFixture$")
		cmd.Env = append(os.Environ(), "YATI_GUI_FIXTURE=1", "YATI_GUI_MODE="+mode,
			"XDG_CONFIG_HOME="+filepath.Join(directory, "config"), "XDG_CACHE_HOME="+filepath.Join(directory, "cache"),
			"XDG_DATA_HOME="+filepath.Join(directory, "data"), "XDG_STATE_HOME="+filepath.Join(directory, "state"))
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("fixture %s: %w\n%s", mode, err, output)
		}
		return nil
	}
	if err := launch("detached"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := launch("quit"); err != nil {
			t.Error(err)
		}
	}()
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }() // Isolated bus cleanup.
	residentPID := func() uint32 {
		t.Helper()
		var pid uint32
		if err := conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.GetConnectionUnixProcessID", 0, applicationID).Store(&pid); err != nil {
			t.Fatal(err)
		}
		return pid
	}
	pid := residentPID()
	if err := launch("detached"); err != nil {
		t.Fatal(err)
	}
	if residentPID() != pid {
		t.Fatal("detached activation replaced the resident instance")
	}
	var data string
	if err := conn.Object(applicationID+".Fixture", "/Fixture").CallWithContext(ctx, applicationID+".Fixture.View", 0).Store(&data); err != nil {
		t.Fatal(err)
	}
}
