package task

import (
	"testing"
	"time"
)

type doorRegenRecorder struct {
	r    *DoorRegen
	dues []time.Time
}

func (e *doorRegenRecorder) RegenDoor(id int, due time.Time) {
	e.dues = append(e.dues, due)
	if !e.r.Tracked(id) || !e.r.Due(id, due) {
		panic("in-flight tick no longer scheduled")
	}
	e.r.Begin(id) // a hit while the tick runs keeps the pending tick
	if !e.r.Due(id, due) {
		panic("Begin displaced the in-flight tick")
	}
	e.r.Next(id, due)
}

func TestDoorRegenTickKeepsEntryWhileInFlight(t *testing.T) {
	now := time.UnixMilli(0)
	r := NewDoorRegen(func() time.Time { return now })
	r.Begin(7)
	first := now.Add(DoorRegenPeriod)

	effects := &doorRegenRecorder{r: r}
	now = first.Add(-time.Second)
	r.Tick(effects)
	if len(effects.dues) != 0 {
		t.Fatalf("tick ran early: %v", effects.dues)
	}
	now = first.Add(time.Second)
	r.Tick(effects)
	if len(effects.dues) != 1 || !effects.dues[0].Equal(first) {
		t.Fatalf("ran dues %v, want [%v]", effects.dues, first)
	}
	if !r.Due(7, first.Add(DoorRegenPeriod)) {
		t.Fatal("Next did not keep the fixed rate")
	}
}
