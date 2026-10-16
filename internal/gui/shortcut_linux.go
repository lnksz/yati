//go:build linux && cgo && !nogui

package gui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const portalName = "org.freedesktop.portal.Desktop"
const portalPath dbus.ObjectPath = "/org/freedesktop/portal/desktop"
const shortcutInterface = "org.freedesktop.portal.GlobalShortcuts"
const registryInterface = "org.freedesktop.host.portal.Registry"

type globalShortcut struct {
	conn    *dbus.Conn
	session dbus.ObjectPath
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

func (a *desktopApp) configureShortcut() {
	if a.busy {
		return
	}
	if err := InstallDesktop(false); err != nil {
		a.errorText = "Could not register the app for desktop shortcuts: " + err.Error()
		a.render()
		return
	}
	if a.shortcut != nil {
		a.work("Opening shortcut settings…", a.shortcut.Configure, nil)
		return
	}
	var shortcut *globalShortcut
	a.work("Choose a shortcut in the desktop dialog…", func(ctx context.Context) error {
		var err error
		shortcut, err = bindShortcut(ctx, func(token string) { a.post(func() { a.event(eventActivate, 0, token) }) })
		return err
	}, func() {
		a.shortcut = shortcut
		if err := rememberShortcut(); err != nil {
			a.errorText = "The shortcut works, but its startup preference could not be saved: " + err.Error()
		}
		a.toast("Global shortcut ready")
	})
}

func bindShortcut(parent context.Context, activate func(string)) (*globalShortcut, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	s := &globalShortcut{conn: conn, cancel: cancel}
	fail := func(err error) (*globalShortcut, error) {
		cancel()
		_ = conn.Close() // Preserve the portal error during cleanup.
		return nil, fmt.Errorf("could not configure a global shortcut: %w. You can bind 'yati gui' in your desktop's keyboard settings", err)
	}
	if err := registerPortalApp(ctx, conn); err != nil {
		return fail(err)
	}
	signals := make(chan *dbus.Signal, 16)
	conn.Signal(signals)
	if err := conn.AddMatchSignal(dbus.WithMatchSender(portalName), dbus.WithMatchInterface("org.freedesktop.portal.Request"), dbus.WithMatchMember("Response")); err != nil {
		return fail(err)
	}
	if err := conn.AddMatchSignal(dbus.WithMatchSender(portalName), dbus.WithMatchInterface(shortcutInterface), dbus.WithMatchMember("Activated")); err != nil {
		return fail(err)
	}
	request := func(method string, arguments ...any) (map[string]dbus.Variant, error) {
		var path dbus.ObjectPath
		if err := conn.Object(portalName, portalPath).CallWithContext(ctx, shortcutInterface+"."+method, 0, arguments...).Store(&path); err != nil {
			return nil, err
		}
		for {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case signal, ok := <-signals:
				if !ok {
					return nil, fmt.Errorf("desktop portal disconnected")
				}
				if signal.Path != path || signal.Name != "org.freedesktop.portal.Request.Response" {
					continue
				}
				var code uint32
				var results map[string]dbus.Variant
				if err := dbus.Store(signal.Body, &code, &results); err != nil {
					return nil, err
				}
				if code != 0 {
					return nil, fmt.Errorf("desktop shortcut permission was cancelled or denied")
				}
				return results, nil
			}
		}
	}
	unique := fmt.Sprintf("yati%d", time.Now().UnixNano())
	results, err := request("CreateSession", map[string]dbus.Variant{
		"handle_token": dbus.MakeVariant(unique), "session_handle_token": dbus.MakeVariant(unique),
	})
	if err != nil {
		return fail(err)
	}
	if result, ok := results["session_handle"]; ok {
		switch value := result.Value().(type) {
		case string:
			s.session = dbus.ObjectPath(value)
		case dbus.ObjectPath:
			s.session = value
		}
	}
	if !s.session.IsValid() {
		return fail(fmt.Errorf("desktop portal returned no shortcut session"))
	}
	type shortcutDescription struct {
		ID      string
		Options map[string]dbus.Variant
	}
	results, err = request("BindShortcuts", s.session, []shortcutDescription{{"quick-entry", map[string]dbus.Variant{"description": dbus.MakeVariant("Open Yati quick entry")}}}, "", map[string]dbus.Variant{"handle_token": dbus.MakeVariant(unique + "bind")})
	if err != nil {
		return fail(err)
	}
	var bound []shortcutDescription
	if err := dbus.Store([]any{results["shortcuts"].Value()}, &bound); err != nil || len(bound) == 0 {
		return fail(fmt.Errorf("no shortcut was assigned"))
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case signal, ok := <-signals:
				if !ok {
					return
				}
				if signal.Name != shortcutInterface+".Activated" || len(signal.Body) != 4 {
					continue
				}
				var session dbus.ObjectPath
				var id string
				var timestamp uint64
				var options map[string]dbus.Variant
				if dbus.Store(signal.Body, &session, &id, &timestamp, &options) != nil || session != s.session || id != "quick-entry" {
					continue
				}
				token, _ := options["activation_token"].Value().(string)
				activate(token)
			}
		}
	}()
	return s, nil
}

func registerPortalApp(ctx context.Context, conn *dbus.Conn) error {
	// Identity belongs to a D-Bus peer, not the process. GTK's application ID
	// does not identify this separate connection. Register before any portal call.
	err := conn.Object(portalName, portalPath).CallWithContext(ctx, registryInterface+".Register", 0, applicationID, map[string]dbus.Variant{}).Err
	if err == nil {
		return nil
	}
	var busError dbus.Error
	if errors.As(err, &busError) && (busError.Name == "org.freedesktop.DBus.Error.UnknownMethod" || busError.Name == "org.freedesktop.DBus.Error.UnknownInterface") {
		// Older portals use desktop-launch/cgroup identity instead of a registry.
		return nil
	}
	return fmt.Errorf("register application ID: %w", err)
}

func (s *globalShortcut) Configure(ctx context.Context) error {
	err := s.conn.Object(portalName, portalPath).CallWithContext(ctx, shortcutInterface+".ConfigureShortcuts", 0, s.session, "", map[string]dbus.Variant{}).Err
	if err != nil {
		return fmt.Errorf("open Yati's shortcut preferences in desktop Settings; this portal cannot reopen them: %w", err)
	}
	return nil
}

func (s *globalShortcut) Close() {
	s.cancel()
	s.wg.Wait()
	_ = s.conn.Close() // Session shutdown is best effort after cancellation.
}

func shortcutPreferencePath() string {
	path, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(path, "yati", "desktop.json")
}
func shortcutWanted() bool {
	data, err := os.ReadFile(shortcutPreferencePath())
	if err != nil {
		return false
	}
	var values map[string]bool
	return json.Unmarshal(data, &values) == nil && values["global_shortcut"]
}
func rememberShortcut() error {
	path := shortcutPreferencePath()
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("no configuration directory available")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte("{\"global_shortcut\":true}\n"), 0600)
}
