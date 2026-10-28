//go:build linux && cgo && !nogui

package gui

/*
#cgo pkg-config: gtk4 libadwaita-1
#include <stdlib.h>
#include "bridge.h"
*/
import "C"

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/sahilm/fuzzy"
	"yati/internal/auth"
	"yati/internal/config"
	"yati/internal/toggl"
	"yati/internal/tracker"
)

// All desktopApp fields, except the queue, are confined to the GTK thread.
type desktopApp struct {
	ctx                                      context.Context
	cancel                                   context.CancelFunc
	wg                                       sync.WaitGroup
	queue                                    chan func()
	cfg                                      *config.Config
	options                                  Options
	stderr                                   io.Writer
	service                                  *tracker.Service
	tray                                     *tray
	shortcut                                 *globalShortcut
	ready, busy                              bool
	query, page, period, errorText, activity string
	projects                                 []*int64
	editProjects                             []*int64
	editEntry                                int64
	projectNames                             map[int64]string
	recent                                   []tracker.Suggestion
	history                                  []toggl.TimeEntry
	iconUpdates                              chan bool
	iconState                                *bool
}

var activeApp *desktopApp

var newBackend = func(token, cachePath string) tracker.Backend {
	return tracker.Remote{Client: toggl.NewClient(token).WithCachePath(cachePath)}
}

func runDesktop(cfg *config.Config, options Options, stderr io.Writer) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ctx, cancel := context.WithCancel(context.Background())
	a := &desktopApp{ctx: ctx, cancel: cancel, cfg: cfg, options: options, stderr: stderr,
		queue: make(chan func(), 8), iconUpdates: make(chan bool, 1), page: "track", period: "d"}
	activeApp = a
	defer func() { activeApp = nil }()
	arguments := []string{"yati"}
	if options.Background {
		arguments = append(arguments, "--background")
	}
	if options.Toggle {
		arguments = append(arguments, "--toggle")
	}
	if options.Quit {
		arguments = append(arguments, "--quit")
	}
	argv := make([]*C.char, len(arguments)+1)
	for i, argument := range arguments {
		argv[i] = C.CString(argument)
		defer C.free(unsafe.Pointer(argv[i]))
	}
	code := C.yati_run(C.int(len(arguments)), (**C.char)(unsafe.Pointer(&argv[0])))
	cancel()
	a.wg.Wait()
	if a.tray != nil {
		a.tray.Close()
	}
	if a.shortcut != nil {
		a.shortcut.Close()
	}
	if code != 0 {
		return fmt.Errorf("desktop application exited with status %d", code)
	}
	return nil
}

func ctext(value string, use func(*C.char)) {
	// GTK expects valid UTF-8 without embedded NUL, even for remote descriptions.
	value = strings.ToValidUTF8(strings.ReplaceAll(value, "\x00", "�"), "�")
	text := C.CString(value)
	defer C.free(unsafe.Pointer(text))
	use(text)
}

func truth(value bool) C.int {
	if value {
		return 1
	}
	return 0
}

//export yati_go_event
func yati_go_event(action C.int, index C.int, text *C.char) {
	if a := activeApp; a != nil {
		a.event(int(action), int(index), C.GoString(text))
	}
}

func (a *desktopApp) post(fn func()) {
	select {
	case <-a.ctx.Done():
		return
	case a.queue <- fn:
		C.yati_post(C.YATI_RESULT, nil)
	}
}

func (a *desktopApp) event(action, index int, text string) {
	if !a.ready && action != C.YATI_STARTUP {
		return
	}
	switch action {
	case C.YATI_STARTUP:
		a.ready = true
		a.wg.Add(1)
		go a.desktopIcons()
		C.yati_set_autostart(truth(AutostartEnabled()))
		if !a.options.NoTray {
			var err error
			a.tray, err = newTray(a.ctx, func(action int) { C.yati_post(C.int(action), nil) })
			if err != nil {
				_, _ = fmt.Fprintf(a.stderr, "Tray unavailable: %v\n", err)
			}
		}
		a.authenticate("", false)
		if a.options.startupReady != nil {
			a.options.startupReady()
		}
	case C.YATI_RESULT:
		for {
			select {
			case fn := <-a.queue:
				fn()
			default:
				return
			}
		}
	case C.YATI_ACTIVATE:
		ctext(text, func(token *C.char) { C.yati_present(token) })
		if a.service != nil && !a.busy {
			a.refresh(false)
		}
	case C.YATI_HIDE:
		C.yati_hide()
	case C.YATI_SEARCH:
		a.query = text
		a.renderLists()
	case C.YATI_START_NEW:
		if a.busy || a.service == nil {
			return
		}
		description := strings.TrimSpace(C.GoString(C.yati_query()))
		if description == "" {
			a.toast("Describe the task before starting it.")
			return
		}
		var project *int64
		if index := int(C.yati_project()); index >= 0 && index < len(a.projects) {
			project = a.projects[index]
		}
		a.start(0, project, description)
	case C.YATI_RESUME:
		if a.busy || a.service == nil {
			return
		}
		if text == "history" {
			if index >= 0 && index < len(a.history) {
				entry := a.history[index]
				a.start(entry.WorkspaceID, entry.ProjectID, entry.Description)
			}
		} else if index >= 0 && index < len(a.recent) {
			entry := a.recent[index]
			a.start(entry.WorkspaceID, entry.ProjectID, entry.Description)
		}
	case C.YATI_STOP:
		if a.service != nil {
			a.work("Stopping timer…", a.service.Stop, func() { a.toast("Timer stopped") })
		}
	case C.YATI_EDIT:
		if !a.busy && a.service != nil {
			a.editCurrent()
		}
	case C.YATI_UPDATE:
		if a.busy || a.service == nil || index < 0 || index >= len(a.editProjects) {
			return
		}
		service, entry, project := a.service, a.editEntry, a.editProjects[index]
		a.work("Updating task…", func(ctx context.Context) error {
			return service.Update(ctx, entry, project, text)
		}, func() { a.toast("Task updated") })
	case C.YATI_CONTINUE:
		if a.service == nil || a.busy {
			return
		}
		if entry := tracker.LastStopped(a.service.State()); entry != nil {
			a.start(entry.WorkspaceID, entry.ProjectID, entry.Description)
		}
	case C.YATI_REFRESH:
		if a.service == nil {
			a.authenticate("", false)
		} else {
			a.refresh(true)
		}
	case C.YATI_TICK:
		a.renderCurrent()
		if a.page == "history" {
			for i, entry := range a.history {
				if entry.Duration < 0 {
					ctext(tracker.FormatDuration(tracker.EntryDuration(entry, time.Now())), func(text *C.char) {
						C.yati_list_update_detail(1, C.int(i), text)
					})
				}
			}
			a.renderSummary()
		}
	case C.YATI_PERIOD:
		if index >= 0 && index < 3 {
			a.period = []string{"d", "w", "m"}[index]
			a.renderLists()
		}
	case C.YATI_PAGE:
		a.page = text
		if text != "connection" {
			a.renderLists()
		}
	case C.YATI_CONNECT:
		a.authenticate(strings.TrimSpace(C.GoString(C.yati_token())), C.yati_remember() != 0)
	case C.YATI_CONNECTION:
		C.yati_set_connection(1)
	case C.YATI_AUTOSTART:
		if err := SetAutostart(index != 0); err != nil {
			a.errorText = "Could not change login startup: " + err.Error()
			C.yati_set_autostart(truth(AutostartEnabled()))
			a.render()
		}
	case C.YATI_SHORTCUT:
		a.configureShortcut()
	case C.YATI_HELP:
		C.yati_show_shortcuts()
	case C.YATI_QUIT:
		a.cancel()
		C.yati_quit()
	}
}

func (a *desktopApp) authenticate(override string, remember bool) {
	if a.busy {
		return
	}
	cfg := *a.cfg
	if override != "" {
		cfg.Token = override
	}
	a.busy, a.activity, a.errorText = true, "Connecting to Toggl…", ""
	a.render()
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		token, err := auth.GetTokenContext(a.ctx, &cfg)
		a.post(func() {
			a.busy = false
			if err != nil {
				a.errorText = "Add your Toggl API token to connect, or use your existing yati credentials."
				a.render()
				C.yati_set_connection(1)
				return
			}
			digest := fmt.Sprintf("%x", sha256.Sum256([]byte(token)))[:16]
			cacheDirectory, cacheErr := os.UserCacheDir()
			cachePath, targetPath := "", ""
			if cacheErr == nil {
				cachePath = filepath.Join(cacheDirectory, "yati", "gui-"+digest+".json")
				targetPath = filepath.Join(cacheDirectory, "yati", "targets-"+digest+".json")
			}
			a.service = tracker.NewService(newBackend(token, targetPath), cachePath)
			a.cfg = &cfg
			a.renderProjects()
			a.render()
			a.work("Refreshing from Toggl…", func(ctx context.Context) error { return a.service.Refresh(ctx, false) }, func() {
				if remember && override != "" {
					if saveErr := saveToken(override); saveErr != nil {
						a.errorText = "Connected, but the API token could not be saved: " + saveErr.Error()
					}
				}
				C.yati_set_connection(0)
				C.yati_clear_token()
				a.toast("Connected to Toggl")
				if shortcutWanted() {
					a.configureShortcut()
				}
			})
		})
	}()
}

func (a *desktopApp) work(message string, operation func(context.Context) error, success func()) {
	if a.busy {
		return
	}
	a.busy, a.activity, a.errorText = true, message, ""
	a.render()
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		err := operation(a.ctx)
		a.post(func() {
			a.busy = false
			if err != nil {
				a.errorText = err.Error()
			} else if success != nil {
				success()
			}
			a.renderProjects()
			a.render()
		})
	}()
}

func (a *desktopApp) refresh(force bool) {
	service := a.service
	a.work("Refreshing from Toggl…", func(ctx context.Context) error { return service.Refresh(ctx, force) }, nil)
}

func (a *desktopApp) start(workspace int64, project *int64, description string) {
	service := a.service
	query := a.query
	a.work("Starting timer…", func(ctx context.Context) error { return service.Start(ctx, workspace, project, description) }, func() {
		if a.query == query {
			ctext("", func(text *C.char) { C.yati_set_query(text) })
		}
		a.toast("Timer started")
	})
}

func (a *desktopApp) state() tracker.Snapshot {
	if a.service == nil {
		return tracker.Snapshot{}
	}
	return a.service.State()
}

func (a *desktopApp) render() {
	state := a.state()
	C.yati_set_connected(truth(state.Profile.ID != 0))
	a.renderCurrent()
	a.renderLists()
	message, failed := "Connect to Toggl to load your tasks", false
	if !state.SyncedAt.IsZero() {
		message = "Updated " + state.SyncedAt.Local().Format("15:04") + " · search works offline"
	}
	if a.busy {
		message = a.activity
	}
	if state.CacheWarning != "" {
		message, failed = state.CacheWarning, true
	}
	if a.errorText != "" {
		message, failed = a.errorText, true
	}
	ctext(message, func(text *C.char) { C.yati_set_status(text, truth(failed), truth(a.busy)) })
	if a.tray != nil {
		a.tray.Update(state, a.busy)
	}
}

func (a *desktopApp) renderCurrent() {
	state := a.state()
	a.updateIconState(state.Current != nil)
	title, subtitle, elapsed := "No timer running", "Start a task below or continue your last entry.", ""
	if entry := state.Current; entry != nil {
		title, subtitle = entryTitle(entry.Description), a.projectName(entry.ProjectID)
		elapsed = tracker.FormatDuration(tracker.EntryDuration(*entry, time.Now()))
	}
	ctext(title, func(t *C.char) {
		ctext(subtitle, func(s *C.char) {
			ctext(elapsed, func(e *C.char) {
				C.yati_set_current(t, s, e, truth(state.Current != nil), truth(tracker.LastStopped(state) != nil), truth(a.busy))
			})
		})
	})
}

func (a *desktopApp) projectName(id *int64) string {
	if id == nil {
		return "No project"
	}
	if name, ok := a.projectNames[*id]; ok {
		return name
	}
	return "Unavailable project"
}

func (a *desktopApp) renderProjects() {
	var previous *int64
	if index := int(C.yati_project()); index >= 0 && index < len(a.projects) {
		previous = a.projects[index]
	}
	a.projects = []*int64{nil}
	C.yati_projects_begin()
	ctext("No project", func(text *C.char) { C.yati_project_add(text) })
	selected := 0
	state := a.state()
	a.projectNames = make(map[int64]string, len(state.Projects))
	for _, project := range state.Projects {
		a.projectNames[project.ID] = project.Name
		if !project.Active || (project.WorkspaceID != 0 && project.WorkspaceID != state.Profile.DefaultWorkspace) {
			continue
		}
		id := project.ID
		a.projects = append(a.projects, &id)
		ctext(project.Name, func(text *C.char) { C.yati_project_add(text) })
		if previous != nil && *previous == id {
			selected = len(a.projects) - 1
		}
	}
	C.yati_projects_end(C.int(selected))
}

func (a *desktopApp) editCurrent() {
	state := a.state()
	entry := state.Current
	if entry == nil {
		return
	}
	a.editEntry, a.editProjects = entry.ID, []*int64{nil}
	C.yati_editor_begin()
	ctext("No project", func(text *C.char) { C.yati_editor_project_add(text) })
	selected := 0
	for _, project := range state.Projects {
		if !project.Active || (project.WorkspaceID != 0 && project.WorkspaceID != entry.WorkspaceID) {
			continue
		}
		id := project.ID
		a.editProjects = append(a.editProjects, &id)
		ctext(project.Name, func(text *C.char) { C.yati_editor_project_add(text) })
		if entry.ProjectID != nil && *entry.ProjectID == id {
			selected = len(a.editProjects) - 1
		}
	}
	if entry.ProjectID != nil && selected == 0 {
		a.editProjects = append(a.editProjects, entry.ProjectID)
		ctext(a.projectName(entry.ProjectID), func(text *C.char) { C.yati_editor_project_add(text) })
		selected = len(a.editProjects) - 1
	}
	ctext(entry.Description, func(text *C.char) { C.yati_editor_show(C.int(selected), text) })
}

func (a *desktopApp) renderLists() {
	if !a.ready {
		return
	}
	state := a.state()
	projectNames := make(map[int64]string)
	for _, p := range state.Projects {
		projectNames[p.ID] = p.Name
	}
	entries := make([]toggl.TimeEntry, 0, len(state.Entries))
	for _, entry := range state.Entries {
		if entry.WorkspaceID == state.Profile.DefaultWorkspace {
			entries = append(entries, entry)
		}
	}
	a.recent = tracker.FilterSuggestions(tracker.Suggestions(entries, projectNames), a.query)
	if len(a.recent) > 100 {
		a.recent = a.recent[:100]
	}
	C.yati_list_begin(0)
	for i, item := range a.recent {
		project := "No project"
		if item.ProjectID != nil {
			project = a.projectName(item.ProjectID)
		}
		a.addRow(false, i, entryTitle(item.Description), project, item.LastUsed.Local().Format("02 Jan"))
	}
	empty := "No recent tasks yet. Type a description above and choose Start new."
	if strings.TrimSpace(a.query) != "" {
		empty = "No matching tasks. Choose a project and Start new, or press Ctrl+Enter."
	}
	ctext(empty, func(text *C.char) { C.yati_list_end(0, text) })

	start, end := tracker.PeriodBounds(a.period, time.Now())
	a.history = nil
	texts := []string{}
	for _, entry := range entries {
		if entry.Start.Before(start) || !entry.Start.Before(end) {
			continue
		}
		a.history = append(a.history, entry)
		texts = append(texts, entry.Description+" "+a.projectName(entry.ProjectID))
	}
	if query := strings.TrimSpace(a.query); query != "" {
		filtered := make([]toggl.TimeEntry, 0)
		for _, match := range fuzzy.Find(query, texts) {
			filtered = append(filtered, a.history[match.Index])
		}
		a.history = filtered
	}
	C.yati_list_begin(1)
	for i, entry := range a.history {
		subtitle := entry.Start.Local().Format("Mon 02 Jan · 15:04") + " · " + a.projectName(entry.ProjectID)
		a.addRow(true, i, entryTitle(entry.Description), subtitle, tracker.FormatDuration(tracker.EntryDuration(entry, time.Now())))
	}
	ctext("No entries for this period and search.", func(text *C.char) { C.yati_list_end(1, text) })
	a.renderSummary()
}

func (a *desktopApp) renderSummary() {
	var total time.Duration
	for _, entry := range a.history {
		total += tracker.EntryDuration(entry, time.Now())
	}
	ctext(fmt.Sprintf("%s total · %d entries", tracker.FormatDuration(total), len(a.history)), func(text *C.char) { C.yati_set_summary(text) })
}

func (a *desktopApp) addRow(history bool, index int, title, subtitle, detail string) {
	ctext(title, func(t *C.char) {
		ctext(subtitle, func(s *C.char) {
			ctext(detail, func(d *C.char) {
				C.yati_list_add(truth(history), C.int(index), t, s, d)
			})
		})
	})
}

func (a *desktopApp) toast(message string) { ctext(message, func(text *C.char) { C.yati_toast(text) }) }
func entryTitle(description string) string {
	if strings.TrimSpace(description) == "" {
		return "(no description)"
	}
	return description
}

func nativeFrame() (int, int, bool) {
	return int(C.yati_width()), int(C.yati_height()), C.yati_transitioning() != 0
}

func nativeHistoryDetail(index int) string { return C.GoString(C.yati_list_detail(1, C.int(index))) }

func nativeShortcutsVisible() bool { return C.yati_help_visible() != 0 }

func nativeEditorVisible() bool { return C.yati_editor_visible() != 0 }

func (a *desktopApp) updateIconState(running bool) {
	if a.iconState != nil && *a.iconState == running {
		return
	}
	a.iconState = &running
	C.yati_set_icon(truth(running))
	select {
	case a.iconUpdates <- running:
	default:
		select {
		case <-a.iconUpdates:
		default:
		}
		select {
		case a.iconUpdates <- running:
		case <-a.ctx.Done():
		}
	}
}

func (a *desktopApp) desktopIcons() {
	defer a.wg.Done()
	for {
		select {
		case <-a.ctx.Done():
			return
		case running := <-a.iconUpdates:
			if err := syncDesktopIcon(running); err != nil {
				_, _ = fmt.Fprintf(a.stderr, "Could not update desktop icon: %v\n", err)
			}
		}
	}
}
