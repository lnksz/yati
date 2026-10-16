package tracker

import (
	"testing"
	"time"

	"yati/internal/toggl"
)

func TestSuggestionIdentityAndProjectSearch(t *testing.T) {
	first, second := int64(1), int64(2)
	now := time.Now()
	items := Suggestions([]toggl.TimeEntry{
		{Description: "Same task", ProjectID: &first, WorkspaceID: 42, Start: now},
		{Description: "Same task", ProjectID: &first, WorkspaceID: 42, Start: now.Add(-time.Hour)},
		{Description: "Same task", ProjectID: &second, WorkspaceID: 42, Start: now.Add(-time.Minute)},
		{Description: "Same task", ProjectID: &first, WorkspaceID: 99, Start: now},
	}, map[int64]string{1: "Firmware", 2: "Documentation"})
	if len(items) != 3 {
		t.Fatalf("merged distinct projects/workspaces: %#v", items)
	}
	matches := FilterSuggestions(items, "doc")
	if len(matches) != 1 || *matches[0].ProjectID != second {
		t.Fatalf("project search: %#v", matches)
	}
}

func TestLocalPeriodBoundsAcrossDaylightSaving(t *testing.T) {
	zone, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		day   int
		hours time.Duration
	}{{29, 23 * time.Hour}, {30, 24 * time.Hour}} {
		t.Run(time.Date(2026, 3, test.day, 12, 0, 0, 0, zone).Format(time.DateOnly), func(t *testing.T) {
			start, end := PeriodBounds("d", time.Date(2026, 3, test.day, 12, 0, 0, 0, zone))
			if end.Sub(start) != test.hours {
				t.Fatalf("calendar day lasted %v, want %v", end.Sub(start), test.hours)
			}
		})
	}
	start, end := PeriodBounds("w", time.Date(2026, 10, 18, 12, 0, 0, 0, zone))
	if start.Weekday() != time.Monday || end.Day() != 19 {
		t.Fatalf("bad Monday/Sunday week: %v..%v", start, end)
	}
}

func TestElapsedNeverNegativeOrWrapsAt24Hours(t *testing.T) {
	now := time.Now()
	if EntryDuration(toggl.TimeEntry{Duration: -1, Start: now.Add(time.Hour)}, now) != 0 {
		t.Fatal("future entry has negative duration")
	}
	if got := FormatDuration(49*time.Hour + 2*time.Minute + 3*time.Second); got != "49:02:03" {
		t.Fatal(got)
	}
}
