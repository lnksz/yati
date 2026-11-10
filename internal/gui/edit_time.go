package gui

import (
	"errors"
	"strings"
	"time"
)

const editStartLayout = "2006-01-02 15:04:05 -07:00"

func formatEditStart(start time.Time) string {
	return start.In(time.Local).Format(editStartLayout)
}

// An untouched field omits start from the update, preserving subsecond precision
// and any start-time change made elsewhere while the editor was open.
func parseEditStart(text string, original, now time.Time) (*time.Time, error) {
	text = strings.TrimSpace(text)
	if text == formatEditStart(original) {
		return nil, nil
	}
	for _, layout := range []string{editStartLayout, "2006-01-02 15:04 -07:00", time.RFC3339Nano} {
		start, err := time.Parse(layout, text)
		if err != nil || start.IsZero() {
			continue
		}
		if start.Equal(original) {
			return nil, nil
		}
		if start.After(now) {
			return nil, errors.New("Start time cannot be in the future.")
		}
		utc := start.UTC()
		return &utc, nil
	}
	return nil, errors.New("Use YYYY-MM-DD HH:MM:SS with a UTC offset, such as +01:00.")
}
