// Package tracker contains the time-entry behavior shared by the desktop UI and TUI.
package tracker

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sahilm/fuzzy"
	"yati/internal/toggl"
)

// Suggestion identifies a description and project without losing their raw IDs.
type Suggestion struct {
	Description string
	ProjectID   *int64
	Project     string
	WorkspaceID int64
	LastUsed    time.Time
}

// Suggestions deduplicates description/project pairs and orders them by last use.
func Suggestions(entries []toggl.TimeEntry, projectNames map[int64]string) []Suggestion {
	type key struct {
		description string
		project     int64
		assigned    bool
		workspace   int64
	}
	unique := make(map[key]Suggestion)
	for _, entry := range entries {
		k := key{description: entry.Description, workspace: entry.WorkspaceID}
		project := ""
		if entry.ProjectID != nil {
			k.project, k.assigned = *entry.ProjectID, true
			project = projectNames[*entry.ProjectID]
		}
		if previous, ok := unique[k]; ok && !entry.Start.After(previous.LastUsed) {
			continue
		}
		unique[k] = Suggestion{entry.Description, entry.ProjectID, project, entry.WorkspaceID, entry.Start}
	}
	items := make([]Suggestion, 0, len(unique))
	for _, item := range unique {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if !a.LastUsed.Equal(b.LastUsed) {
			return a.LastUsed.After(b.LastUsed)
		}
		if !strings.EqualFold(a.Description, b.Description) {
			return strings.ToLower(a.Description) < strings.ToLower(b.Description)
		}
		if !strings.EqualFold(a.Project, b.Project) {
			return strings.ToLower(a.Project) < strings.ToLower(b.Project)
		}
		if a.WorkspaceID != b.WorkspaceID {
			return a.WorkspaceID < b.WorkspaceID
		}
		if a.Description != b.Description {
			return a.Description < b.Description
		}
		if (a.ProjectID == nil) != (b.ProjectID == nil) {
			return a.ProjectID == nil
		}
		return a.ProjectID != nil && *a.ProjectID < *b.ProjectID
	})
	return items
}

// FilterSuggestions searches descriptions and project names entirely in memory.
func FilterSuggestions(items []Suggestion, query string) []Suggestion {
	query = strings.TrimSpace(query)
	if query == "" {
		return items
	}
	texts := make([]string, len(items))
	for i, item := range items {
		texts[i] = item.Description + " " + item.Project
	}
	matches := fuzzy.Find(query, texts)
	filtered := make([]Suggestion, 0, len(matches))
	for _, match := range matches {
		filtered = append(filtered, items[match.Index])
	}
	return filtered
}

// PeriodBounds returns local calendar boundaries, including daylight saving changes.
func PeriodBounds(period string, now time.Time) (time.Time, time.Time) {
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch period {
	case "w":
		offset := (int(now.Weekday()) + 6) % 7
		start = start.AddDate(0, 0, -offset)
		return start, start.AddDate(0, 0, 7)
	case "m":
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		return start, start.AddDate(0, 1, 0)
	default:
		return start, start.AddDate(0, 0, 1)
	}
}

// EntryDuration returns elapsed time without contacting the server.
func EntryDuration(entry toggl.TimeEntry, now time.Time) time.Duration {
	if entry.Duration < 0 {
		return max(0, now.Sub(entry.Start))
	}
	return max(0, time.Duration(entry.Duration)*time.Second)
}

// FormatDuration formats durations without wrapping hours at 24.
func FormatDuration(duration time.Duration) string {
	seconds := int64(max(0, duration) / time.Second)
	return fmt.Sprintf("%02d:%02d:%02d", seconds/3600, seconds/60%60, seconds%60)
}
