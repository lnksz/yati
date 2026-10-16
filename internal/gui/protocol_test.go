//go:build linux && cgo && !nogui

package gui

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"yati/internal/toggl"
	"yati/internal/tracker"
)

type fixtureWatcher struct{ registered chan string }

func (w *fixtureWatcher) RegisterStatusNotifierItem(name string) *dbus.Error {
	w.registered <- name
	return nil
}

func trayIconColored(ctx context.Context, conn *dbus.Conn, name string) (bool, error) {
	var value dbus.Variant
	if err := conn.Object(name, itemPath).CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0, itemInterface, "IconPixmap").Store(&value); err != nil {
		return false, err
	}
	var pixmaps []iconPixmap
	if err := dbus.Store([]any{value.Value()}, &pixmaps); err != nil {
		return false, err
	}
	if len(pixmaps) == 0 {
		return false, fmt.Errorf("tray exported no icon pixels")
	}
	for _, pixmap := range pixmaps {
		for i := 0; i+3 < len(pixmap.Data); i += 4 {
			if pixmap.Data[i] > 200 && (pixmap.Data[i+1] != pixmap.Data[i+2] || pixmap.Data[i+2] != pixmap.Data[i+3]) {
				return true, nil
			}
		}
	}
	return false, nil
}

func TestTrayProtocol(t *testing.T) {
	if !integrationEnabled() {
		t.Skip("requires the isolated session bus from scripts/test-gui.sh")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	watcher, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = watcher.Close() }() // Private test bus cleanup.
	if _, err := watcher.RequestName(watcherName, dbus.NameFlagDoNotQueue); err != nil {
		t.Fatal(err)
	}
	fixture := &fixtureWatcher{make(chan string, 4)}
	if err := watcher.Export(fixture, "/StatusNotifierWatcher", watcherName); err != nil {
		t.Fatal(err)
	}
	actions := make(chan int, 8)
	icon, err := newTray(ctx, func(action int) { actions <- action })
	if err != nil {
		t.Fatal(err)
	}
	defer icon.Close()
	var name string
	select {
	case name = <-fixture.registered:
	case <-ctx.Done():
		t.Fatal("tray did not register")
	}
	client, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }() // Private test bus cleanup.
	state := tracker.Snapshot{Profile: toggl.Me{ID: 1, DefaultWorkspace: 42}, Current: &toggl.TimeEntry{ID: 12, WorkspaceID: 42, Description: "A & B <C>", Start: time.Now().Add(-time.Minute), Duration: -1}, Entries: []toggl.TimeEntry{{ID: 11, WorkspaceID: 42, Description: "Previous", Duration: 10}}}
	icon.Update(state, false)
	if colored, err := trayIconColored(ctx, client, name); err != nil || !colored {
		t.Fatalf("running tray icon: colored=%v err=%v", colored, err)
	}
	icon.Update(tracker.Snapshot{}, false)
	if colored, err := trayIconColored(ctx, client, name); err != nil || colored {
		t.Fatalf("stopped tray icon: colored=%v err=%v", colored, err)
	}
	icon.Update(state, false)
	var revision uint32
	var layout menuLayout
	if err := client.Object(name, menuPath).CallWithContext(ctx, menuInterface+".GetLayout", 0, int32(0), int32(-1), []string{}).Store(&revision, &layout); err != nil {
		t.Fatal(err)
	}
	if len(layout.Children) != 6 || layout.Children[2].Value().([]any)[1] == nil {
		t.Fatalf("invalid exported menu: %#v", layout)
	}
	if err := client.Object(name, itemPath).CallWithContext(ctx, itemInterface+".Activate", 0, int32(0), int32(0)).Err; err != nil {
		t.Fatal(err)
	}
	select {
	case action := <-actions:
		if action != eventActivate {
			t.Fatal(action)
		}
	case <-ctx.Done():
		t.Fatal("activation not delivered")
	}
	if err := client.Object(name, menuPath).CallWithContext(ctx, menuInterface+".Event", 0, int32(3), "clicked", dbus.MakeVariant(int32(0)), uint32(0)).Err; err != nil {
		t.Fatal(err)
	}
	select {
	case action := <-actions:
		if action != eventStop {
			t.Fatal(action)
		}
	case <-ctx.Done():
		t.Fatal("stop not delivered")
	}
	// Restarting a shell-side tray host must register the existing item again.
	if _, err := watcher.ReleaseName(watcherName); err != nil {
		t.Fatal(err)
	}
	if _, err := watcher.RequestName(watcherName, dbus.NameFlagDoNotQueue); err != nil {
		t.Fatal(err)
	}
	select {
	case registered := <-fixture.registered:
		if registered != name {
			t.Fatal(registered)
		}
	case <-ctx.Done():
		t.Fatal("tray did not re-register")
	}
}

type shortcutInfo struct {
	ID         string
	Properties map[string]dbus.Variant
}
type fixturePortal struct {
	conn          *dbus.Conn
	mu            sync.Mutex
	deny          bool
	configured    bool
	registered    map[dbus.Sender]string
	legacy        bool
	registerError bool
	created       int
}

const fixtureSession dbus.ObjectPath = "/org/freedesktop/portal/desktop/session/yati_test"

func (p *fixturePortal) Register(appID string, _ map[string]dbus.Variant, sender dbus.Sender) *dbus.Error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.registerError {
		return dbus.NewError("org.freedesktop.portal.Error.InvalidArgument", []any{"invalid application ID"})
	}
	if appID != applicationID || p.registered[sender] != "" {
		return dbus.MakeFailedError(fmt.Errorf("invalid or repeated registration"))
	}
	p.registered[sender] = appID
	return nil
}

func (p *fixturePortal) response(options map[string]dbus.Variant, results map[string]dbus.Variant, sender dbus.Sender) (dbus.ObjectPath, *dbus.Error) {
	token, _ := options["handle_token"].Value().(string)
	path := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/test/" + token)
	code := uint32(0)
	p.mu.Lock()
	if p.deny {
		code = 1
	}
	p.mu.Unlock()
	if err := p.conn.Emit(path, "org.freedesktop.portal.Request.Response", code, results); err != nil {
		return "", dbus.MakeFailedError(err)
	}
	return path, nil
}
func (p *fixturePortal) CreateSession(options map[string]dbus.Variant, sender dbus.Sender) (dbus.ObjectPath, *dbus.Error) {
	p.mu.Lock()
	if !p.legacy && p.registered[sender] != applicationID {
		p.mu.Unlock()
		return "", dbus.NewError("org.freedesktop.portal.Error.InvalidArgument", []any{"An app id is required"})
	}
	p.created++
	p.mu.Unlock()
	return p.response(options, map[string]dbus.Variant{"session_handle": dbus.MakeVariant(string(fixtureSession))}, sender)
}
func (p *fixturePortal) BindShortcuts(session dbus.ObjectPath, shortcuts []shortcutInfo, parent string, options map[string]dbus.Variant, sender dbus.Sender) (dbus.ObjectPath, *dbus.Error) {
	if session != fixtureSession || len(shortcuts) != 1 || shortcuts[0].ID != "quick-entry" {
		return "", dbus.MakeFailedError(fmt.Errorf("invalid shortcut request"))
	}
	return p.response(options, map[string]dbus.Variant{"shortcuts": dbus.MakeVariant(shortcuts)}, sender)
}
func (p *fixturePortal) ConfigureShortcuts(session dbus.ObjectPath, parent string, options map[string]dbus.Variant) *dbus.Error {
	p.mu.Lock()
	p.configured = true
	p.mu.Unlock()
	return nil
}

func TestShortcutPortal(t *testing.T) {
	if !integrationEnabled() {
		t.Skip("requires isolated session bus")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = server.Close() }() // Private test bus cleanup.
	if _, err := server.RequestName(portalName, dbus.NameFlagDoNotQueue); err != nil {
		t.Fatal(err)
	}
	fixture := &fixturePortal{conn: server, registered: make(map[dbus.Sender]string)}
	if err := server.Export(fixture, portalPath, shortcutInterface); err != nil {
		t.Fatal(err)
	}
	if err := server.Export(fixture, portalPath, "org.freedesktop.host.portal.Registry"); err != nil {
		t.Fatal(err)
	}
	activated := make(chan string, 1)
	shortcut, err := bindShortcut(ctx, func(token string) { activated <- token })
	if err != nil {
		t.Fatal(err)
	}
	defer shortcut.Close()
	if err := server.Emit(portalPath, shortcutInterface+".Activated", fixtureSession, "quick-entry", uint64(1), map[string]dbus.Variant{"activation_token": dbus.MakeVariant("test-focus-token")}); err != nil {
		t.Fatal(err)
	}
	select {
	case token := <-activated:
		if token != "test-focus-token" {
			t.Fatal(token)
		}
	case <-ctx.Done():
		t.Fatal("activation token not delivered")
	}
	if err := shortcut.Configure(ctx); err != nil {
		t.Fatal(err)
	}
	fixture.mu.Lock()
	configured := fixture.configured
	fixture.deny = true
	fixture.mu.Unlock()
	if !configured {
		t.Fatal("shortcut preferences were not requested")
	}
	if denied, err := bindShortcut(ctx, func(string) {}); err == nil {
		denied.Close()
		t.Fatal("denied permission was accepted")
	}
	fixture.mu.Lock()
	fixture.deny, fixture.registerError = false, true
	created := fixture.created
	fixture.mu.Unlock()
	if invalid, err := bindShortcut(ctx, func(string) {}); err == nil {
		invalid.Close()
		t.Fatal("registration failure was ignored")
	}
	fixture.mu.Lock()
	if fixture.created != created {
		t.Error("created a session after registration failed")
	}
	fixture.legacy = true
	fixture.mu.Unlock()
	if err := server.Export(nil, portalPath, "org.freedesktop.host.portal.Registry"); err != nil {
		t.Fatal(err)
	}
	legacy, err := bindShortcut(ctx, func(string) {})
	if err != nil {
		t.Fatalf("portal without a host registry: %v", err)
	}
	legacy.Close()
}

// TestHostShortcutPortalIdentity explicitly opts into the real desktop portal.
// It creates and closes a session without requesting any shortcut assignment.
func TestHostShortcutPortalIdentity(t *testing.T) {
	if os.Getenv("YATI_GUI_HOST_PORTAL") != "1" {
		t.Skip("set YATI_GUI_HOST_PORTAL=1 on a desktop with Yati's launcher installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }() // Close the test's own portal connection.
	signals := make(chan *dbus.Signal, 4)
	conn.Signal(signals)
	if err := conn.AddMatchSignal(dbus.WithMatchSender(portalName), dbus.WithMatchInterface("org.freedesktop.portal.Request"), dbus.WithMatchMember("Response")); err != nil {
		t.Fatal(err)
	}
	if err := registerPortalApp(ctx, conn); err != nil {
		t.Fatal(err)
	}
	unique := fmt.Sprintf("yatiIdentityTest%d", time.Now().UnixNano())
	var request dbus.ObjectPath
	if err := conn.Object(portalName, portalPath).CallWithContext(ctx, shortcutInterface+".CreateSession", 0, map[string]dbus.Variant{
		"handle_token": dbus.MakeVariant(unique), "session_handle_token": dbus.MakeVariant(unique),
	}).Store(&request); err != nil {
		t.Fatal(err)
	}
	for {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case signal := <-signals:
			if signal.Path != request {
				continue
			}
			var code uint32
			var results map[string]dbus.Variant
			if err := dbus.Store(signal.Body, &code, &results); err != nil {
				t.Fatal(err)
			}
			if code != 0 {
				t.Fatalf("real desktop rejected session creation: %d", code)
			}
			var session dbus.ObjectPath
			if err := dbus.Store([]any{results["session_handle"].Value()}, &session); err != nil || !session.IsValid() {
				t.Fatalf("invalid session handle: %v (%v)", results, err)
			}
			if err := conn.Object(portalName, session).CallWithContext(ctx, "org.freedesktop.portal.Session.Close", 0).Err; err != nil {
				t.Fatal(err)
			}
			return
		}
	}
}
