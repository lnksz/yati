package toggl

import "time"

type TimeEntry struct {
	ID          int64      `json:"id"`
	WorkspaceID int64      `json:"workspace_id"`
	ProjectID   *int64     `json:"project_id"`
	Description string     `json:"description"`
	Start       time.Time  `json:"start"`
	Stop        *time.Time `json:"stop,omitempty"`
	Duration    int64      `json:"duration"`
	Tags        []string   `json:"tags,omitempty"`
}

type Project struct {
	ID          int64  `json:"id"`
	WorkspaceID int64  `json:"workspace_id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Active      bool   `json:"active"`
}

type Workspace struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type Me struct {
	ID               int64       `json:"id"`
	DefaultWorkspace int64       `json:"default_workspace_id"`
	Email            string      `json:"email"`
	Fullname         string      `json:"fullname"`
	Workspaces       []Workspace `json:"workspaces"`
}

type StartTimeEntryRequest struct {
	Description string `json:"description"`
	WorkspaceID int64  `json:"workspace_id"`
	ProjectID   *int64 `json:"project_id,omitempty"`
	Start       string `json:"start"`    // ISO 8601 date and time
	Duration    int64  `json:"duration"` // -1 for running
	CreatedWith string `json:"created_with"`
}
