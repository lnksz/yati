package cli

import (
	"testing"
	"time"
)

func TestInteractiveHistoryStart(t *testing.T) {
	endDate := time.Date(2026, time.September, 7, 14, 30, 0, 0, time.FixedZone("test", 2*60*60))
	want := time.Date(2026, time.March, 7, 14, 30, 0, 0, endDate.Location())

	if got := interactiveHistoryStart(endDate); !got.Equal(want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}
