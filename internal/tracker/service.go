package tracker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"yati/internal/localfile"
	"yati/internal/toggl"
)

// Backend is the context-aware subset of Toggl needed by the desktop app.
type Backend interface {
	Profile(context.Context) (*toggl.Me, error)
	Entries(context.Context, time.Time, time.Time) ([]toggl.TimeEntry, error)
	Projects(context.Context, int64, []toggl.TimeEntry) ([]toggl.Project, error)
	Current(context.Context) (*toggl.TimeEntry, error)
	Start(context.Context, int64, *int64, string) (*toggl.TimeEntry, error)
	Stop(context.Context, int64, int64) (*toggl.TimeEntry, error)
	Update(context.Context, int64, int64, *int64, string) (*toggl.TimeEntry, error)
}

// Remote adapts the existing Toggl client for the desktop service.
type Remote struct{ Client *toggl.Client }

func (r Remote) Profile(ctx context.Context) (*toggl.Me, error) {
	return r.Client.WithContext(ctx).GetMe()
}
func (r Remote) Entries(ctx context.Context, start, end time.Time) ([]toggl.TimeEntry, error) {
	return r.Client.WithContext(ctx).GetTimeEntries(start, end)
}
func (r Remote) Projects(ctx context.Context, workspace int64, entries []toggl.TimeEntry) ([]toggl.Project, error) {
	projects, _, err := r.Client.WithContext(ctx).GetTargetsWithTimeEntries(workspace, entries)
	return projects, err
}
func (r Remote) Current(ctx context.Context) (*toggl.TimeEntry, error) {
	return r.Client.WithContext(ctx).GetCurrentTimeEntry()
}
func (r Remote) Start(ctx context.Context, workspace int64, project *int64, description string) (*toggl.TimeEntry, error) {
	return r.Client.WithContext(ctx).StartTimeEntry(workspace, project, description)
}
func (r Remote) Stop(ctx context.Context, workspace, entry int64) (*toggl.TimeEntry, error) {
	return r.Client.WithContext(ctx).StopTimeEntry(workspace, entry)
}
func (r Remote) Update(ctx context.Context, workspace, entry int64, project *int64, description string) (*toggl.TimeEntry, error) {
	return r.Client.WithContext(ctx).UpdateTimeEntry(workspace, entry, project, description)
}

// Snapshot contains the last known server state and no credentials.
type Snapshot struct {
	Profile      toggl.Me          `json:"profile"`
	Projects     []toggl.Project   `json:"projects"`
	Entries      []toggl.TimeEntry `json:"entries"`
	Current      *toggl.TimeEntry  `json:"current"`
	SyncedAt     time.Time         `json:"synced_at"`
	CacheWarning string            `json:"-"`
}

// Service serializes remote operations. State never waits for network I/O.
type Service struct {
	backend Backend
	path    string
	op      sync.Mutex
	mu      sync.RWMutex
	state   Snapshot
	retryAt time.Time
	now     func() time.Time
}

// NewService loads an account-specific cache, ignoring missing or invalid data.
func NewService(backend Backend, cachePath string) *Service {
	s := &Service{backend: backend, path: cachePath, now: time.Now}
	if data, err := os.ReadFile(cachePath); err == nil {
		var cached Snapshot
		if json.Unmarshal(data, &cached) == nil && cached.Profile.ID != 0 && cached.Profile.DefaultWorkspace != 0 {
			s.state = cached
		}
	}
	return s
}

// State returns a copy that can be read while a remote operation is in progress.
func (s *Service) State() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state := s.state
	state.Projects = append([]toggl.Project(nil), state.Projects...)
	state.Entries = append([]toggl.TimeEntry(nil), state.Entries...)
	if state.Current != nil {
		entry := *state.Current
		state.Current = &entry
	}
	return state
}

// Refresh fetches server state at most every five minutes unless forced.
// Quota backoff also applies to explicit refreshes and mutations.
func (s *Service) Refresh(ctx context.Context, force bool) error {
	s.op.Lock()
	defer s.op.Unlock()
	if err := s.available(); err != nil {
		return err
	}
	state, now := s.State(), s.now()
	if !force && !state.SyncedAt.IsZero() && now.Sub(state.SyncedAt) < 5*time.Minute {
		return nil
	}
	if state.Profile.ID == 0 {
		profile, err := s.backend.Profile(ctx)
		if err != nil {
			return s.remoteError(err)
		}
		if profile == nil || profile.ID == 0 || profile.DefaultWorkspace == 0 {
			return errors.New("the Toggl response did not include a default workspace")
		}
		state.Profile = *profile
	}
	entries, err := s.backend.Entries(ctx, now.AddDate(0, -3, 0), now)
	if err != nil {
		return s.remoteError(err)
	}
	current, err := s.backend.Current(ctx)
	if err != nil {
		return s.remoteError(err)
	}
	projects, err := s.backend.Projects(ctx, state.Profile.DefaultWorkspace, entries)
	if err != nil {
		return s.remoteError(err)
	}
	state.Entries, state.Current, state.Projects, state.SyncedAt = entries, current, projects, now
	s.store(state)
	return nil
}

// Start stops the known running entry before creating its replacement. A changed
// remote timer requires review. Mutations are never automatically retried.
func (s *Service) Start(ctx context.Context, workspace int64, project *int64, description string) error {
	s.op.Lock()
	defer s.op.Unlock()
	if err := s.available(); err != nil {
		return err
	}
	state := s.State()
	if state.Profile.ID == 0 {
		return errors.New("connect to Toggl before starting a timer")
	}
	if workspace == 0 {
		workspace = state.Profile.DefaultWorkspace
	}
	if workspace != state.Profile.DefaultWorkspace {
		return errors.New("this task belongs to another workspace; choose a task from the current workspace")
	}
	if project != nil {
		found := false
		for _, p := range state.Projects {
			if p.ID == *project && p.Active {
				found = true
				break
			}
		}
		if !found {
			return errors.New("this project is unavailable or archived; choose an active project for a new entry")
		}
	}
	current, err := s.checkCurrent(ctx, &state)
	if err != nil {
		return err
	}
	if current != nil {
		stopped, err := s.backend.Stop(ctx, current.WorkspaceID, current.ID)
		if err != nil {
			return s.remoteError(err)
		}
		if stopped == nil || stopped.ID != current.ID {
			return errors.New("the stop was not confirmed by Toggl; refresh before trying again")
		}
		state.Current = nil
		state.Entries = replaceEntry(state.Entries, *stopped)
		s.store(state)
	}
	entry, err := s.backend.Start(ctx, workspace, project, description)
	if err != nil {
		return fmt.Errorf("could not start the new entry; refresh before retrying: %w", s.remoteError(err))
	}
	if entry == nil || entry.ID == 0 {
		return errors.New("the new entry was not confirmed by Toggl; refresh before trying again")
	}
	state.Current = entry
	state.Entries = replaceEntry(state.Entries, *entry)
	s.store(state)
	return nil
}

// Stop rechecks the running entry so a stale UI cannot stop a different timer.
func (s *Service) Stop(ctx context.Context) error {
	s.op.Lock()
	defer s.op.Unlock()
	if err := s.available(); err != nil {
		return err
	}
	state := s.State()
	current, err := s.checkCurrent(ctx, &state)
	if err != nil {
		return err
	}
	if current == nil {
		return nil
	}
	stopped, err := s.backend.Stop(ctx, current.WorkspaceID, current.ID)
	if err != nil {
		return s.remoteError(err)
	}
	if stopped == nil || stopped.ID != current.ID {
		return errors.New("the stop was not confirmed by Toggl; refresh before trying again")
	}
	state.Entries, state.Current = replaceEntry(state.Entries, *stopped), nil
	s.store(state)
	return nil
}

// Update updates the title and project of the entry selected when the editor opened. It never
// stops or replaces a timer, and refuses to edit a different running entry.
func (s *Service) Update(ctx context.Context, entryID int64, project *int64, description string) error {
	s.op.Lock()
	defer s.op.Unlock()
	if err := s.available(); err != nil {
		return err
	}
	state := s.State()
	if state.Current == nil || state.Current.ID != entryID {
		return errors.New("the running timer changed; reopen Edit running task and try again")
	}
	workspace := state.Current.WorkspaceID
	if workspace != state.Profile.DefaultWorkspace {
		return errors.New("this timer belongs to another workspace; edit it in Toggl")
	}
	current, err := s.checkCurrent(ctx, &state)
	if err != nil {
		return err
	}
	entry := current
	projectChanged := !sameProject(current.ProjectID, project)
	if projectChanged || current.Description != description {
		if projectChanged && project != nil {
			found := false
			for _, p := range state.Projects {
				if p.ID == *project && p.Active && (p.WorkspaceID == 0 || p.WorkspaceID == workspace) {
					found = true
					break
				}
			}
			if !found {
				return errors.New("this project is unavailable or archived; choose an active project")
			}
		}
		entry, err = s.backend.Update(ctx, workspace, current.ID, project, description)
		if err != nil {
			return s.remoteError(err)
		}
		if entry == nil || entry.ID != current.ID || entry.WorkspaceID != workspace ||
			entry.Duration >= 0 || entry.Stop != nil || !entry.Start.Equal(current.Start) ||
			!sameProject(entry.ProjectID, project) || entry.Description != description {
			return errors.New("the task update was not confirmed by Toggl; refresh before trying again")
		}
	}
	state.Current, state.Entries = entry, replaceEntry(state.Entries, *entry)
	s.store(state)
	return nil
}

func sameProject(a, b *int64) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func (s *Service) checkCurrent(ctx context.Context, state *Snapshot) (*toggl.TimeEntry, error) {
	current, err := s.backend.Current(ctx)
	if err != nil {
		return nil, s.remoteError(err)
	}
	changed := current != nil && (state.Current == nil || state.Current.ID != current.ID) || current == nil && state.Current != nil
	if changed {
		// Reconcile history too: the previous running row needs its server-confirmed
		// stop time, otherwise local totals would keep advancing after it stopped.
		now := s.now()
		entries, err := s.backend.Entries(ctx, now.AddDate(0, -3, 0), now)
		if err != nil {
			return nil, s.remoteError(err)
		}
		state.Current, state.Entries = current, entries
		if current != nil {
			state.Entries = replaceEntry(state.Entries, *current)
		}
		s.store(*state)
		return nil, errors.New("the running timer changed elsewhere; review it and try again")
	}
	return current, nil
}

// LastStopped returns the latest stopped entry in the active workspace.
func LastStopped(state Snapshot) *toggl.TimeEntry {
	var last *toggl.TimeEntry
	for _, entry := range state.Entries {
		if entry.Duration < 0 || entry.WorkspaceID != state.Profile.DefaultWorkspace {
			continue
		}
		if last == nil || entry.Start.After(last.Start) {
			copy := entry
			last = &copy
		}
	}
	return last
}

func replaceEntry(entries []toggl.TimeEntry, entry toggl.TimeEntry) []toggl.TimeEntry {
	for i := range entries {
		if entries[i].ID == entry.ID {
			entries[i] = entry
			return entries
		}
	}
	return append(entries, entry)
}

func (s *Service) available() error {
	if s.now().Before(s.retryAt) {
		return fmt.Errorf("the Toggl request limit was reached; try again after %s", s.retryAt.Local().Format("15:04"))
	}
	return nil
}

func (s *Service) remoteError(err error) error {
	var apiErr *toggl.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case 401, 403:
			return errors.New("the request was rejected by Toggl; check your API token and workspace permissions in Connection")
		case 402, 429:
			delay := apiErr.RetryAfter
			if delay <= 0 {
				delay = time.Minute
				if apiErr.StatusCode == 402 {
					delay = time.Hour
				}
			}
			s.retryAt = s.now().Add(delay)
			return s.available()
		}
	}
	return err
}

func (s *Service) store(state Snapshot) {
	sort.SliceStable(state.Entries, func(i, j int) bool { return state.Entries[i].Start.After(state.Entries[j].Start) })
	state.CacheWarning = ""
	if err := s.persist(state); err != nil {
		state.CacheWarning = "Local history could not be saved; this session will still work. " + err.Error()
	}
	s.mu.Lock()
	s.state = state
	s.mu.Unlock()
}

func (s *Service) persist(state Snapshot) error {
	if s.path == "" {
		return nil
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return localfile.WritePrivate(s.path, data)
}
