package task

import (
	"fmt"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

type shadowItemFakeEffects struct {
	mu     sync.Mutex
	events []string
}

func (e *shadowItemFakeEffects) ManaThreshold(actorID int32, inst *item.Instance, secondsLeft int) {
	e.record(fmt.Sprintf("%d threshold %d %d", actorID, inst.ObjectID, secondsLeft))
}

func (e *shadowItemFakeEffects) Expire(actorID int32, inst *item.Instance) {
	e.record(fmt.Sprintf("%d expire %d", actorID, inst.ObjectID))
}

func (e *shadowItemFakeEffects) record(s string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, s)
}

func (e *shadowItemFakeEffects) take() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.events
	e.events = nil
	return out
}

func TestShadowItems_TrackDecaysManaEachTick(t *testing.T) {
	effects := &shadowItemFakeEffects{}
	s, err := NewShadowItems(effects)
	if err != nil {
		t.Fatalf("NewShadowItems() error = %v", err)
	}

	tmpl := &item.Template{Duration: 5} // 300 seconds of mana
	inst := &item.Instance{ObjectID: 1, ManaLeft: tmpl.InitialManaLeft()}

	s.Track(10, inst, tmpl)
	if !s.Tracked(inst) {
		t.Fatalf("Track() should start tracking a shadow item")
	}
	if inst.ManaLeft != 300 {
		t.Fatalf("first Track() must not cost extra mana, ManaLeft = %d, want 300", inst.ManaLeft)
	}

	s.Tick()
	if inst.ManaLeft != 299 {
		t.Errorf("ManaLeft after one tick = %d, want 299", inst.ManaLeft)
	}
}

func TestShadowItems_Track_NonShadowItemIgnored(t *testing.T) {
	effects := &shadowItemFakeEffects{}
	s, _ := NewShadowItems(effects)

	tmpl := &item.Template{Duration: -1}
	inst := &item.Instance{ObjectID: 1, ManaLeft: -1}

	s.Track(10, inst, tmpl)
	if s.Tracked(inst) {
		t.Errorf("Track() should ignore a non-shadow item")
	}
}

func TestShadowItems_Track_ReequipCostsOneMinute(t *testing.T) {
	effects := &shadowItemFakeEffects{}
	s, _ := NewShadowItems(effects)

	tmpl := &item.Template{Duration: 5}
	inst := &item.Instance{ObjectID: 1, ManaLeft: tmpl.InitialManaLeft()}

	s.Track(10, inst, tmpl)
	s.Tick() // let a second elapse so mana drifts off the full-duration value
	s.Untrack(inst)
	if inst.ManaLeft != 299 {
		t.Fatalf("ManaLeft before re-equip = %d, want 299", inst.ManaLeft)
	}

	s.Track(10, inst, tmpl) // re-equip: mana no longer at full duration
	if inst.ManaLeft != 239 {
		t.Errorf("re-equipping after mana has already drifted should cost one extra minute, ManaLeft = %d, want 239", inst.ManaLeft)
	}
}

func TestShadowItems_Track_ReequipFreeWhenManaNeverMoved(t *testing.T) {
	// Equipping then immediately unequipping without a tick in between
	// leaves mana at the template's full duration, so re-equipping costs
	// nothing — the penalty only applies once mana has actually drifted.
	effects := &shadowItemFakeEffects{}
	s, _ := NewShadowItems(effects)

	tmpl := &item.Template{Duration: 5}
	inst := &item.Instance{ObjectID: 1, ManaLeft: tmpl.InitialManaLeft()}

	s.Track(10, inst, tmpl)
	s.Untrack(inst)
	s.Track(10, inst, tmpl)
	if inst.ManaLeft != 300 {
		t.Errorf("ManaLeft = %d, want 300 (no tick elapsed, so re-equip is free)", inst.ManaLeft)
	}
}

func TestShadowItems_Tick_FiresThresholdsAndExpiry(t *testing.T) {
	effects := &shadowItemFakeEffects{}
	s, _ := NewShadowItems(effects)

	tmpl := &item.Template{Duration: 5} // 300 seconds of mana
	inst := &item.Instance{ObjectID: 1, ManaLeft: tmpl.InitialManaLeft()}
	s.Track(10, inst, tmpl)
	effects.take()

	// Fast-forward straight to just above the 1-minute threshold instead
	// of ticking 240 times to get there.
	inst.ManaLeft = 61

	s.Tick()
	if got := effects.take(); len(got) != 1 || got[0] != "10 threshold 1 60" {
		t.Fatalf("Tick() at the 1-minute mark = %v, want [10 threshold 1 60]", got)
	}

	for i := 0; i < 60; i++ {
		s.Tick()
	}
	got := effects.take()
	if len(got) == 0 || got[len(got)-1] != "10 expire 1" {
		t.Fatalf("Tick() at zero mana = %v, want an expiry event", got)
	}
	if s.Tracked(inst) {
		t.Errorf("an expired item should no longer be tracked")
	}
}

func TestShadowItems_Remove_StopsTrackingByActor(t *testing.T) {
	effects := &shadowItemFakeEffects{}
	s, _ := NewShadowItems(effects)

	tmpl := &item.Template{Duration: 5}
	instA := &item.Instance{ObjectID: 1, ManaLeft: tmpl.InitialManaLeft()}
	instB := &item.Instance{ObjectID: 2, ManaLeft: tmpl.InitialManaLeft()}
	s.Track(10, instA, tmpl)
	s.Track(20, instB, tmpl)

	s.Remove(10)
	if s.Tracked(instA) {
		t.Errorf("Remove(10) should stop tracking actor 10's item")
	}
	if !s.Tracked(instB) {
		t.Errorf("Remove(10) must not affect actor 20's item")
	}
}
