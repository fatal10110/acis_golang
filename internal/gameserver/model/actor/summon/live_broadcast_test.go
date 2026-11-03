package summon

import (
	"reflect"
	"testing"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable/attackabletest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
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
	actor.BroadcastMove(move)
	actor.BroadcastStop()
	actor.BroadcastSelfSkillUse(1422, 1)
	actor.BroadcastAttack(event.Attack{AttackerID: 7})

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

	// No sink attached: these must not panic and must reach nobody.
	actor.BroadcastStop()
	actor.BroadcastSelfSkillUse(1422, 1)
}

func TestSummonBroadcastMoveToPawnEmitsDistanceFromOrigin(t *testing.T) {
	actor, rec := newBroadcastFixture(t)

	target := mustServitor(t, ServitorConfig{ObjectID: 9})
	actor.world.Spawn(target, 1020, 1000, 0, 0)

	actor.BroadcastMoveToPawn(target)
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

	state.RemoveSummon(owner.ObjectID(), actor)
	if actor.OwnerStillLinked() {
		t.Fatal("OwnerStillLinked() = true after owner cleared summon, want false")
	}
}

func (*fakeSummonOwner) ServitorVanished()                          {}
func (*fakeSummonOwner) PvPFlagState() task.PvPFlagState            { return task.PvPFlagNone }
func (*fakeSummonOwner) AwardSummonKillKarma(attackable.Combatant)  {}
func (*fakeSummonOwner) SummonDied(_, _ attackable.Combatant)       {}
func (*fakeSummonOwner) OfferSummonRevive()                         {}
func (*fakeSummonOwner) Invul() bool                                { return false }
func (*fakeSummonOwner) HP() float64                                { return 0 }
func (*fakeSummonOwner) MaxHPValue() float64                        { return 0 }
func (*fakeSummonOwner) ClearReviveOffer()                          {}
func (*fakeSummonOwner) ReviveOffer() (bool, bool)                  { return false, false }
func (*fakeSummonOwner) InDuel() bool                               { return false }
func (*fakeSummonOwner) NoteServitorPvPAttack(attackable.Combatant) {}
func (*fakeSummonOwner) NoteServitorPvPSkillTargets([]attackable.Combatant, bool, string) {
}

// The fake owner is in no duel, party, clan or alliance, does not see
// invisible creatures, and leaves every social rule undecided.
func (*fakeSummonOwner) DuelID() int32                        { return 0 }
func (*fakeSummonOwner) DuelTeam() int                        { return 0 }
func (*fakeSummonOwner) OlympiadMode() bool                   { return false }
func (*fakeSummonOwner) SeesInvisible() bool                  { return false }
func (*fakeSummonOwner) IsInParty() bool                      { return false }
func (*fakeSummonOwner) IsInSameParty(skilltarget.Actor) bool { return false }
func (*fakeSummonOwner) IsInSameClan(skilltarget.Actor) bool  { return false }
func (*fakeSummonOwner) IsInSameAlly(skilltarget.Actor) bool  { return false }
func (*fakeSummonOwner) HasClan() bool                        { return false }
func (*fakeSummonOwner) CanCastOnPlayable(skilltarget.Actor, *modelskill.Definition, bool, bool, bool) bool {
	return true
}

func (*fakeSummonOwner) OffensiveCastAllowed(attackable.ArenaMember, skilltarget.Actor, *modelskill.Definition, bool, bool) bool {
	return true
}

func (*fakeSummonOwner) SocialWithoutForce(attackable.ArenaMember, skilltarget.Actor) (allowed, decided bool) {
	return false, false
}
