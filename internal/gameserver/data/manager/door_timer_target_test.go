package manager

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
)

// doorChangeLog is a task.DoorEffects that records each due change's
// target and applies nothing.
type doorChangeLog struct{ targets []bool }

func (l *doorChangeLog) SetDoorOpen(_ int, open bool) bool {
	l.targets = append(l.targets, open)
	return false
}

// An open whose watcher closes the same door before the open's timer is
// scheduled still schedules the auto-close, closeTime later: the timer
// follows the change that was asked for, not the state the door is left in.
func TestDoorOpenTimerTargetsCloseWhenWatcherFlipsDoor(t *testing.T) {
	const id = 19210001
	now := time.UnixMilli(0)
	changes := &doorChangeLog{}
	objects, _, _, _, doorTimers := newTestWorldObjectsWith(t, &door.Template{
		ID: id, Name: "gate", Kind: door.KindDoor, Level: 1,
		OpenTime: 30, CloseTime: 60,
	}, changes, func() time.Time { return now })

	flipped := false
	if !objects.WatchDoor(id, door.WatchScript, "flip", func(obj *door.Object) {
		if flipped || !obj.Opened() {
			return
		}
		flipped = true
		if !objects.SetDoorOpen(id, false) {
			t.Error("watcher close = false, want a state change")
		}
	}) {
		t.Fatal("WatchDoor = false")
	}
	if !objects.SetDoorOpen(id, true) {
		t.Fatal("SetDoorOpen(open) = false, want a state change")
	}
	if !flipped {
		t.Fatal("watcher did not close the door")
	}

	// Pending: the spawn's auto-open and the watcher close's auto-open
	// (30s each), then the open's auto-close (60s).
	now = now.Add(59 * time.Second)
	doorTimers.Tick()
	if want := []bool{true, true}; !slices.Equal(changes.targets, want) {
		t.Fatalf("changes due by 59s = %v, want %v", changes.targets, want)
	}
	now = now.Add(time.Second)
	doorTimers.Tick()
	if want := []bool{true, true, false}; !slices.Equal(changes.targets, want) {
		t.Fatalf("changes due by 60s = %v, want %v", changes.targets, want)
	}
}
