package importprogress

import (
	"testing"
	"time"
)

// The bar must never claim to be finished before the work is.
//
// Every "stuck at 100%" report the platform has had came from arithmetic
// reaching the end of the last band while a commit was still writing. 100 is
// the terminal state's word, not the calculation's.
func TestPercentNeverReachesHundred(t *testing.T) {
	last := Band{Start: 84, End: 99}
	for _, tc := range []struct {
		name           string
		current, total int
		elapsed        time.Duration
	}{
		{"counted and complete", 9000, 9000, 0},
		{"counted past its total", 12000, 9000, 0},
		{"drifting for a minute", 0, 0, time.Minute},
		{"drifting for an hour", 0, 0, time.Hour},
		{"drifting for a day", 0, 0, 24 * time.Hour},
	} {
		if got := Percent(last, tc.current, tc.total, tc.elapsed); got >= Complete {
			t.Errorf("%s: Percent = %d, want < %d", tc.name, got, Complete)
		}
	}
}

// A stage that cannot count its work still moves, and never stops moving.
//
// This is what the AI adjudication pass needs: it waits on a network for a
// minute or more with no row counter, and a bar that freezes there is read as a
// hung import.
func TestDriftIsMonotonicAndBounded(t *testing.T) {
	prev := -1.0
	for _, d := range []time.Duration{
		0, time.Second, 5 * time.Second, 12 * time.Second,
		30 * time.Second, time.Minute, 5 * time.Minute, time.Hour,
	} {
		got := Drift(d)
		if got < prev {
			t.Fatalf("Drift(%v) = %v went backwards from %v", d, got, prev)
		}
		if got >= 1 {
			t.Fatalf("Drift(%v) = %v, want strictly below 1", d, got)
		}
		prev = got
	}
	// And it must actually move within the time somebody is watching.
	if Drift(5*time.Second) <= 0.1 {
		t.Errorf("Drift(5s) = %v; the bar would look frozen", Drift(5*time.Second))
	}
}

// Within a band, a counted stage interpolates across that band and nowhere else.
func TestCountedStageStaysInItsBand(t *testing.T) {
	b := Band{Start: 46, End: 84}
	if got := Percent(b, 0, 100, 0); got != 46 {
		t.Errorf("start of band = %d, want 46", got)
	}
	if got := Percent(b, 50, 100, 0); got != 65 {
		t.Errorf("halfway = %d, want 65", got)
	}
	if got := Percent(b, 100, 100, 0); got != 84 {
		t.Errorf("end of band = %d, want 84", got)
	}
}

// A stage that has not started reports its band's floor, not zero: the earlier
// stages really did happen and the bar must not rewind across a boundary.
func TestBandFloorIsHeld(t *testing.T) {
	if got := Percent(Band{Start: 46, End: 84}, 0, 0, 0); got != 46 {
		t.Errorf("Percent at stage entry = %d, want the band floor 46", got)
	}
}

// An unknown or malformed band degrades to its start rather than to zero, so a
// mis-declared stage cannot make the bar jump backwards.
func TestInvalidBandDoesNotRewind(t *testing.T) {
	if got := Percent(Band{Start: 60, End: 40}, 5, 10, 0); got != 60 {
		t.Errorf("Percent on an inverted band = %d, want its start 60", got)
	}
	if got := Percent(Band{}, 5, 10, 0); got != 0 {
		t.Errorf("Percent on a zero band = %d, want 0", got)
	}
}
