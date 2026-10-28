package tracker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"yati/internal/toggl"
)

type fakeBackend struct {
	current  *toggl.TimeEntry
	entries  []toggl.TimeEntry
	calls    []string
	failure  error
	failStop bool
	update   func(context.Context, int64, int64, *int64, string) (*toggl.TimeEntry, error)
}

func (f *fakeBackend) Profile(context.Context) (*toggl.Me, error) {
	f.calls = append(f.calls, "profile")
	return &toggl.Me{ID: 1, DefaultWorkspace: 42}, f.failure
}
func (f *fakeBackend) Entries(context.Context, time.Time, time.Time) ([]toggl.TimeEntry, error) {
	f.calls = append(f.calls, "entries")
	return append([]toggl.TimeEntry(nil), f.entries...), f.failure
}
func (f *fakeBackend) Projects(context.Context, int64, []toggl.TimeEntry) ([]toggl.Project, error) {
	f.calls = append(f.calls, "projects")
	return []toggl.Project{{ID: 7, WorkspaceID: 42, Name: "Project", Active: true}}, f.failure
}
func (f *fakeBackend) Current(context.Context) (*toggl.TimeEntry, error) {
	f.calls = append(f.calls, "current")
	return f.current, f.failure
}
func (f *fakeBackend) Start(_ context.Context, workspace int64, project *int64, description string) (*toggl.TimeEntry, error) {
	f.calls = append(f.calls, "start")
	if f.failure != nil {
		return nil, f.failure
	}
	f.current = &toggl.TimeEntry{ID: 99, WorkspaceID: workspace, ProjectID: project, Description: description, Start: time.Now(), Duration: -1}
	return f.current, nil
}
func (f *fakeBackend) Stop(_ context.Context, workspace, id int64) (*toggl.TimeEntry, error) {
	f.calls = append(f.calls, "stop")
	if f.failStop {
		return nil, errors.New("stop rejected")
	}
	stopped := *f.current
	stopped.Duration = 120
	f.current = nil
	return &stopped, f.failure
}

func (f *fakeBackend) Update(ctx context.Context, workspace, id int64, project *int64, description string) (*toggl.TimeEntry, error) {
	f.calls = append(f.calls, "update")
	if f.update != nil {
		return f.update(ctx, workspace, id, project, description)
	}
	if f.current == nil || f.current.ID != id || f.current.WorkspaceID != workspace {
		return nil, errors.New("unexpected entry to update")
	}
	entry := *f.current
	entry.ProjectID, entry.Description = project, description
	f.current = &entry
	return &entry, nil
}

func loadedService(t *testing.T, backend *fakeBackend) *Service {
	t.Helper()
	s := NewService(backend, filepath.Join(t.TempDir(), "gui.json"))
	if err := s.Refresh(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	backend.calls = nil
	return s
}

func TestSwitchStopsBeforeStartAndPreservesProject(t *testing.T) {
	backend := &fakeBackend{current: &toggl.TimeEntry{ID: 1, WorkspaceID: 42, Duration: -1, Start: time.Now().Add(-time.Minute)}}
	s := loadedService(t, backend)
	project := int64(7)
	if err := s.Start(context.Background(), 42, &project, "Exact <description> & text"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(backend.calls, []string{"current", "stop", "start"}) {
		t.Fatalf("unexpected order: %v", backend.calls)
	}
	state := s.State()
	if state.Current == nil || *state.Current.ProjectID != 7 || state.Current.Description != "Exact <description> & text" {
		t.Fatalf("lost entry identity: %#v", state.Current)
	}
	if len(state.Entries) != 2 {
		t.Fatalf("expected stopped and running entries, got %v", state.Entries)
	}
}

func TestFailedStopNeverStartsReplacement(t *testing.T) {
	backend := &fakeBackend{current: &toggl.TimeEntry{ID: 1, WorkspaceID: 42, Duration: -1}, failStop: true}
	s := loadedService(t, backend)
	if err := s.Start(context.Background(), 42, nil, "Replacement"); err == nil {
		t.Fatal("expected failure")
	}
	if !reflect.DeepEqual(backend.calls, []string{"current", "stop"}) {
		t.Fatalf("unexpected calls: %v", backend.calls)
	}
	if s.State().Current == nil {
		t.Fatal("lost running entry after failed stop")
	}
}

func TestRemoteTimerChangeRequiresReview(t *testing.T) {
	for _, operation := range []string{"start", "stop"} {
		t.Run(operation, func(t *testing.T) {
			backend := &fakeBackend{}
			s := loadedService(t, backend)
			backend.current = &toggl.TimeEntry{ID: 12, WorkspaceID: 42, Description: "Changed on phone", Duration: -1}
			var err error
			if operation == "start" {
				err = s.Start(context.Background(), 42, nil, "Next")
			} else {
				err = s.Stop(context.Background())
			}
			if err == nil || !strings.Contains(err.Error(), "changed elsewhere") {
				t.Fatalf("expected review message, got %v", err)
			}
			if !reflect.DeepEqual(backend.calls, []string{"current", "entries"}) {
				t.Fatalf("mutated changed timer: %v", backend.calls)
			}
			if s.State().Current.ID != 12 {
				t.Fatal("did not reconcile changed timer")
			}
		})
	}
}

func TestExternalStopReconcilesHistoryBeforeAnotherWrite(t *testing.T) {
	running := toggl.TimeEntry{ID: 1, WorkspaceID: 42, Start: time.Now().Add(-time.Hour), Duration: -1}
	backend := &fakeBackend{current: &running, entries: []toggl.TimeEntry{running}}
	s := loadedService(t, backend)
	backend.current = nil
	backend.entries[0].Duration = 120
	if err := s.Start(context.Background(), 42, nil, "Next task"); err == nil || !strings.Contains(err.Error(), "changed elsewhere") {
		t.Fatalf("expected review of the external stop, got %v", err)
	}
	state := s.State()
	if state.Current != nil || len(state.Entries) != 1 || state.Entries[0].Duration != 120 {
		t.Fatalf("history still contains a stale running row: %+v", state)
	}
	if !reflect.DeepEqual(backend.calls, []string{"current", "entries"}) {
		t.Fatalf("unexpected mutation: %v", backend.calls)
	}
}

func TestRefreshThrottleAndQuotaBackoff(t *testing.T) {
	backend := &fakeBackend{}
	s := loadedService(t, backend)
	if err := s.Refresh(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if len(backend.calls) != 0 {
		t.Fatalf("warm reopen fetched remote data: %v", backend.calls)
	}
	backend.failure = &toggl.APIError{StatusCode: 402, RetryAfter: 30 * time.Minute}
	if err := s.Refresh(context.Background(), true); err == nil {
		t.Fatal("expected quota failure")
	}
	backend.calls = nil
	if err := s.Refresh(context.Background(), true); err == nil {
		t.Fatal("forced refresh bypassed quota backoff")
	}
	if err := s.Start(context.Background(), 42, nil, "Task"); err == nil {
		t.Fatal("mutation bypassed quota backoff")
	}
	if len(backend.calls) != 0 {
		t.Fatalf("requests sent during backoff: %v", backend.calls)
	}
}

func TestCacheIsPrivateAndWorksWithoutStorage(t *testing.T) {
	backend := &fakeBackend{}
	s := loadedService(t, backend)
	info, err := os.Stat(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("cache permissions: %v", info.Mode())
	}
	loaded := NewService(backend, s.path)
	if loaded.State().Profile.ID != 1 {
		t.Fatal("cache not restored")
	}
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	s = NewService(backend, filepath.Join(blocked, "cache.json"))
	if err := s.Refresh(context.Background(), true); err != nil {
		t.Fatalf("cache failure prevented remote use: %v", err)
	}
	if s.State().Profile.ID == 0 || s.State().CacheWarning == "" {
		t.Fatal("missing state or storage warning")
	}
}

func TestUnavailableProjectRejectedBeforeRequests(t *testing.T) {
	backend := &fakeBackend{}
	s := loadedService(t, backend)
	project := int64(100)
	if err := s.Start(context.Background(), 42, &project, "Task"); err == nil {
		t.Fatal("accepted an unavailable project")
	}
	if len(backend.calls) != 0 {
		t.Fatalf("unexpected requests: %v", backend.calls)
	}
}
