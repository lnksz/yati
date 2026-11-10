package toggl

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestUpdateTimeEntryOnlyUpdatesExistingEntryFields(t *testing.T) {
	project := int64(7)
	start := time.Date(2026, time.November, 10, 9, 30, 0, 0, time.FixedZone("CET", 3600))
	for _, edit := range []struct {
		name, title string
		project     *int64
		start       *time.Time
	}{
		{"assign and rename", "Renamed <title> & Überprüfen", &project, nil},
		{"clear project", "Keep title", nil, nil},
		{"clear title", "", &project, nil},
		{"change start", "Edited start", &project, &start},
	} {
		t.Run(edit.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method != http.MethodPut || r.URL.Path != "/workspaces/42/time_entries/123" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				var body map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				want := "null"
				if edit.project != nil {
					want = "7"
				}
				var title string
				if err := json.Unmarshal(body["description"], &title); err != nil {
					t.Error(err)
				}
				fields := 3
				if edit.start != nil {
					fields++
					var got string
					if err := json.Unmarshal(body["start"], &got); err != nil || got != edit.start.UTC().Format(time.RFC3339Nano) {
						t.Errorf("start was not sent in UTC: %s (%v)", body["start"], err)
					}
				} else if _, present := body["start"]; present {
					t.Error("untouched start must be omitted")
				}
				if len(body) != fields || string(body["workspace_id"]) != "42" || string(body["project_id"]) != want || title != edit.title {
					t.Errorf("entry update body: %s", body)
				}
				if err := json.NewEncoder(w).Encode(TimeEntry{ID: 123, WorkspaceID: 42, ProjectID: edit.project, Description: edit.title, Duration: -1}); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			client := NewClient("test-token")
			client.baseURL = server.URL
			entry, err := client.UpdateTimeEntry(42, 123, edit.project, edit.title, edit.start)
			if err != nil {
				t.Fatal(err)
			}
			if requests != 1 || entry.ID != 123 || entry.WorkspaceID != 42 || entry.Duration != -1 || entry.Description != edit.title ||
				(edit.project == nil) != (entry.ProjectID == nil) || edit.project != nil && *entry.ProjectID != *edit.project {
				t.Fatalf("unexpected updated entry: %+v (%d requests)", entry, requests)
			}
		})
	}
}
