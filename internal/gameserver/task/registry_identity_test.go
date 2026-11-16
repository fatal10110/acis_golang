package task

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// A removal names the actor, not its object id: a second actor tracked under
// the same id, such as the session that won a character's registration, is
// left tracked by the other's removal.

func TestAutosaveRemoveLeavesAnotherActorWithTheSameID(t *testing.T) {
	a, _ := NewAutosave(&autosaveFakeEffects{}, nil)
	winner, loser := &autosaveFakeActor{id: 1}, &autosaveFakeActor{id: 1}
	a.Add(winner)
	a.Remove(loser)
	a.mu.Lock()
	e, ok := a.entries[1]
	a.mu.Unlock()
	if !ok || e.actor != winner {
		t.Fatal("removing another actor with the same id stopped the tracked actor's autosave")
	}
	a.Remove(winner)
	a.mu.Lock()
	_, ok = a.entries[1]
	a.mu.Unlock()
	if ok {
		t.Fatal("removing the tracked actor left its autosave")
	}
}

type clockFakeActor struct{ id int32 }

func (a *clockFakeActor) ObjectID() int32                                        { return a.id }
func (a *clockFakeActor) Queue() *sim.Queue                                      { return nil }
func (a *clockFakeActor) HasSkill(int) bool                                      { return false }
func (a *clockFakeActor) SetSkillLevel(int, int)                                 {}
func (clockFakeEffects) NotifyPlayingTooLong(int32)                              {}
func (clockFakeEffects) NotifyDayNightSkillTransition(int32, bool, int32, int32) {}

type clockFakeEffects struct{}

func TestPlayerClockRemoveLeavesAnotherActorWithTheSameID(t *testing.T) {
	p, err := NewPlayerClock(NewGameClock(func() time.Time { return time.UnixMilli(0) }), nil, clockFakeEffects{})
	if err != nil {
		t.Fatalf("NewPlayerClock: %v", err)
	}
	winner, loser := &clockFakeActor{id: 1}, &clockFakeActor{id: 1}
	p.Add(winner)
	p.Remove(loser)
	if !p.tracked(1) {
		t.Fatal("removing another actor with the same id stopped the tracked actor's reminder")
	}
	p.Remove(winner)
	if p.tracked(1) {
		t.Fatal("removing the tracked actor left its reminder")
	}
}

type shadowFakeOwner struct{ id int32 }

func (o *shadowFakeOwner) ObjectID() int32 { return o.id }

func TestShadowItemsRemoveLeavesAnotherOwnerWithTheSameID(t *testing.T) {
	s, _ := NewShadowItems(&shadowItemFakeEffects{})
	tmpl := &item.Template{Duration: 5}
	inst := &item.Instance{ObjectID: 7, ManaLeft: tmpl.InitialManaLeft()}
	winner, loser := &shadowFakeOwner{id: 1}, &shadowFakeOwner{id: 1}
	s.Track(winner, inst, tmpl)
	s.Remove(loser)
	if !s.Tracked(inst) {
		t.Fatal("removing another owner with the same id stopped the tracked owner's item")
	}
	s.Remove(winner)
	if s.Tracked(inst) {
		t.Fatal("removing the owner left its item tracked")
	}
}
