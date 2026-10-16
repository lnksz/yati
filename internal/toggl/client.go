package toggl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const baseURL = "https://api.track.toggl.com/api/v9"

type Client struct {
	token      string
	httpClient *http.Client
	baseURL    string
	cachePath  string
	now        func() time.Time
	ctx        context.Context
}

// WithContext returns a client whose requests use ctx. The original client can
// still be used by another caller; its configuration is not modified.
func (c *Client) WithContext(ctx context.Context) *Client {
	copy := *c
	copy.ctx = ctx
	return &copy
}

// WithCachePath returns a client with an isolated project/client cache.
func (c *Client) WithCachePath(path string) *Client {
	copy := *c
	copy.cachePath = path
	return &copy
}

func NewClient(token string) *Client {
	cachePath := ""
	if cacheDir, err := os.UserCacheDir(); err == nil {
		cachePath = filepath.Join(cacheDir, "yati", "cache.json")
	}
	return &Client{
		token:      token,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseURL:    baseURL,
		cachePath:  cachePath,
		now:        time.Now,
	}
}

func (c *Client) req(method, path string, body interface{}) (*http.Request, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(b)
	}

	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(c.token, "api_token")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return req, nil
}

func (c *Client) do(req *http.Request, v interface{}) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return responseError(resp, body)
	}

	if v != nil {
		err := json.NewDecoder(resp.Body).Decode(v)
		if err != nil && err != io.EOF {
			return err
		}
	}
	return nil
}

func (c *Client) GetMe() (*Me, error) {
	req, err := c.req(http.MethodGet, "/me", nil)
	if err != nil {
		return nil, err
	}

	var me Me
	if err := c.do(req, &me); err != nil {
		return nil, err
	}
	return &me, nil
}

func (c *Client) GetCurrentTimeEntry() (*TimeEntry, error) {
	req, err := c.req(http.MethodGet, "/me/time_entries/current", nil)
	if err != nil {
		return nil, err
	}

	var entry TimeEntry
	if err := c.do(req, &entry); err != nil {
		return nil, err
	}

	// API returns an empty response body if there is no running time entry,
	// which json.Decode handles as an empty struct, but wait, usually Toggl returns null or empty string.
	// Actually, if there is no current time entry, json decode might fail on empty body or return empty TimeEntry.
	// We'll check if ID is 0.
	if entry.ID == 0 {
		return nil, nil
	}
	return &entry, nil
}

func (c *Client) StartTimeEntry(workspaceID int64, projectID *int64, description string) (*TimeEntry, error) {
	reqBody := StartTimeEntryRequest{
		Description: description,
		WorkspaceID: workspaceID,
		ProjectID:   projectID,
		Start:       time.Now().UTC().Format(time.RFC3339),
		Duration:    -1,
		CreatedWith: "yati",
	}

	req, err := c.req(http.MethodPost, fmt.Sprintf("/workspaces/%d/time_entries", workspaceID), reqBody)
	if err != nil {
		return nil, err
	}

	var entry TimeEntry
	if err := c.do(req, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

func (c *Client) StopTimeEntry(workspaceID int64, timeEntryID int64) (*TimeEntry, error) {
	req, err := c.req(http.MethodPatch, fmt.Sprintf("/workspaces/%d/time_entries/%d/stop", workspaceID, timeEntryID), nil)
	if err != nil {
		return nil, err
	}

	var entry TimeEntry
	if err := c.do(req, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

func (c *Client) GetProjects(workspaceID int64) ([]Project, error) {
	projects, _, err := c.GetTargets(workspaceID)
	return projects, err
}

func (c *Client) getProjects(workspaceID int64) ([]Project, error) {
	req, err := c.req(http.MethodGet, fmt.Sprintf("/workspaces/%d/projects", workspaceID), nil)
	if err != nil {
		return nil, err
	}

	var projects []Project
	if err := c.do(req, &projects); err != nil {
		return nil, err
	}
	return projects, nil
}

func (c *Client) getClients(workspaceID int64) ([]WorkspaceClient, error) {
	req, err := c.req(http.MethodGet, fmt.Sprintf("/workspaces/%d/clients", workspaceID), nil)
	if err != nil {
		return nil, err
	}

	var clients []WorkspaceClient
	if err := c.do(req, &clients); err != nil {
		return nil, err
	}
	return clients, nil
}

func (c *Client) GetTargets(workspaceID int64) ([]Project, []WorkspaceClient, error) {
	return c.getTargets(workspaceID, nil, false)
}

func (c *Client) GetTargetsWithTimeEntries(workspaceID int64, entries []TimeEntry) ([]Project, []WorkspaceClient, error) {
	return c.getTargets(workspaceID, entries, true)
}

func (c *Client) getTargets(workspaceID int64, entries []TimeEntry, haveEntries bool) ([]Project, []WorkspaceClient, error) {
	if cache, ok := c.readTargetCache(); ok {
		return cache.Projects, cache.Clients, nil
	}

	projects, err := c.getProjects(workspaceID)
	if err != nil {
		return nil, nil, err
	}
	clients, err := c.getClients(workspaceID)
	if err != nil {
		return nil, nil, err
	}
	now := c.now()
	if !haveEntries {
		entries, err = c.GetTimeEntries(now.AddDate(0, -3, 0), now)
		if err != nil {
			return nil, nil, err
		}
	}
	projects = projectsByLastUsed(projects, entries)

	if projects == nil {
		projects = []Project{}
	}
	if clients == nil {
		clients = []WorkspaceClient{}
	}
	cache := targetCache{
		Date:     now.Format(time.DateOnly),
		Projects: projects,
		Clients:  clients,
	}
	_ = c.writeTargetCache(cache)
	return projects, clients, nil
}

func (c *Client) GetTimeEntries(startDate, endDate time.Time) ([]TimeEntry, error) {
	// Toggl requires start_date and end_date query params
	path := fmt.Sprintf("/me/time_entries?start_date=%s&end_date=%s",
		startDate.UTC().Format(time.RFC3339),
		endDate.UTC().Format(time.RFC3339))

	req, err := c.req(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var entries []TimeEntry
	if err := c.do(req, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}
