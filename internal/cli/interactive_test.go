package cli

import (
	"testing"
	"time"

	"yati/internal/toggl"
)

func TestInteractiveHistoryStart(t *testing.T) {
	endDate := time.Date(2026, time.September, 7, 14, 30, 0, 0, time.FixedZone("test", 2*60*60))
	want := time.Date(2026, time.June, 7, 14, 30, 0, 0, endDate.Location())

	if got := interactiveHistoryStart(endDate); !got.Equal(want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestTaskSuggestionsByLastUsed(t *testing.T) {
	entries := []toggl.TimeEntry{
		{Description: "Task", ProjectID: interactiveInt64Pointer(1), Start: time.Date(2026, time.September, 1, 9, 0, 0, 0, time.UTC)},
		{Description: "Other", ProjectID: interactiveInt64Pointer(2), Start: time.Date(2026, time.September, 10, 9, 0, 0, 0, time.UTC)},
		{Description: "Task", ProjectID: interactiveInt64Pointer(1), Start: time.Date(2026, time.September, 8, 9, 0, 0, 0, time.UTC)},
		{Description: "Task", ProjectID: interactiveInt64Pointer(3), Start: time.Date(2026, time.September, 5, 9, 0, 0, 0, time.UTC)},
		{Description: "Unassigned", Start: time.Date(2026, time.September, 2, 9, 0, 0, 0, time.UTC)},
	}
	projectNames := map[int64]string{1: "One", 2: "Two", 3: "Three"}

	items := taskSuggestionsByLastUsed(entries, projectNames)
	if len(items) != 4 {
		t.Fatalf("got %d suggestions, want 4", len(items))
	}
	wantDescriptions := []string{"Other", "Task", "Task", "Unassigned"}
	wantProjects := []string{"Two", "One", "Three", ""}
	for i := range items {
		if items[i].desc != wantDescriptions[i] || items[i].project != wantProjects[i] {
			t.Fatalf("position %d: got %q/%q, want %q/%q", i, items[i].desc, items[i].project, wantDescriptions[i], wantProjects[i])
		}
	}
}

func interactiveInt64Pointer(value int64) *int64 {
	return &value
}
