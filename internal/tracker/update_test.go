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
	// JSON decodes a Z timestamp as time.UTC, which differs from time.Local
	// even on UTC hosts. Keep the fixture canonical for the deep comparison.
	entry := toggl.TimeEntry{ID: 1, WorkspaceID: 42, Description: "Exact <description> & Überprüfen", Start: time.Now().UTC().Add(-time.Hour).Truncate(time.Second), Duration: -1, Tags: []string{"keep"}}
	backend := &fakeBackend{current: &entry, entries: []toggl.TimeEntry{entry, {ID: 2, WorkspaceID: 42, Duration: 120}}}
	s := loadedService(t, backend)
	earlier, later := entry.Start.Add(-24*time.Hour), entry.Start.Add(30*time.Minute)
	want := entry
	for _, edit := range []struct {
		project *int64
		title   string
		start   *time.Time
	}{
		{&project, entry.Description, nil},
		{&project, "Renamed <title> & Überprüfen", nil},
		{&project, "Renamed <title> & Überprüfen", &earlier},
		{nil, "Changed title, project and start", &later},
		{nil, "", nil},
	} {
		backend.calls = nil
		if err := s.Update(context.Background(), entry.ID, edit.project, edit.title, edit.start); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(backend.calls, []string{"current", "update"}) {
			t.Fatalf("edit stopped or started a timer: %v", backend.calls)
		}
		want.ProjectID, want.Description = edit.project, edit.title
		if edit.start != nil {
			want.Start = *edit.start
		}
		state := s.State()
		if !reflect.DeepEqual(state.Current, &want) || len(state.Entries) != 2 || !reflect.DeepEqual(state.Entries[0], want) {
			t.Fatalf("edit lost entry identity or history: %+v", state)
		}
		cached := NewService(backend, s.path).State()
		if !reflect.DeepEqual(cached.Current, &want) || !reflect.DeepEqual(cached.Entries, state.Entries) {
			t.Fatalf("edit was not persisted: %+v", cached)
		}
		now := time.Now()
		if elapsed := EntryDuration(*state.Current, now); elapsed != now.Sub(want.Start) {
			t.Fatalf("elapsed time did not follow the edited start: %s", elapsed)
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
			start := time.Now().Add(-time.Hour)
			if err := s.Update(context.Background(), expected, nil, "Renamed", &start); err == nil || !strings.Contains(err.Error(), "changed") {
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
			backend.update = func(context.Context, int64, int64, *int64, string, *time.Time) (*toggl.TimeEntry, error) {
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
			if err := s.Update(context.Background(), entry.ID, &project, "Renamed", nil); err == nil {
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
		err := s.Update(context.Background(), entry.ID, &id, entry.Description, nil)
		if (id == 7) != (err == nil) {
			t.Fatalf("project %d: unexpected result %v", id, err)
		}
		if !reflect.DeepEqual(backend.calls, []string{"current"}) || !reflect.DeepEqual(s.State().Current, &entry) {
			t.Fatalf("unexpected project mutation: %v, %+v", backend.calls, s.State().Current)
		}
	}
	// A title-only edit must work even if the current project was archived.
	s.state.Projects[0].Active = false
	if err := s.Update(context.Background(), entry.ID, &project, "Renamed archived task", nil); err != nil {
		t.Fatal(err)
	}
	if got := s.State().Current; got.Description != "Renamed archived task" || !sameProject(got.ProjectID, &project) {
		t.Fatalf("title-only edit changed the project: %+v", got)
	}
}

func TestUpdateStartRejectsInvalidTimesAndUnconfirmedChange(t *testing.T) {
	entry := toggl.TimeEntry{ID: 1, WorkspaceID: 42, Start: time.Now().UTC().Add(-time.Hour), Duration: -1}
	for _, start := range []time.Time{{}, time.Now().Add(time.Hour)} {
		backend := &fakeBackend{current: &entry}
		s := loadedService(t, backend)
		if err := s.Update(context.Background(), entry.ID, nil, entry.Description, &start); err == nil {
			t.Fatal("accepted an invalid start time")
		}
		if len(backend.calls) != 0 || !s.State().Current.Start.Equal(entry.Start) {
			t.Fatalf("invalid start caused a remote operation: %v", backend.calls)
		}
	}
	backend := &fakeBackend{current: &entry}
	s := loadedService(t, backend)
	backend.update = func(context.Context, int64, int64, *int64, string, *time.Time) (*toggl.TimeEntry, error) {
		return &entry, nil // Simulate the server ignoring the requested start.
	}
	start := entry.Start.Add(-time.Hour)
	if err := s.Update(context.Background(), entry.ID, nil, entry.Description, &start); err == nil {
		t.Fatal("accepted an unconfirmed start change")
	}
	if !s.State().Current.Start.Equal(entry.Start) {
		t.Fatal("unconfirmed start changed local state")
	}
}

func TestUpdateUntouchedStartPreservesExternalCorrection(t *testing.T) {
	entry := toggl.TimeEntry{ID: 1, WorkspaceID: 42, Start: time.Now().UTC().Add(-time.Hour), Duration: -1}
	backend := &fakeBackend{current: &entry, entries: []toggl.TimeEntry{entry}}
	s := loadedService(t, backend)
	corrected := entry
	corrected.Start = entry.Start.Add(-time.Hour)
	backend.current = &corrected
	if err := s.Update(context.Background(), entry.ID, nil, "Renamed", nil); err != nil {
		t.Fatal(err)
	}
	if !s.State().Current.Start.Equal(corrected.Start) {
		t.Fatal("title edit overwrote an external start-time correction")
	}
}
