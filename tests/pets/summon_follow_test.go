package pets

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// petFollowToggleAction is the pet shortcut that switches following the
// owner on and off.
const petFollowToggleAction = int32(15)

// petCloseToOwner is how near a following pet ends up: its 70 follow offset
// plus both collision radii, with slack for the last follow step.
const petCloseToOwner = 200

// ownerEffector returns the owner as the effector of an effect it applies.
func (h *petWorld) ownerEffector(t *testing.T) effect.Actor {
	t.Helper()
	obj, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner missing from world state")
	}
	owner, ok := obj.(effect.Actor)
	if !ok {
		t.Fatalf("world player %T is not an effect actor", obj)
	}
	return owner
}

// landPetEffect applies the named effect from effector to the pet on the
// pet's queue, waits for its start hook, and returns it for removal.
func landPetEffect(t *testing.T, petActor *summon.Actor, effector effect.Actor, name string) *effect.Effect {
	t.Helper()
	return landPetSkillEffect(t, petActor, effector, 1299, modelskill.EffectTemplate{Name: name, Time: 30})
}

// landPetSkillEffect applies tmpl as skill skillID's effect from effector to
// the pet on the pet's queue, waits for its start hook, and returns it.
func landPetSkillEffect(t *testing.T, petActor *summon.Actor, effector effect.Actor, skillID modelskill.ID, tmpl modelskill.EffectTemplate) *effect.Effect {
	t.Helper()
	e, err := effect.New(effect.Skill{ID: skillID, Level: 1}, tmpl)
	if err != nil {
		t.Fatalf("effect.New(%s): %v", tmpl.Name, err)
	}
	e.Effector, e.Effected = effector, petActor
	runOnPetQueue(t, petActor, func() { petActor.EffectList().Add(e) })
	return e
}

// removePetEffect ends e early on the pet's queue, running its exit hook.
func removePetEffect(t *testing.T, petActor *summon.Actor, e *effect.Effect) {
	t.Helper()
	runOnPetQueue(t, petActor, func() { petActor.EffectList().Remove(e) })
}

func runOnPetQueue(t *testing.T, petActor *summon.Actor, fn func()) {
	t.Helper()
	done := make(chan struct{})
	if !petActor.Queue().Post(func() { fn(); close(done) }) {
		t.Fatal("post to pet queue: queue closed")
	}
	<-done
}

// ownerWalksAway sends the owner 600 units along X and lets the walk finish.
func (h *petWorld) ownerWalksAway(t *testing.T) {
	t.Helper()
	x, y, z := h.srv.PlayerPosition(t, h.ownerID)
	destX := x + 600
	h.client.Send(encodeMoveBackwardToLocation(int32(destX), int32(y), int32(z)))
	h.srv.AdvanceUntil(t, "owner reaching the walk destination", func() bool {
		ox, _, _ := h.srv.PlayerPosition(t, h.ownerID)
		return ox == destX
	})
}

func (h *petWorld) petDistanceToOwner(t *testing.T, petActor *summon.Actor) float64 {
	t.Helper()
	ox, oy, _ := h.srv.PlayerPosition(t, h.ownerID)
	px, py, _ := petActor.Position()
	return math.Hypot(float64(ox-px), float64(oy-py))
}

// bootFollowingPet spawns the owner's wolf with the AI registry wired, so a
// test drives the pet's one-second thinks by hand.
func bootFollowingPet(t *testing.T) (*petWorld, *summon.Actor) {
	t.Helper()
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithAITask()})
	petActor, _ := h.spawnWolf(t)
	drainUntilQuiet(t, h.client)
	return h, petActor
}

// think lets one AI interval pass and runs the AI cycle it ends with.
func (h *petWorld) think(t *testing.T) {
	t.Helper()
	h.srv.Advance(t, task.AITick)
	if err := h.srv.AI.Tick(); err != nil {
		t.Fatalf("AI.Tick() = %v", err)
	}
	h.srv.Settle(t)
}

// handled lets the server finish handling every frame the client sent.
func (h *petWorld) handled(t *testing.T) {
	t.Helper()
	h.srv.Advance(t, 10*time.Millisecond)
}

// requirePetStaysPut lets the owner walk away and three AI thinks pass,
// failing if the pet moved at all.
func (h *petWorld) requirePetStaysPut(t *testing.T, petActor *summon.Actor, why string) {
	t.Helper()
	x, y, z := petActor.Position()
	h.ownerWalksAway(t)
	for range 3 {
		h.think(t)
	}
	if gx, gy, gz := petActor.Position(); gx != x || gy != y || gz != z {
		t.Fatalf("%s: pet moved from (%d,%d,%d) to (%d,%d,%d), want it held in place", why, x, y, z, gx, gy, gz)
	}
	if petActor.Move().Moving() {
		t.Fatalf("%s: pet has a move in flight", why)
	}
}

// requirePetCatchesUp runs AI thinks until the pet is back beside its owner.
func (h *petWorld) requirePetCatchesUp(t *testing.T, petActor *summon.Actor, what string) {
	t.Helper()
	for range 10 {
		if h.petDistanceToOwner(t, petActor) < petCloseToOwner {
			return
		}
		h.think(t)
	}
	t.Fatalf("%s: pet still %.0f from its owner after 10 thinks", what, h.petDistanceToOwner(t, petActor))
}

// TestRootedPetDoesNotFollowOwner roots a pet that follows its owner. While
// rooted it stays where it is as the owner walks off; once the root ends it
// walks after the owner again.
func TestRootedPetDoesNotFollowOwner(t *testing.T) {
	t.Parallel()
	h, petActor := bootFollowingPet(t)
	if got := petActor.Intent(); got != summon.IntentFollowOwner {
		t.Fatalf("pet intent after spawn = %v, want follow-owner", got)
	}

	root := landPetEffect(t, petActor, petActor, "Root")
	h.requirePetStaysPut(t, petActor, "rooted pet")

	removePetEffect(t, petActor, root)
	h.requirePetCatchesUp(t, petActor, "pet following its owner once the root ends")
	drainUntilQuiet(t, h.client)
}

// TestImmobilizedPetDropsFollowAndRestoresIt lands the owner's immobilizing
// pet buff on a following pet. The buff drops follow mode, so the pet idles
// and stays behind; when the buff ends, follow mode comes back and the pet
// walks after its owner.
func TestImmobilizedPetDropsFollowAndRestoresIt(t *testing.T) {
	t.Parallel()
	h, petActor := bootFollowingPet(t)

	buff := landPetEffect(t, petActor, h.ownerEffector(t), "ImobilePetBuff")
	if !petActor.Immobilized() {
		t.Fatal("ImobilePetBuff from the owner left the pet mobile")
	}
	if got := petActor.Intent(); got != summon.IntentIdle {
		t.Fatalf("pet intent under the buff = %v, want idle", got)
	}
	h.requirePetStaysPut(t, petActor, "immobilized pet")

	removePetEffect(t, petActor, buff)
	if got := petActor.Intent(); got != summon.IntentFollowOwner {
		t.Fatalf("pet intent after the buff = %v, want follow-owner restored", got)
	}
	h.requirePetCatchesUp(t, petActor, "pet following its owner once the buff ends")
	drainUntilQuiet(t, h.client)
}

// TestImmobilizedPetRestoresFollowModeFromBeforeTheBuff has the owner turn
// follow off, land the immobilizing buff, then turn follow back on while the
// buff holds. The pet cannot move meanwhile, and when the buff ends it goes
// back to the follow mode it had when the buff landed: off.
func TestImmobilizedPetRestoresFollowModeFromBeforeTheBuff(t *testing.T) {
	t.Parallel()
	h, petActor := bootFollowingPet(t)

	h.client.Send(encodeRequestActionUse(petFollowToggleAction, false))
	h.handled(t)
	if got := petActor.Intent(); got != summon.IntentIdle {
		t.Fatalf("pet intent after follow off = %v, want idle", got)
	}

	buff := landPetEffect(t, petActor, h.ownerEffector(t), "ImobilePetBuff")
	h.client.Send(encodeRequestActionUse(petFollowToggleAction, false))
	h.handled(t)
	if got := petActor.Intent(); got != summon.IntentFollowOwner {
		t.Fatalf("pet intent after follow on under the buff = %v, want follow-owner", got)
	}
	h.requirePetStaysPut(t, petActor, "immobilized pet told to follow")

	removePetEffect(t, petActor, buff)
	if got := petActor.Intent(); got != summon.IntentIdle {
		t.Fatalf("pet intent after the buff = %v, want idle (follow was off when it landed)", got)
	}
	h.requirePetStaysPut(t, petActor, "pet whose follow mode was off before the buff")
	drainUntilQuiet(t, h.client)
}

// TestStackedImmobilizingBuffsRestoreFollowModeFromTheLastLock lands the
// owner's Servitor Empowerment (ImobilePetBuff) on a following pet, then has
// the pet use Wild Defense (ImobileBuff on itself) while it holds. The first
// lock drops follow mode; the second records that follow mode is already off.
// Ending either buff restores that recorded mode, so the pet stays idle and
// does not walk after its owner, and ending the other one keeps it off.
func TestStackedImmobilizingBuffsRestoreFollowModeFromTheLastLock(t *testing.T) {
	t.Parallel()
	h, petActor := bootFollowingPet(t)

	empowerment := landPetSkillEffect(t, petActor, h.ownerEffector(t), 1299,
		modelskill.EffectTemplate{Name: "ImobilePetBuff", Time: 30, StackType: "pd_up_special", StackOrder: 1})
	wildDefense := landPetSkillEffect(t, petActor, petActor, 4711,
		modelskill.EffectTemplate{Name: "ImobileBuff", Time: 30, StackType: "ultimate_buff", StackOrder: 1})
	if got := len(petActor.EffectList().All()); got != 2 {
		t.Fatalf("pet carries %d effects, want both immobilizing buffs", got)
	}
	if got := petActor.Intent(); got != summon.IntentIdle {
		t.Fatalf("pet intent under both buffs = %v, want idle", got)
	}

	removePetEffect(t, petActor, empowerment)
	if petActor.FollowActive() {
		t.Fatal("follow mode after the first buff ended = on, want off (the second lock recorded it off)")
	}
	if got := petActor.Intent(); got != summon.IntentIdle {
		t.Fatalf("pet intent after the first buff ended = %v, want idle", got)
	}
	h.requirePetStaysPut(t, petActor, "pet after the first of two stacked buffs ended")

	removePetEffect(t, petActor, wildDefense)
	if petActor.FollowActive() {
		t.Fatal("follow mode after both buffs ended = on, want off")
	}
	h.requirePetStaysPut(t, petActor, "pet after both stacked buffs ended")
	drainUntilQuiet(t, h.client)
}

// TestPetStopAfterAttackFollowsOwner sends the pet at a monster and presses
// Stop. The pet drops the attack and goes back to following its owner, and
// walks after the owner when the owner moves away, instead of standing idle
// until follow is toggled.
func TestPetStopAfterAttackFollowsOwner(t *testing.T) {
	t.Parallel()
	h, petActor := bootFollowingPet(t)
	hostile := h.srv.SpawnHostileNPC(t)

	h.client.Send(encodeAction(hostile.ObjectID(), hostileX, hostileY, hostileZ, false))
	drainFrames(t, h.client)
	h.client.Send(encodeRequestActionUse(petAttackAction, false))
	h.handled(t)
	if got := petActor.Intent(); got != summon.IntentAttackTarget {
		t.Fatalf("pet intent while attacking = %v, want attack-target", got)
	}

	h.client.Send(encodeRequestActionUse(petStopAction, false))
	h.handled(t)
	if got := petActor.Intent(); got != summon.IntentFollowOwner {
		t.Fatalf("pet intent after Stop = %v, want follow-owner", got)
	}

	h.ownerWalksAway(t)
	h.requirePetCatchesUp(t, petActor, "pet following its owner after Stop")
	if got := petActor.Intent(); got != summon.IntentFollowOwner {
		t.Fatalf("pet intent after catching up = %v, want follow-owner", got)
	}
	drainUntilQuiet(t, h.client)
}
