package tracker

import (
	"testing"
	"time"

	"yati/internal/toggl"
)

func TestTodayDurationMatchesDailyEntriesAcrossWorkspaces(t *testing.T) {
	now := time.Date(2026, time.November, 10, 12, 0, 0, 0, time.UTC)
	entries := []toggl.TimeEntry{
		{ID: 1, WorkspaceID: 42, Start: now.Add(-12 * time.Hour), Duration: 3600},
		{ID: 2, WorkspaceID: 42, Start: now.Add(-2 * time.Hour), Duration: -1},
		{ID: 3, WorkspaceID: 99, Start: now.Add(-time.Hour), Duration: 900},
		{ID: 4, Start: now.Add(-13 * time.Hour), Duration: 7200}, // Started yesterday, even though it ended today.
		{ID: 5, Start: now.Add(12 * time.Hour), Duration: 600},   // Tomorrow's boundary is excluded.
		{ID: 6, Start: now.Add(time.Hour), Duration: -1},         // Future running time cannot be negative.
	}
	current := &toggl.TimeEntry{ID: 2, WorkspaceID: 42, Start: now.Add(-90 * time.Minute), Duration: -1}
	if got := TodayDuration(entries, current, now); got != 2*time.Hour+45*time.Minute {
		t.Fatalf("got %s; want stopped time plus the corrected running row, counted once", got)
	}
	if got := TodayDuration(nil, current, now); got != 90*time.Minute {
		t.Fatalf("missing running row was not included: %s", got)
	}
	current.Start = now.Add(-24 * time.Hour)
	if got := TodayDuration(entries[1:2], current, now); got != 0 {
		t.Fatalf("cached start overrode the current entry's previous-day start: %s", got)
	}
	if got := TodayDuration(entries, nil, now.AddDate(0, 0, 2)); got != 0 {
		t.Fatalf("old entries carried into a new day: %s", got)
	}
	if got := TodayDuration(nil, nil, now); got != 0 {
		t.Fatal(got)
	}
}

func TestTodayDurationUsesLocalCalendarAndDaylightSaving(t *testing.T) {
	zone, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		month time.Month
		day   int
		want  time.Duration
	}{
		{time.March, 29, 11 * time.Hour},
		{time.October, 25, 13 * time.Hour},
	} {
		now := time.Date(2026, test.month, test.day, 12, 0, 0, 0, zone)
		midnight := time.Date(2026, test.month, test.day, 0, 0, 0, 0, zone)
		current := &toggl.TimeEntry{ID: 1, Start: midnight, Duration: -1}
		if got := TodayDuration(nil, current, now); got != test.want {
			t.Fatalf("%s: got %s, want %s", now.Format(time.DateOnly), got, test.want)
		}
	}
	now := time.Date(2026, time.November, 10, 0, 30, 0, 0, zone)
	entries := []toggl.TimeEntry{
		{ID: 1, Start: now.Add(-15 * time.Minute).UTC(), Duration: 600},
		{ID: 2, Start: now.Add(-40 * time.Minute).UTC(), Duration: 1200},
	}
	if got := TodayDuration(entries, nil, now); got != 10*time.Minute {
		t.Fatalf("used UTC date instead of local date: %s", got)
	}
}
