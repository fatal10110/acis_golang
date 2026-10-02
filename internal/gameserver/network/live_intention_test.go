package network

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/rs/zerolog"
)

// intentionCastDef is a plain single-target magic cast long enough to stay
// in flight for the whole test: nothing schedules its launch.
var intentionCastDef = modelskill.Definition{ID: 1177, Level: 1, Magic: true, HitTime: 4000, CastRange: 600}

// intentionFixture is a player with a cast controller installed the way
// attach installs one, so a cast in flight is the one CastingNow sees, in a
// world where it knows the creatures its intentions act on. The cast's events
// go nowhere: no link broadcasts its end.
type intentionFixture struct {
	link  *GameClientLink
	live  *livePlayer
	world *world.State
}

func newIntentionFixture(t *testing.T) intentionFixture {
	t.Helper()
	live := newTestLivePlayer(t, 1, &testsupport.FrameCapture{})
	live.cast = actorcast.NewController(actorcast.PlayerActor{Character: live.Character, Attack: live.attack}, nil)
	live.cast.SetQueue(live.Queue())
	live.Character.SetCastController(live.cast)
	f := intentionFixture{link: &GameClientLink{log: zerolog.Nop()}, live: live, world: world.New()}
	f.spawn(t, live)
	return f
}

func (f intentionFixture) spawn(t *testing.T, obj attackable.Combatant) {
	t.Helper()
	tracked, ok := obj.(world.Tracked)
	if !ok {
		t.Fatalf("%d is not a world object", obj.ObjectID())
	}
	x, y, z := obj.Position()
	f.world.Spawn(tracked, x, y, z, 0)
	if obj.ObjectID() != f.live.ObjectID() && !f.live.Knows(obj) {
		t.Fatalf("fixture player does not know %d", obj.ObjectID())
	}
}

// player is another player f's player knows.
func (f intentionFixture) player(t *testing.T, id int32) *livePlayer {
	t.Helper()
	other := newTestLivePlayer(t, id, &testsupport.FrameCapture{})
	f.spawn(t, other)
	return other
}

// npc is a hostile NPC f's player knows, so an attack on it is held rather
// than dropped as lost.
func (f intentionFixture) npc(t *testing.T) *npc.Hostile {
	t.Helper()
	hostile := newTestHostileNPC(t, 50)
	f.spawn(t, hostile)
	return hostile
}

func requireIntentionFinalTarget(t *testing.T, live *livePlayer, want int32, state string) {
	t.Helper()
	if got := live.IntentionFinalTarget(); got != want {
		t.Fatalf("%s: IntentionFinalTarget() = %d, want %d", state, got, want)
	}
}

// TestIntentionFinalTargetIdle: a player whose intention acts on nothing,
// and one whose cast ended, report no final target.
func TestIntentionFinalTargetIdle(t *testing.T) {
	f := newIntentionFixture(t)
	live := f.live
	requireIntentionFinalTarget(t, live, 0, "idle")

	target := f.player(t, 9)
	if _, err := live.cast.Start(time.Unix(1000, 0), target.Character, intentionCastDef); err != nil {
		t.Fatalf("cast Start: %v", err)
	}
	live.cast.Stop()
	requireIntentionFinalTarget(t, live, 0, "after the cast stopped")
}

// TestIntentionFinalTargetCastInFlightBeatsDeferredAttack: an attack
// requested mid-cast waits for the cast, so the CAST intention, and its
// target, stays current until the cast ends; the attack target then is.
func TestIntentionFinalTargetCastInFlightBeatsDeferredAttack(t *testing.T) {
	f := newIntentionFixture(t)
	live := f.live
	target := f.player(t, 9)
	npc := f.npc(t)

	if _, err := live.cast.Start(time.Unix(1000, 0), target.Character, intentionCastDef); err != nil {
		t.Fatalf("cast Start: %v", err)
	}
	if live.combat.Start(npc, false) {
		t.Fatal("attack requested mid-cast was thought at once, want it deferred behind the cast")
	}
	if got := live.combat.Target(); got == nil || got.ObjectID() != npc.ObjectID() {
		t.Fatalf("deferred attack target = %v, want npc %d held", got, npc.ObjectID())
	}
	requireIntentionFinalTarget(t, live, target.ObjectID(), "cast in flight with an attack deferred behind it")

	live.cast.Stop()
	requireIntentionFinalTarget(t, live, npc.ObjectID(), "after the cast ended")
}

// TestIntentionFinalTargetCastApproach: a cast approach walking to its
// final target is the current CAST intention and reports that target, ahead
// of an attack target still held; a cast only queued as the next intention
// is not current, so the attack target stays the final target.
func TestIntentionFinalTargetCastApproach(t *testing.T) {
	const finalID = 77
	req := clientpackets.RequestMagicSkillUse{SkillID: int32(intentionCastDef.ID), CtrlPressed: true}
	itemSkill := modelskill.Definition{ID: 2039, Level: 1, Magic: true}
	for _, tc := range []struct {
		name string
		park func(live *livePlayer)
		want func(npcID int32) int32
	}{
		{"magic skill approach", func(live *livePlayer) {
			live.approachMagicSkill(req, live.Character, finalID)
		}, func(int32) int32 { return finalID }},
		{"item cast approach", func(live *livePlayer) {
			live.approachItemAICast(nil, nil, itemSkill, live.Character, true, finalID)
		}, func(int32) int32 { return finalID }},
		{"magic skill queued next", func(live *livePlayer) {
			live.deferMagicSkill(req, live.Character)
		}, func(npcID int32) int32 { return npcID }},
		{"item cast queued next", func(live *livePlayer) {
			live.deferItemAICast(nil, nil, itemSkill, live.Character, true)
		}, func(npcID int32) int32 { return npcID }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newIntentionFixture(t)
			live := f.live
			tc.park(live)
			alone := tc.want(0)
			requireIntentionFinalTarget(t, live, alone, "parked alone")

			npc := f.npc(t)
			live.combat.Start(npc, false)
			if got := live.combat.Target(); got == nil || got.ObjectID() != npc.ObjectID() {
				t.Fatalf("attack target = %v, want npc %d", got, npc.ObjectID())
			}
			tc.park(live)
			requireIntentionFinalTarget(t, live, tc.want(npc.ObjectID()), "parked beside an attack target")

			live.clearParkedApproaches()
			requireIntentionFinalTarget(t, live, npc.ObjectID(), "after the approach was cleared")
		})
	}
}

// TestIntentionFinalTargetAttackThenFollow: an attack reports its target;
// following another player replaces the attack, and the followed player is
// then the final target.
func TestIntentionFinalTargetAttackThenFollow(t *testing.T) {
	f := newIntentionFixture(t)
	live := f.live
	npc := f.npc(t)
	friend := f.player(t, 9)

	live.combat.Start(npc, false)
	requireIntentionFinalTarget(t, live, npc.ObjectID(), "attacking")

	f.link.startLiveFollow(live, friend.Character, false)
	if got := live.move.FriendlyFollowTarget(); got == nil || got.ObjectID() != friend.ObjectID() {
		t.Fatalf("friendly follow target = %v, want %d", got, friend.ObjectID())
	}
	if live.combat.Target() != nil {
		t.Fatal("follow left the attack target held, want it replaced")
	}
	requireIntentionFinalTarget(t, live, friend.ObjectID(), "following")

	live.move.CancelFriendlyFollow()
	requireIntentionFinalTarget(t, live, 0, "after the follow ended")
}
