package toggl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type targetCache struct {
	Date     string            `json:"date"`
	Projects []Project         `json:"projects"`
	Clients  []WorkspaceClient `json:"clients"`
}

func projectsByLastUsed(projects []Project, entries []TimeEntry) []Project {
	lastUsed := make(map[int64]time.Time)
	for _, entry := range entries {
		if entry.ProjectID == nil {
			continue
		}
		if previous, ok := lastUsed[*entry.ProjectID]; !ok || entry.Start.After(previous) {
			lastUsed[*entry.ProjectID] = entry.Start
		}
	}

	ordered := append([]Project(nil), projects...)
	sort.SliceStable(ordered, func(i, j int) bool {
		iUsed, iOK := lastUsed[ordered[i].ID]
		jUsed, jOK := lastUsed[ordered[j].ID]
		if iOK != jOK {
			return iOK
		}
		if iOK && !iUsed.Equal(jUsed) {
			return iUsed.After(jUsed)
		}
		return strings.ToLower(ordered[i].Name) < strings.ToLower(ordered[j].Name)
	})
	return ordered
}

func (c *Client) readTargetCache() (targetCache, bool) {
	if c.cachePath == "" {
		return targetCache{}, false
	}

	data, err := os.ReadFile(c.cachePath)
	if err != nil {
		return targetCache{}, false
	}

	var cache targetCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return targetCache{}, false
	}
	if cache.Date != c.now().Format(time.DateOnly) || cache.Projects == nil || cache.Clients == nil {
		return targetCache{}, false
	}
	return cache, true
}

func (c *Client) writeTargetCache(cache targetCache) error {
	if c.cachePath == "" {
		return nil
	}

	dir := filepath.Dir(c.cachePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".cache-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	encoder := json.NewEncoder(tmp)
	if err := encoder.Encode(cache); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, c.cachePath)
}
