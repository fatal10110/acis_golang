package pets

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// summonFollowTaskInterval covers one run of a summon's 500 ms offensive
// follow task.
const summonFollowTaskInterval = 600 * time.Millisecond

// requireFollowsOwnerAgain checks the pet is back to following its owner,
// then walks the owner away and requires the pet to catch up.
func (h *petWorld) requireFollowsOwnerAgain(t *testing.T, petActor *summon.Actor, what string) {
	t.Helper()
	if !petActor.FollowActive() {
		t.Fatalf("%s: follow mode off, want on", what)
	}
	if got := petActor.Intent(); got != summon.IntentFollowOwner {
		t.Fatalf("%s: pet intent = %v, want follow-owner", what, got)
	}
	h.ownerWalksAway(t)
	h.requirePetCatchesUp(t, petActor, what)
}

// toggleFollow presses the pet's follow toggle shortcut.
func (h *petWorld) toggleFollow(t *testing.T) {
	t.Helper()
	h.client.Send(encodeRequestActionUse(petFollowToggleAction, false))
	h.handled(t)
}

// attackSelected selects targetID at (x,y,z) and commands the pet to attack it.
func (h *petWorld) attackSelected(t *testing.T, targetID, x, y, z int32) {
	t.Helper()
	h.client.Send(encodeAction(targetID, x, y, z, false))
	drainFrames(t, h.client)
	h.client.Send(encodeRequestActionUse(petAttackAction, false))
	h.handled(t)
}

// TestPetKillsTargetThenFollowsOwner has a following pet's target die while
// the pet swings at it. Once the swing is over the pet cannot attack the
// corpse and goes idle, and an idle pet that follows its owner walks after
// it again instead of standing by the corpse.
func TestPetKillsTargetThenFollowsOwner(t *testing.T) {
	t.Parallel()
	h, petActor := bootFollowingPet(t)
	hostile := spawnTankyHostile(t, h)
	sendPetMidSwing(t, h, petActor, hostile)

	if !hostile.Queue().Post(func() { hostile.Kill(petActor) }) {
		t.Fatal("post kill: queue closed")
	}
	h.srv.AdvanceUntil(t, "the monster dying", hostile.Dead)
	h.srv.AdvanceUntil(t, "the swing ending", func() bool { return !petActor.IsAttackingNow() })
	h.think(t)

	h.requireFollowsOwnerAgain(t, petActor, "pet after its target died")
	drainUntilQuiet(t, h.client)
}

// TestPetRefusedAttackFollowsOwner commands a following pet at a monster it
// cannot see. The refused attack sends it idle, and an idle pet that follows
// its owner walks after it again.
func TestPetRefusedAttackFollowsOwner(t *testing.T) {
	t.Parallel()
	geo := &sightGeo{}
	geo.blind.Store(true)
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithAITask(), gameservertest.WithGeo(geo)})
	petActor, _ := h.spawnWolf(t)
	hostile := h.srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, h.client)

	h.attackSelected(t, hostile.ObjectID(), hostileX, hostileY, hostileZ)
	h.think(t)
	h.think(t)
	if petActor.IsAttackingNow() {
		t.Fatal("pet swung at a monster it cannot see")
	}
	geo.blind.Store(false)

	h.requireFollowsOwnerAgain(t, petActor, "pet after its refused attack")
	drainUntilQuiet(t, h.client)
}

// TestPetRefusedStrikeFollowsOwner commands the pet's strike at an unflagged
// player without CTRL. The refused cast sends it idle, and an idle pet that
// follows its owner walks after it again.
func TestPetRefusedStrikeFollowsOwner(t *testing.T) {
	t.Parallel()
	h, petActor, bystanderID := bootWolfStrikerWithBystander(t, wolfStrike())
	h.targetPlayer(t, bystanderID)

	h.client.Send(encodeRequestActionUse(wolfStrikeAction, false))
	frames := readUntilOpcode(t, h.client, serverpackets.OpcodeActionFailed, "strike ActionFailed")
	requireSummonStrikeRefused(t, frames, petActor, bystanderID)

	h.requireFollowsOwnerAgain(t, petActor, "pet after its refused strike")
	drainUntilQuiet(t, h.client)
}

// TestPetTargetDeletedAfterHitFollowsOwner has a pet that was told to stop
// following swing at a monster that leaves the world once the hit has
// landed. When the swing ends the pet finds its target lost: that turns
// following back on, and the pet walks after its owner. (A target gone
// before the hit stops the swing instead, which leaves follow mode alone.)
func TestPetTargetDeletedAfterHitFollowsOwner(t *testing.T) {
	t.Parallel()
	h, petActor := bootFollowingPet(t)
	h.toggleFollow(t)
	if petActor.FollowActive() {
		t.Fatal("follow toggle left follow mode on")
	}
	hostile := spawnTankyHostile(t, h)
	sendPetMidSwing(t, h, petActor, hostile)

	h.srv.AdvanceUntil(t, "a hit landing", func() bool { return hostile.HP() < float64(hostile.MaxHP()) })
	if !petActor.IsAttackingNow() {
		t.Fatal("swing over as its hit landed, want it still running")
	}
	hostile.DeleteMe()
	h.srv.AdvanceUntil(t, "the swing ending", func() bool { return !petActor.IsAttackingNow() })
	h.think(t)

	h.requireFollowsOwnerAgain(t, petActor, "pet whose target left after the hit")
	drainUntilQuiet(t, h.client)
}

// TestPetTargetDeletedWhileChasingDropsFollow sends a following pet at a far
// monster and takes the monster out of the world while the pet runs at it.
// The chase's follow task finds the target gone: it turns following off and
// stops the pet where it is, so the owner's next follow toggle turns
// following back on.
func TestPetTargetDeletedWhileChasingDropsFollow(t *testing.T) {
	t.Parallel()
	h, petActor := bootFollowingPet(t)
	hostile := h.srv.SpawnHostileNPCAt(t, location.Location{X: 600, Y: 20, Z: 30})
	drainUntilQuiet(t, h.client)

	h.attackSelected(t, hostile.ObjectID(), 600, 20, 30)
	if !petActor.Move().Moving() {
		t.Fatal("pet is not running at the far monster")
	}

	hostile.DeleteMe()
	h.srv.Advance(t, summonFollowTaskInterval)
	if petActor.FollowActive() {
		t.Fatal("follow mode still on after the chased target left, want off")
	}
	if got := petActor.Intent(); got != summon.IntentIdle {
		t.Fatalf("pet intent after the chased target left = %v, want idle", got)
	}
	if petActor.Move().Moving() {
		t.Fatal("pet still moving after the chased target left")
	}

	h.toggleFollow(t)
	h.requireFollowsOwnerAgain(t, petActor, "pet told to follow after losing its chase")
	drainUntilQuiet(t, h.client)
}

// TestPetQueuedStrikeOnDeletedTargetFollowsOwner queues the pet's strike
// behind its swing and takes the target out of the world before the swing
// ends. The queued strike then finds its target lost and the pet goes back
// to following its owner instead of keeping the attack.
func TestPetQueuedStrikeOnDeletedTargetFollowsOwner(t *testing.T) {
	t.Parallel()
	h, petActor, _ := bootWolfStrikerWith(t, wolfStrike(), gameservertest.WithAITask())
	hostile := spawnTankyHostile(t, h)
	sendPetMidSwing(t, h, petActor, hostile)

	h.client.Send(encodeRequestActionUse(wolfStrikeAction, false))
	h.handled(t)
	brain, _ := petActor.AI().(*ai.Summon)
	if brain == nil {
		t.Fatalf("pet AI = %T, want *ai.Summon", petActor.AI())
	}
	if kind, _, ok := brain.NextIntention(); !ok || kind != ai.IntentionCast {
		t.Fatalf("queued intention = %v (%t), want the strike queued behind the swing", kind, ok)
	}
	hostile.DeleteMe()
	h.srv.AdvanceUntil(t, "the swing ending", func() bool { return !petActor.IsAttackingNow() })
	h.think(t)
	if petActor.CastingNow() {
		t.Fatal("pet cast the queued strike at a target that left")
	}

	h.requireFollowsOwnerAgain(t, petActor, "pet whose queued strike lost its target")
	drainUntilQuiet(t, h.client)
}
