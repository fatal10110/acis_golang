package summon

import (
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable/attackabletest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// fakeSummonOwner is a minimal owner for SpawnBesideOwner fixtures.
type fakeSummonOwner struct {
	attackabletest.Combatant
	world.Presence

	id int32
}

func (o *fakeSummonOwner) ObjectID() int32           { return o.id }
func (*fakeSummonOwner) Kind() actor.Kind            { return actor.KindPlayer }
func (o *fakeSummonOwner) LevelValue() int           { return 1 }
func (o *fakeSummonOwner) Position() (int, int, int) { return 1000, 1000, 0 }
func (o *fakeSummonOwner) InCombat() bool            { return false }

func newBroadcastFixture(t *testing.T) (*Actor, *event.Recorder) {
	t.Helper()
	state := world.New()
	actor := mustServitor(t, ServitorConfig{ObjectID: 7})
	rec := &event.Recorder{}
	actor.Attach(Runtime{Sink: rec})
	SpawnBesideOwner(state, actor, &fakeSummonOwner{id: 1}, location.Location{})
	return actor, rec
}

func TestSummonBroadcastEmitsTypedEvents(t *testing.T) {
	actor, rec := newBroadcastFixture(t)

	move := event.Move{Origin: location.Location{X: 1}, Destination: location.Location{X: 2}}
	if err := actor.BroadcastMove(move); err != nil {
		t.Fatalf("BroadcastMove() error = %v", err)
	}
	if err := actor.BroadcastStop(); err != nil {
		t.Fatalf("BroadcastStop() error = %v", err)
	}
	if err := actor.BroadcastSelfSkillUse(1422, 1); err != nil {
		t.Fatalf("BroadcastSelfSkillUse() error = %v", err)
	}
	if err := actor.BroadcastAttack(event.Attack{AttackerID: 7}); err != nil {
		t.Fatalf("BroadcastAttack() error = %v", err)
	}

	x, y, z := actor.Position()
	at := location.Location{X: x, Y: y, Z: z}
	want := []event.Event{
		move,
		event.Stopped{},
		event.MagicSkillUse{CasterID: 7, CasterAt: at, TargetID: 7, TargetAt: at, SkillID: 1422, Level: 1},
		event.Attack{AttackerID: 7},
	}
	if got := rec.Events(); !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %+v, want %+v", got, want)
	}
}

func TestSummonBroadcastWithoutSinkIsSilentNoOp(t *testing.T) {
	state := world.New()
	actor := mustServitor(t, ServitorConfig{ObjectID: 7})
	SpawnBesideOwner(state, actor, &fakeSummonOwner{id: 1}, location.Location{})

	if err := actor.BroadcastStop(); err != nil {
		t.Fatalf("BroadcastStop() with no sink error = %v, want nil", err)
	}
	if err := actor.BroadcastSelfSkillUse(1422, 1); err != nil {
		t.Fatalf("BroadcastSelfSkillUse() with no sink error = %v, want nil", err)
	}
}

func TestSummonBroadcastMoveToPawnEmitsDistanceFromOrigin(t *testing.T) {
	actor, rec := newBroadcastFixture(t)

	target := mustServitor(t, ServitorConfig{ObjectID: 9})
	actor.world.Spawn(target, 1020, 1000, 0, 0)

	if err := actor.BroadcastMoveToPawn(target); err != nil {
		t.Fatalf("BroadcastMoveToPawn() error = %v", err)
	}
	x, y, z := actor.Position()
	origin := location.Location{X: x, Y: y, Z: z}
	want := event.MoveToPawn{TargetID: 9, Distance: int(origin.Distance3D(location.Location{X: 1020, Y: 1000})), Origin: origin}
	if got := event.Of[event.MoveToPawn](rec); len(got) != 1 || got[0] != want {
		t.Fatalf("MoveToPawn events = %+v, want [%+v]", got, want)
	}
}

func TestSummonAbnormalEffectReportedOnlyAfterOwnerDiscovery(t *testing.T) {
	actor, rec := newBroadcastFixture(t)

	actor.UpdateAbnormalEffect()
	if got := event.Count[event.AbnormalEffectChanged](rec); got != 0 {
		t.Fatalf("abnormal effect events before owner discovery = %d, want 0", got)
	}
	actor.MarkDiscoveredByOwner()
	actor.UpdateAbnormalEffect()
	if got := event.Count[event.AbnormalEffectChanged](rec); got != 1 {
		t.Fatalf("abnormal effect events after owner discovery = %d, want 1", got)
	}
}

func TestOwnerStillLinkedReflectsActiveSummonRegistration(t *testing.T) {
	state := world.New()
	owner := &fakeSummonOwner{id: 42}
	actor := mustServitor(t, ServitorConfig{ObjectID: 7, Owner: owner})
	SpawnBesideOwner(state, actor, owner, location.Location{})

	if !actor.OwnerStillLinked() {
		t.Fatal("OwnerStillLinked() = false with active registration, want true")
	}

	state.RemoveSummon(owner.ObjectID())
	if actor.OwnerStillLinked() {
		t.Fatal("OwnerStillLinked() = true after owner cleared summon, want false")
	}
}
