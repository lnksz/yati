package toggl

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDecodeNull(t *testing.T) {
	var entry TimeEntry
	err := json.NewDecoder(strings.NewReader("null")).Decode(&entry)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if entry.ID != 0 {
		t.Errorf("Expected ID 0, got %d", entry.ID)
	}

	var entry2 TimeEntry
	err = json.NewDecoder(strings.NewReader("")).Decode(&entry2)
	if err != io.EOF {
		t.Errorf("Expected io.EOF, got %v", err)
	}
}

func TestNewClientUsesXDGCacheHome(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	client := NewClient("token")
	want := filepath.Join(cacheHome, "yati", "cache.json")
	if client.cachePath != want {
		t.Fatalf("cache path: got %q, want %q", client.cachePath, want)
	}
}

func TestGetTargetsCachesProjectsClientsAndUsageOrderForDay(t *testing.T) {
	var projectCalls, clientCalls, timeEntryCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/workspaces/42/projects":
			projectCalls++
			fmt.Fprint(w, `[{"id":1,"workspace_id":42,"name":"Old","active":true},{"id":3,"workspace_id":42,"name":"Recent","active":true},{"id":2,"workspace_id":42,"name":"alpha","active":true}]`)
		case "/workspaces/42/clients":
			clientCalls++
			fmt.Fprint(w, `[{"id":2,"workspace_id":42,"name":"Client","archived":false}]`)
		case "/me/time_entries":
			timeEntryCalls++
			if r.URL.Query().Get("start_date") == "" || r.URL.Query().Get("end_date") == "" {
				t.Error("time-entry request is missing its date range")
			}
			fmt.Fprint(w, `[{"id":10,"project_id":1,"start":"2026-08-01T09:00:00Z"},{"id":11,"project_id":3,"start":"2026-09-14T09:00:00Z"}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	today := time.Date(2026, time.September, 15, 12, 0, 0, 0, time.Local)
	client := NewClient("token")
	client.baseURL = server.URL
	client.cachePath = filepath.Join(t.TempDir(), "yati", "cache.json")
	client.now = func() time.Time { return today }

	projects, clients, err := client.GetTargets(42)
	if err != nil {
		t.Fatalf("first GetTargets failed: %v", err)
	}
	if len(projects) != 3 || projects[0].Name != "Recent" || projects[1].Name != "Old" || projects[2].Name != "alpha" {
		t.Fatalf("unexpected projects: %#v", projects)
	}
	if len(clients) != 1 || clients[0].Name != "Client" {
		t.Fatalf("unexpected clients: %#v", clients)
	}

	if _, _, err := client.GetTargets(42); err != nil {
		t.Fatalf("cached GetTargets failed: %v", err)
	}
	if projectCalls != 1 || clientCalls != 1 || timeEntryCalls != 1 {
		t.Fatalf("same-day calls: projects=%d, clients=%d, time entries=%d; want 1 each", projectCalls, clientCalls, timeEntryCalls)
	}

	data, err := os.ReadFile(client.cachePath)
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}
	var cached map[string]json.RawMessage
	if err := json.Unmarshal(data, &cached); err != nil {
		t.Fatalf("decode cache: %v", err)
	}
	if len(cached) != 3 || cached["date"] == nil || cached["projects"] == nil || cached["clients"] == nil {
		t.Fatalf("unexpected cache shape: %s", data)
	}

	today = today.AddDate(0, 0, 1)
	if _, _, err := client.GetTargets(42); err != nil {
		t.Fatalf("next-day GetTargets failed: %v", err)
	}
	if projectCalls != 2 || clientCalls != 2 || timeEntryCalls != 2 {
		t.Fatalf("next-day calls: projects=%d, clients=%d, time entries=%d; want 2 each", projectCalls, clientCalls, timeEntryCalls)
	}
}

func TestProjectsByLastUsedSortsUnusedProjectsByName(t *testing.T) {
	projects := []Project{
		{ID: 1, Name: "Zulu"},
		{ID: 2, Name: "Used"},
		{ID: 3, Name: "alpha"},
	}
	entries := []TimeEntry{
		{ProjectID: testInt64Pointer(2), Start: time.Date(2026, time.September, 1, 9, 0, 0, 0, time.UTC)},
	}

	ordered := projectsByLastUsed(projects, entries)
	want := []int64{2, 3, 1}
	for i, id := range want {
		if ordered[i].ID != id {
			t.Fatalf("position %d: got project %d, want %d", i, ordered[i].ID, id)
		}
	}
	if projects[0].ID != 1 {
		t.Fatal("projectsByLastUsed modified its input")
	}
}

func TestGetTargetsWithTimeEntriesReusesProvidedEntries(t *testing.T) {
	var timeEntryCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/workspaces/42/projects":
			fmt.Fprint(w, `[{"id":1,"name":"Old"},{"id":2,"name":"Recent"}]`)
		case "/workspaces/42/clients":
			fmt.Fprint(w, `[]`)
		case "/me/time_entries":
			timeEntryCalls++
			fmt.Fprint(w, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient("token")
	client.baseURL = server.URL
	client.cachePath = filepath.Join(t.TempDir(), "cache.json")
	client.now = func() time.Time {
		return time.Date(2026, time.September, 15, 12, 0, 0, 0, time.UTC)
	}
	entries := []TimeEntry{
		{ProjectID: testInt64Pointer(2), Start: time.Date(2026, time.September, 14, 9, 0, 0, 0, time.UTC)},
	}

	projects, _, err := client.GetTargetsWithTimeEntries(42, entries)
	if err != nil {
		t.Fatalf("GetTargetsWithTimeEntries failed: %v", err)
	}
	if timeEntryCalls != 0 {
		t.Fatalf("time-entry endpoint called %d times, want 0", timeEntryCalls)
	}
	if len(projects) != 2 || projects[0].ID != 2 {
		t.Fatalf("unexpected project order: %#v", projects)
	}
}

func testInt64Pointer(value int64) *int64 {
	return &value
}
