package gui

import (
	"testing"
	"time"
)

func TestParseEditStart(t *testing.T) {
	original := time.Date(2026, time.November, 10, 8, 0, 0, 123456789, time.UTC)
	now := original.Add(2 * time.Hour)
	for _, test := range []struct {
		name, input, want string
		invalid           bool
	}{
		{"untouched keeps precision", formatEditStart(original), "", false},
		{"same instant", original.Format(time.RFC3339Nano), "", false},
		{"earlier", "2026-11-10 08:30:00 +01:00", "2026-11-10T07:30:00Z", false},
		{"later", "2026-11-10 09:30 +01:00", "2026-11-10T08:30:00Z", false},
		{"previous day", "2026-11-09 22:30:00 -05:00", "2026-11-10T03:30:00Z", false},
		{"DST first occurrence", "2026-10-25 02:30:00 +02:00", "2026-10-25T00:30:00Z", false},
		{"DST second occurrence", "2026-10-25 02:30:00 +01:00", "2026-10-25T01:30:00Z", false},
		{"empty", "", "", true},
		{"invalid date", "2026-02-30 09:00:00 +01:00", "", true},
		{"missing offset", "2026-11-10 09:00:00", "", true},
		{"invalid offset", "2026-11-10 09:00:00 +99:00", "", true},
		{"future", "2026-11-10 12:00:00 +01:00", "", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseEditStart(test.input, original, now)
			if test.invalid {
				if err == nil {
					t.Fatal("accepted invalid start time")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if test.want == "" {
				if got != nil {
					t.Fatalf("untouched start would be overwritten: %v", got)
				}
			} else if got == nil || got.Format(time.RFC3339Nano) != test.want {
				t.Fatalf("got %v, want %s", got, test.want)
			}
		})
	}
}
