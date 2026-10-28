package tracker

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"yati/internal/toggl"
)

func TestUpdatePreservesRunningEntryAndHistory(t *testing.T) {
	project := int64(7)
	entry := toggl.TimeEntry{ID: 1, WorkspaceID: 42, Description: "Exact <description> & Überprüfen", Start: time.Now().Add(-time.Hour).Truncate(time.Second), Duration: -1, Tags: []string{"keep"}}
	backend := &fakeBackend{current: &entry, entries: []toggl.TimeEntry{entry, {ID: 2, WorkspaceID: 42, Duration: 120}}}
	s := loadedService(t, backend)
	for _, edit := range []struct {
		project *int64
		title   string
	}{
		{&project, entry.Description},
		{&project, "Renamed <title> & Überprüfen"},
		{nil, "Changed title and project"},
		{nil, ""},
	} {
		backend.calls = nil
		if err := s.Update(context.Background(), entry.ID, edit.project, edit.title); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(backend.calls, []string{"current", "update"}) {
			t.Fatalf("edit stopped or started a timer: %v", backend.calls)
		}
		want := entry
		want.ProjectID, want.Description = edit.project, edit.title
		state := s.State()
		if !reflect.DeepEqual(state.Current, &want) || len(state.Entries) != 2 || !reflect.DeepEqual(state.Entries[0], want) {
			t.Fatalf("edit lost entry identity or history: %+v", state)
		}
		cached := NewService(backend, s.path).State()
		if !reflect.DeepEqual(cached.Current, &want) || !reflect.DeepEqual(cached.Entries, state.Entries) {
			t.Fatalf("edit was not persisted: %+v", cached)
		}
	}
}

func TestUpdateRejectsChangedTimer(t *testing.T) {
	for _, change := range []string{"replaced", "stopped", "stale editor"} {
		t.Run(change, func(t *testing.T) {
			entry := toggl.TimeEntry{ID: 1, WorkspaceID: 42, Duration: -1}
			backend := &fakeBackend{current: &entry, entries: []toggl.TimeEntry{entry}}
			s := loadedService(t, backend)
			expected := entry.ID
			wantCalls := []string{"current", "entries"}
			switch change {
			case "replaced":
				backend.current = &toggl.TimeEntry{ID: 3, WorkspaceID: 42, Duration: -1}
			case "stopped":
				backend.current = nil
			case "stale editor":
				expected = 3
				wantCalls = nil
			}
			if err := s.Update(context.Background(), expected, nil, "Renamed"); err == nil || !strings.Contains(err.Error(), "changed") {
				t.Fatalf("expected changed timer error, got %v", err)
			}
			if !reflect.DeepEqual(backend.calls, wantCalls) {
				t.Fatalf("mutated a different timer: %v", backend.calls)
			}
			if !reflect.DeepEqual(s.State().Current, backend.current) {
				t.Fatal("did not reconcile the changed timer")
			}
		})
	}
}

func TestUpdateKeepsStateOnFailure(t *testing.T) {
	project := int64(7)
	for _, failure := range []string{"network", "empty", "wrong entry", "wrong workspace", "wrong project", "wrong title", "reset start", "stopped"} {
		t.Run(failure, func(t *testing.T) {
			entry := toggl.TimeEntry{ID: 1, WorkspaceID: 42, Start: time.Now().Add(-time.Hour), Duration: -1}
			backend := &fakeBackend{current: &entry, entries: []toggl.TimeEntry{entry}}
			s := loadedService(t, backend)
			before := s.State()
			backend.update = func(context.Context, int64, int64, *int64, string) (*toggl.TimeEntry, error) {
				updated := entry
				updated.ProjectID, updated.Description = &project, "Renamed"
				switch failure {
				case "network":
					return nil, errors.New("connection lost")
				case "empty":
					return nil, nil
				case "wrong entry":
					updated.ID++
				case "wrong workspace":
					updated.WorkspaceID++
				case "wrong project":
					updated.ProjectID = nil
				case "wrong title":
					updated.Description = "Old title"
				case "reset start":
					updated.Start = time.Now()
				case "stopped":
					updated.Duration = 3600
				}
				return &updated, nil
			}
			if err := s.Update(context.Background(), entry.ID, &project, "Renamed"); err == nil {
				t.Fatal("accepted an unconfirmed update")
			}
			if !reflect.DeepEqual(s.State(), before) || !reflect.DeepEqual(backend.calls, []string{"current", "update"}) {
				t.Fatalf("failed edit changed local state or retried: %+v, %v", s.State(), backend.calls)
			}
		})
	}
}

func TestUpdateValidatesProjectAndSkipsUnchangedValues(t *testing.T) {
	project := int64(7)
	entry := toggl.TimeEntry{ID: 1, WorkspaceID: 42, ProjectID: &project, Duration: -1}
	backend := &fakeBackend{current: &entry}
	s := loadedService(t, backend)
	s.state.Projects = append(s.state.Projects, toggl.Project{ID: 8, WorkspaceID: 42}, toggl.Project{ID: 9, WorkspaceID: 99, Active: true})
	for _, id := range []int64{7, 8, 9, 10} {
		backend.calls = nil
		err := s.Update(context.Background(), entry.ID, &id, entry.Description)
		if (id == 7) != (err == nil) {
			t.Fatalf("project %d: unexpected result %v", id, err)
		}
		if !reflect.DeepEqual(backend.calls, []string{"current"}) || !reflect.DeepEqual(s.State().Current, &entry) {
			t.Fatalf("unexpected project mutation: %v, %+v", backend.calls, s.State().Current)
		}
	}
	// A title-only edit must work even if the current project was archived.
	s.state.Projects[0].Active = false
	if err := s.Update(context.Background(), entry.ID, &project, "Renamed archived task"); err != nil {
		t.Fatal(err)
	}
	if got := s.State().Current; got.Description != "Renamed archived task" || !sameProject(got.ProjectID, &project) {
		t.Fatalf("title-only edit changed the project: %+v", got)
	}
}
