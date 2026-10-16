//go:build linux && cgo && !nogui

package gui

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
	"yati/internal/tracker"
)

const (
	itemInterface                 = "org.kde.StatusNotifierItem"
	watcherName                   = "org.kde.StatusNotifierWatcher"
	menuInterface                 = "com.canonical.dbusmenu"
	itemPath      dbus.ObjectPath = "/StatusNotifierItem"
	menuPath      dbus.ObjectPath = "/Menu"
)

type tray struct {
	ctx            context.Context
	cancel         context.CancelFunc
	conn           *dbus.Conn
	props          *prop.Properties
	action         func(int)
	mu             sync.RWMutex
	state          tracker.Snapshot
	busy           bool
	revision       uint32
	lastError      error
	wg             sync.WaitGroup
	idle, tracking []iconPixmap
}

type trayItem struct{ owner *tray }
type trayMenu struct{ owner *tray }
type trayProperties struct {
	owner *tray
	base  *prop.Properties
}

func (p *trayProperties) Get(iface, name string) (dbus.Variant, *dbus.Error) {
	if iface == itemInterface && name == "IconPixmap" {
		return dbus.MakeVariant(p.owner.currentPixmaps()), nil
	}
	return p.base.Get(iface, name)
}
func (p *trayProperties) GetAll(iface string) (map[string]dbus.Variant, *dbus.Error) {
	values, err := p.base.GetAll(iface)
	if err == nil && iface == itemInterface {
		values["IconPixmap"] = dbus.MakeVariant(p.owner.currentPixmaps())
	}
	return values, err
}
func (p *trayProperties) Set(iface, name string, value dbus.Variant) *dbus.Error {
	return p.base.Set(iface, name, value)
}

func (t *tray) currentPixmaps() []iconPixmap {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.state.Current != nil {
		return t.tracking
	}
	return t.idle
}

type tooltip struct {
	Icon    string
	Pixmaps []iconPixmap
	Title   string
	Text    string
}
type iconPixmap struct {
	Width  int32
	Height int32
	Data   []byte
}
type menuLayout struct {
	ID         int32
	Properties map[string]dbus.Variant
	Children   []dbus.Variant
}
type menuProperties struct {
	ID         int32
	Properties map[string]dbus.Variant
}
type menuEvent struct {
	ID        int32
	Event     string
	Data      dbus.Variant
	Timestamp uint32
}

func newTray(parent context.Context, action func(int)) (*tray, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	t := &tray{ctx: ctx, cancel: cancel, conn: conn, action: action, revision: 1}
	fail := func(err error) (*tray, error) { cancel(); _ = conn.Close(); return nil, err } // Preserve the registration failure.
	if t.idle, err = trayPixmaps(false); err != nil {
		return fail(err)
	}
	if t.tracking, err = trayPixmaps(true); err != nil {
		return fail(err)
	}
	name := fmt.Sprintf("org.kde.StatusNotifierItem-%d-1", os.Getpid())
	if result, err := conn.RequestName(name, dbus.NameFlagDoNotQueue); err != nil {
		return fail(err)
	} else if result != dbus.RequestNameReplyPrimaryOwner {
		return fail(fmt.Errorf("could not claim the tray's D-Bus name"))
	}
	item, menu := &trayItem{t}, &trayMenu{t}
	if err := conn.Export(item, itemPath, itemInterface); err != nil {
		return fail(err)
	}
	if err := conn.Export(menu, menuPath, menuInterface); err != nil {
		return fail(err)
	}
	values := map[string]*prop.Prop{}
	for name, value := range map[string]any{
		"Category": "ApplicationStatus", "Id": "yati", "Title": "Yati", "Status": "Active",
		"WindowId": uint32(0), "IconName": "", "IconPixmap": t.idle,
		"OverlayIconName": "", "OverlayIconPixmap": []iconPixmap{},
		"AttentionIconName": "dialog-warning-symbolic", "AttentionIconPixmap": []iconPixmap{},
		"AttentionMovieName": "", "IconThemePath": "", "ItemIsMenu": false, "Menu": menuPath,
		"ToolTip": tooltip{"", t.idle, "Yati", "No timer running"},
	} {
		values[name] = &prop.Prop{Value: value, Emit: prop.EmitFalse}
	}
	properties, err := prop.Export(conn, itemPath, map[string]map[string]*prop.Prop{itemInterface: values})
	if err != nil {
		return fail(err)
	}
	t.props = properties
	// Reply with immutable artwork snapshots. prop.SetMust reuses slice storage,
	// which would mutate cached art and race with replies still being marshaled.
	if err := conn.Export(&trayProperties{owner: t, base: properties}, itemPath, "org.freedesktop.DBus.Properties"); err != nil {
		return fail(err)
	}
	menuProps := map[string]*prop.Prop{}
	for name, value := range map[string]any{"Version": uint32(3), "TextDirection": "ltr", "Status": "normal", "IconThemePath": []string{}} {
		menuProps[name] = &prop.Prop{Value: value, Emit: prop.EmitTrue}
	}
	if _, err := prop.Export(conn, menuPath, map[string]map[string]*prop.Prop{menuInterface: menuProps}); err != nil {
		return fail(err)
	}
	if err := conn.Export(introspect.NewIntrospectable(&introspect.Node{Interfaces: []introspect.Interface{
		{Name: itemInterface, Methods: introspect.Methods(item), Properties: properties.Introspection(itemInterface), Signals: []introspect.Signal{{Name: "NewIcon"}, {Name: "NewToolTip"}}},
		prop.IntrospectData,
	}}), itemPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		return fail(err)
	}
	if err := conn.Export(introspect.NewIntrospectable(&introspect.Node{Interfaces: []introspect.Interface{
		{Name: menuInterface, Methods: introspect.Methods(menu), Signals: []introspect.Signal{{Name: "LayoutUpdated", Args: []introspect.Arg{{Name: "revision", Type: "u"}, {Name: "parent", Type: "i"}}}}},
		prop.IntrospectData,
	}}), menuPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		return fail(err)
	}
	signals := make(chan *dbus.Signal, 8)
	conn.Signal(signals)
	if err := conn.AddMatchSignal(dbus.WithMatchInterface("org.freedesktop.DBus"), dbus.WithMatchMember("NameOwnerChanged"), dbus.WithMatchArg(0, watcherName)); err != nil {
		return fail(err)
	}
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		register := func() {
			ctx, cancel := context.WithTimeout(t.ctx, 3*time.Second)
			defer cancel()
			// A missing host is normal on stock GNOME. Keep listening so enabling
			// the extension or restarting the shell restores the icon automatically.
			if err := conn.Object(watcherName, "/StatusNotifierWatcher").CallWithContext(ctx, watcherName+".RegisterStatusNotifierItem", 0, name).Err; err != nil {
				t.recordError(err)
			}
		}
		register()
		for {
			select {
			case <-ctx.Done():
				return
			case signal, ok := <-signals:
				if !ok {
					return
				}
				if len(signal.Body) == 3 && signal.Body[2] != "" {
					register()
				}
			}
		}
	}()
	return t, nil
}

func (t *tray) Close() {
	t.cancel()
	t.wg.Wait()
	if err := t.conn.Close(); err != nil {
		t.recordError(err)
	}
}

func (t *tray) Update(state tracker.Snapshot, busy bool) {
	t.mu.Lock()
	t.state, t.busy = state, busy
	t.revision++
	revision := t.revision
	t.mu.Unlock()
	pixmaps, title, text := t.idle, "Yati · stopped", "No timer running"
	if state.Current != nil {
		pixmaps, title, text = t.tracking, "Yati · tracking", state.Current.Description
	}
	t.props.SetMust(itemInterface, "Title", title)
	t.props.SetMust(itemInterface, "ToolTip", tooltip{"", pixmaps, title, text})
	// A closed session bus simply removes the optional tray; the GUI remains usable.
	t.emit(itemPath, itemInterface+".NewIcon")
	t.emit(itemPath, itemInterface+".NewToolTip")
	t.emit(menuPath, menuInterface+".LayoutUpdated", revision, int32(0))
}

func (t *tray) recordError(err error) { t.mu.Lock(); t.lastError = err; t.mu.Unlock() }
func (t *tray) emit(path dbus.ObjectPath, signal string, values ...any) {
	if err := t.conn.Emit(path, signal, values...); err != nil {
		t.recordError(err)
	}
}

func (i *trayItem) Activate(_, _ int32) *dbus.Error { i.owner.action(eventActivate); return nil }
func (i *trayItem) SecondaryActivate(_, _ int32) *dbus.Error {
	i.owner.action(eventActivate)
	return nil
}
func (i *trayItem) ContextMenu(_, _ int32) *dbus.Error   { return nil }
func (i *trayItem) Scroll(_ int32, _ string) *dbus.Error { return nil }

// Numeric event constants match bridge.h; no C types cross the D-Bus boundary.
const (
	eventActivate = 2
	eventStop     = 6
	eventContinue = 7
	eventQuit     = 9
)

func (m *trayMenu) layout() (uint32, menuLayout) {
	t := m.owner
	t.mu.RLock()
	defer t.mu.RUnlock()
	status := "No timer running"
	if t.state.Current != nil {
		status = entryTitle(t.state.Current.Description) + " · " + tracker.FormatDuration(tracker.EntryDuration(*t.state.Current, time.Now()))
	}
	child := func(id int32, title string, enabled bool) dbus.Variant {
		return dbus.MakeVariant(menuLayout{id, map[string]dbus.Variant{"label": dbus.MakeVariant(title), "enabled": dbus.MakeVariant(enabled), "visible": dbus.MakeVariant(true)}, []dbus.Variant{}})
	}
	root := menuLayout{0, map[string]dbus.Variant{"children-display": dbus.MakeVariant("submenu")}, []dbus.Variant{
		child(1, status, false), child(2, "Open Yati", true),
		child(3, "Stop timer", t.state.Current != nil && !t.busy),
		child(4, "Continue last", tracker.LastStopped(t.state) != nil && !t.busy),
		dbus.MakeVariant(menuLayout{5, map[string]dbus.Variant{"type": dbus.MakeVariant("separator")}, []dbus.Variant{}}),
		child(6, "Quit Yati", true),
	}}
	return t.revision, root
}

func (m *trayMenu) GetLayout(parentID, depth int32, _ []string) (uint32, menuLayout, *dbus.Error) {
	revision, layout := m.layout()
	if parentID != 0 {
		for _, child := range layout.Children {
			item := child.Value().(menuLayout)
			if item.ID == parentID {
				return revision, item, nil
			}
		}
		return 0, menuLayout{}, dbus.MakeFailedError(fmt.Errorf("unknown menu item %d", parentID))
	}
	if depth == 0 {
		layout.Children = []dbus.Variant{}
	}
	return revision, layout, nil
}
func (m *trayMenu) GetGroupProperties(ids []int32, _ []string) ([]menuProperties, *dbus.Error) {
	_, root := m.layout()
	items := []menuProperties{{root.ID, root.Properties}}
	for _, child := range root.Children {
		item := child.Value().(menuLayout)
		items = append(items, menuProperties{item.ID, item.Properties})
	}
	if len(ids) == 0 {
		return items, nil
	}
	filtered := []menuProperties{}
	for _, item := range items {
		for _, id := range ids {
			if item.ID == id {
				filtered = append(filtered, item)
				break
			}
		}
	}
	return filtered, nil
}
func (m *trayMenu) GetProperty(id int32, name string) (dbus.Variant, *dbus.Error) {
	_, layout, err := m.GetLayout(id, 0, nil)
	if err != nil {
		return dbus.Variant{}, err
	}
	if value, ok := layout.Properties[name]; ok {
		return value, nil
	}
	return dbus.MakeVariant(""), nil
}
func (m *trayMenu) Event(id int32, event string, _ dbus.Variant, _ uint32) *dbus.Error {
	if event != "clicked" {
		return nil
	}
	t := m.owner
	t.mu.RLock()
	busy, running, continuable := t.busy, t.state.Current != nil, tracker.LastStopped(t.state) != nil
	t.mu.RUnlock()
	switch id {
	case 2:
		t.action(eventActivate)
	case 3:
		if !busy && running {
			t.action(eventStop)
		}
	case 4:
		if !busy && continuable {
			t.action(eventContinue)
		}
	case 6:
		t.action(eventQuit)
	}
	return nil
}
func (m *trayMenu) EventGroup(events []menuEvent) ([]int32, *dbus.Error) {
	failed := []int32{}
	for _, event := range events {
		if err := m.Event(event.ID, event.Event, event.Data, event.Timestamp); err != nil {
			failed = append(failed, event.ID)
		}
	}
	return failed, nil
}
func (m *trayMenu) AboutToShow(_ int32) (bool, *dbus.Error) { return true, nil }
func (m *trayMenu) AboutToShowGroup(ids []int32) ([]int32, []int32, *dbus.Error) {
	return ids, []int32{}, nil
}
