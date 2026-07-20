package toggl

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
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
