package clock

import (
	"testing"
	"time"
)

func TestNowMock(t *testing.T) {
	fixed := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	reset := Mock(fixed)
	if got := Now(); !got.Equal(fixed) {
		t.Fatalf("want %v, got %v", fixed, got)
	}
	before := time.Now()
	reset()
	if Now().Before(before) {
		t.Fatalf("expected real clock after reset")
	}
}
