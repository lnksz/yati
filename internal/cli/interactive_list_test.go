package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
)

func makeItems(durations ...time.Duration) []list.Item {
	items := make([]list.Item, len(durations))
	for i, d := range durations {
		items[i] = listItem{
			desc:     "task",
			duration: d,
			date:     "2026-01-01 09:00",
		}
	}
	return items
}

func makeMixedItems() []list.Item {
	return []list.Item{
		listItem{desc: "meeting", duration: 54*time.Minute + 30*time.Second, date: "2026-01-01 09:00"},
		listItem{desc: "coding", duration: 2*time.Hour + 15*time.Second, date: "2026-01-01 10:00"},
		listItem{desc: "review", duration: 45 * time.Second, date: "2026-01-01 11:00"},
	}
}

func TestVisibleDurationSum_AllItems(t *testing.T) {
	l := list.New(makeMixedItems(), list.NewDefaultDelegate(), 80, 20)
	total := visibleDurationSum(l)
	expected := (54*time.Minute + 30*time.Second) + (2*time.Hour + 15*time.Second) + 45*time.Second
	if total != expected {
		t.Errorf("expected %v, got %v", expected, total)
	}
}

func TestVisibleDurationSum_Filtered(t *testing.T) {
	l := list.New(makeMixedItems(), list.NewDefaultDelegate(), 80, 20)
	l.SetFilterText("meet")
	total := visibleDurationSum(l)
	expected := 54*time.Minute + 30*time.Second
	if total != expected {
		t.Errorf("expected %v, got %v", expected, total)
	}
}

func TestVisibleDurationSum_NoMatch(t *testing.T) {
	l := list.New(makeMixedItems(), list.NewDefaultDelegate(), 80, 20)
	l.SetFilterText("zzzz")
	total := visibleDurationSum(l)
	if total != 0 {
		t.Errorf("expected 0, got %v", total)
	}
}

func TestVisibleDurationSum_ClearedFilter(t *testing.T) {
	l := list.New(makeMixedItems(), list.NewDefaultDelegate(), 80, 20)
	l.SetFilterText("meet")
	l.SetFilterText("")
	total := visibleDurationSum(l)
	expected := 54*time.Minute + 30*time.Second + 2*time.Hour + 15*time.Second + 45*time.Second
	if total != expected {
		t.Errorf("after clearing filter: expected %v, got %v", expected, total)
	}
}

func TestFormatDuration_Rounding(t *testing.T) {
	tests := []struct {
		input    time.Duration
		expected string
	}{
		{0, "Total duration: 0s"},
		{300 * time.Millisecond, "Total duration: 0s"},
		{500 * time.Millisecond, "Total duration: 1s"},
		{59*time.Minute + 59*time.Second + 999*time.Millisecond, "Total duration: 1h0m0s"},
		{1*time.Hour + 30*time.Minute + 45*time.Second, "Total duration: 1h30m45s"},
	}
	for _, tt := range tests {
		got := formatDuration(tt.input)
		if got != tt.expected {
			t.Errorf("formatDuration(%v): expected %q, got %q", tt.input, tt.expected, got)
		}
	}
}

func TestListItem_Description_RoundsSeconds(t *testing.T) {
	li := listItem{
		desc:     "task",
		duration: 2*time.Minute + 30*time.Second + 750*time.Millisecond,
		date:     "2026-01-01 09:00",
	}
	got := li.Description()
	if !strings.Contains(got, "Duration: 2m31s") {
		t.Errorf("expected rounded duration in description, got: %s", got)
	}
}

func TestView_ContainsTotal(t *testing.T) {
	l := list.New(makeMixedItems(), list.NewDefaultDelegate(), 80, 20)
	m := listModel{list: l}
	m.list.SetSize(80, 20)
	m.list.SetShowStatusBar(false)
	m.list.SetShowHelp(false)
	m.list.SetShowPagination(false)
	m.list.SetShowTitle(false)
	view := m.View()
	if !strings.Contains(view, "Total duration:") {
		t.Errorf("view missing total duration")
	}
	if !strings.Contains(view, "2h55m30s") {
		t.Errorf("view missing expected total 2h55m30s")
	}
}

func TestView_FilteredTotal(t *testing.T) {
	l := list.New(makeMixedItems(), list.NewDefaultDelegate(), 80, 20)
	l.SetFilterText("cod")
	m := listModel{list: l}
	m.list.SetSize(80, 20)
	m.list.SetShowStatusBar(false)
	m.list.SetShowHelp(false)
	m.list.SetShowPagination(false)
	m.list.SetShowTitle(false)
	view := m.View()
	if !strings.Contains(view, "2h0m15s") {
		t.Errorf("expected filtered total 2h0m15s")
	}
}

func TestView_NoMatchShowsZero(t *testing.T) {
	l := list.New(makeMixedItems(), list.NewDefaultDelegate(), 80, 20)
	l.SetFilterText("zzz")
	m := listModel{list: l}
	m.list.SetSize(80, 20)
	m.list.SetShowStatusBar(false)
	m.list.SetShowHelp(false)
	m.list.SetShowPagination(false)
	m.list.SetShowTitle(false)
	view := m.View()
	if !strings.Contains(view, "Total duration: 0s") {
		t.Errorf("expected 0s total for no matches")
	}
}

func TestVisibleDurationSum_ManyItems(t *testing.T) {
	items := make([]list.Item, 20)
	for i := range items {
		items[i] = listItem{
			desc:     "task",
			duration: 1 * time.Minute,
			date:     "2026-01-01 09:00",
		}
	}
	l := list.New(items, list.NewDefaultDelegate(), 80, 3)
	l.SetShowPagination(true)
	total := visibleDurationSum(l)
	expected := 20 * time.Minute
	if total != expected {
		t.Errorf("expected %v across pages, got %v", expected, total)
	}
}
