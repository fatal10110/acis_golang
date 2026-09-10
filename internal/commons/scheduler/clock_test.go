package scheduler

import (
	"reflect"
	"testing"
	"time"
)

func TestManualClockAdvanceRunsDueTimersInDeadlineThenRegistrationOrder(t *testing.T) {
	clock := NewManualClock(time.Unix(100, 0))
	var got []string

	clock.AfterFunc(2*time.Second, func() { got = append(got, "second") })
	clock.AfterFunc(time.Second, func() {
		got = append(got, "first")
		clock.AfterFunc(time.Second, func() { got = append(got, "nested") })
	})
	clock.AfterFunc(time.Second, func() { got = append(got, "same-deadline") })

	clock.Advance(2 * time.Second)
	if want := []string{"first", "same-deadline", "second", "nested"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("timer order = %v, want %v", got, want)
	}
}

func TestManualClockStoppedTimerDoesNotRun(t *testing.T) {
	clock := NewManualClock(time.Unix(100, 0))
	ran := false
	timer := clock.AfterFunc(time.Second, func() { ran = true })
	if !timer.Stop() {
		t.Fatal("Stop() = false, want true before firing")
	}
	clock.Advance(time.Second)
	if ran {
		t.Fatal("stopped timer ran")
	}
}
