package npc

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// bowFolkAttack is a scripted attack controller whose bow reuse the test
// switches.
type bowFolkAttack struct {
	*scriptedFolkAttack
	cooling bool
}

func (a *bowFolkAttack) BowCoolingDown() bool { return a.cooling }

// newBowFolkRig is the fight rig with a bow in the NPC's right hand.
func newBowFolkRig(t *testing.T) (*folkFightRig, *bowFolkAttack) {
	t.Helper()
	r := newFolkFightRig(t)
	bow := &bowFolkAttack{scriptedFolkAttack: r.attack}
	r.f.SetAttacker(bow)
	r.f.fight.weapon = &item.WeaponDetail{Type: item.WeaponBow}
	return r, bow
}

// landShot ends the bow shot in flight the way the controller does: the
// hit animation closes and the AI re-runs while the bow's reuse runs.
func landShot(r *folkFightRig, bow *bowFolkAttack) {
	bow.attacking, bow.hitAnim, bow.cooling = false, false, true
	r.f.AttackEvents().Emit(event.AttackRethink{})
	r.in.Run()
}

// reuseBow ends the bow's reuse.
func reuseBow(r *folkFightRig, bow *bowFolkAttack) {
	bow.cooling = false
	r.f.AttackEvents().Emit(event.AttackFinished{BowReuse: true})
	r.in.Run()
}

// TestFolkBowReuseWaitsForTheNextSelection follows CreatureAI
// onEvtBowAttackReuse 38-43 (a civilian NPC's AI is no AttackableAI): the
// shot's re-run meets the bow still cooling and puts the attack aside, and
// the reuse ending then starts no shot; the next AI tick does.
func TestFolkBowReuseWaitsForTheNextSelection(t *testing.T) {
	r, bow := newBowFolkRig(t)
	r.f.AddAttackDesire(r.near, 2000)
	r.in.Run()
	if len(bow.swings) != 1 {
		t.Fatalf("swings = %v, want the first shot", bow.swings)
	}
	landShot(r, bow)
	if r.f.fight.next == nil {
		t.Fatal("the attack was not put aside while the bow cools down")
	}
	reuseBow(r, bow)
	if len(bow.swings) != 1 {
		t.Fatalf("swings = %v as the reuse ended with an attack put aside, want no shot until the next AI tick", bow.swings)
	}
	r.tick(t)
	if len(bow.swings) != 2 || bow.swings[1] != r.near.ObjectID() {
		t.Fatalf("swings = %v after the next AI tick, want a second shot at %d", bow.swings, r.near.ObjectID())
	}
}

// TestFolkBowReuseThinksWithNothingPutAside follows CreatureAI
// onEvtBowAttackReuse 38-43: with no attack put aside, the reuse ending
// takes one more step of the current attack, which shoots.
func TestFolkBowReuseThinksWithNothingPutAside(t *testing.T) {
	r, bow := newBowFolkRig(t)
	r.f.AddAttackDesire(r.near, 2000)
	r.in.Run()
	landShot(r, bow)
	r.f.fight.next = nil
	reuseBow(r, bow)
	if len(bow.swings) != 2 {
		t.Fatalf("swings = %v as the reuse ended, want the current attack to shoot again", bow.swings)
	}
}

// TestFolkAbortAllAndStopAttackStopTheSwing follows Creature.abortAll
// 1298-1306: the effect actor's abort and attack stop end the swing in
// flight.
func TestFolkAbortAllAndStopAttackStopTheSwing(t *testing.T) {
	r := newFolkFightRig(t)
	r.f.AddAttackDesire(r.near, 2000)
	r.in.Run()
	r.f.StopAttack()
	if r.attack.attacking {
		t.Fatal("StopAttack left the swing in flight")
	}
	r.attack.DoAttack(r.near)
	r.f.AbortAll(false)
	if r.attack.attacking {
		t.Fatal("AbortAll left the swing in flight")
	}
}

// throughGeo is hostileGeo that lets a creature stand anywhere it is put.
type throughGeo struct{ hostileGeo }

func (throughGeo) ValidLocation(_, _, _, tx, ty, tz int) location.Location {
	return location.Location{X: tx, Y: ty, Z: tz}
}

// deadTarget is a target that has died.
type deadTarget struct{ *hostileTarget }

func (deadTarget) Dead() bool { return true }

// folkFollowRig is a walking civilian NPC at (100, 0, 0), in its walk
// stance, with a scripted attack controller, its follow dice answered by
// hundred and other, and a target 500 units east of it.
type folkFollowRig struct {
	f      *Folk
	in     *sim.Inline
	attack *scriptedFolkAttack
	target *hostileTarget
	// hundred answers the follow step's move chance, other (capped to the
	// die) its scatter and teleport offset dice.
	hundred, other int
}

func newFolkFollowRig(t *testing.T) *folkFollowRig {
	t.Helper()
	inst, err := NewInstance(1, &Template{ID: 31677, TemplateID: 31677, Type: "Folk", Level: 80, HPMax: 2880, RunSpeed: 120, WalkSpeed: 50, BaseAttackRange: 100, CanMove: true})
	if err != nil {
		t.Fatal(err)
	}
	inst.Home = location.Location{X: 100}
	f, err := NewFolk(inst)
	if err != nil {
		t.Fatal(err)
	}
	r := &folkFollowRig{f: f, in: sim.NewInline(time.Unix(0, 0)), attack: &scriptedFolkAttack{}, target: &hostileTarget{id: 2, playable: true}, hundred: 99}
	q := r.in.NewQueue("folk")
	w := world.New()
	if err := f.Attach(FolkRuntime{World: w, Queue: q, Sink: &recordingSink{}, AI: &recordingFolkAI{}}); err != nil {
		t.Fatal(err)
	}
	f.SetAttacker(r.attack)
	if _, err := f.EnableMovement(FolkMovement{Geo: throughGeo{}, Queue: q, World: w}); err != nil {
		t.Fatal(err)
	}
	f.fight.roll = func(n int) int {
		if n == 100 {
			return r.hundred
		}
		return min(r.other, n-1)
	}
	w.Spawn(f, 100, 0, 0, 0)
	w.Spawn(r.target, 600, 0, 0, 0)
	// The follower starts in its walk stance, so the step's run shows.
	f.forceWalkStance()
	return r
}

// follow queues a follow desire on target, makes it current and takes one
// follow step at the AI step count step.
func (r *folkFollowRig) follow(target attackable.Combatant, step int) *ai.Desire {
	r.f.AddFollowDesire(target, 100)
	d, _ := r.f.cast.desires.Peek()
	r.f.setCurrentDesire(d)
	r.f.cast.step = step
	r.f.thinkFollow(d)
	r.in.Run()
	return d
}

// TestFolkFollowStepsEverySecondTick follows NpcAI.thinkFollow 111-117 and
// 210-218: on an even step nothing happens; on an odd one the follower
// switches to its run stance and, outside 150 of its target and on a roll
// over 50, walks to a point within 300 of it (the scatter rolled to zero
// here, so to the target itself).
func TestFolkFollowStepsEverySecondTick(t *testing.T) {
	r := newFolkFollowRig(t)
	r.follow(r.target, 0)
	if r.f.Running() || r.f.IsMoving() {
		t.Fatalf("running = %v, moving = %v on an even step, want neither", r.f.Running(), r.f.IsMoving())
	}
	r.f.cast.step = 1
	d, _ := r.f.cast.desires.Peek()
	r.f.thinkFollow(d)
	if !r.f.Running() {
		t.Fatal("the follow step left the NPC in its walk stance, want run")
	}
	to, moving := r.f.MovingTo()
	if !moving || to.X != 600 || to.Y != 0 {
		t.Fatalf("moving = %v to %+v, want a walk to the target at (600, 0)", moving, to)
	}
}

// TestFolkFollowRollsToMove follows NpcAI.thinkFollow 210: a roll of 50 or
// less leaves the follower where it stands.
func TestFolkFollowRollsToMove(t *testing.T) {
	r := newFolkFollowRig(t)
	r.hundred = 50
	r.follow(r.target, 1)
	if r.f.IsMoving() {
		t.Fatal("the follower moved on a roll of 50, want only over 50")
	}
	if !r.f.Running() {
		t.Fatal("the follow step left the NPC in its walk stance, want run")
	}
}

// TestFolkFollowHoldsWithinItsLooseRadius follows NpcAI.thinkFollow 210: a
// target within 150 draws no step.
func TestFolkFollowHoldsWithinItsLooseRadius(t *testing.T) {
	r := newFolkFollowRig(t)
	nearby := &hostileTarget{id: 3, playable: true}
	r.f.world.Spawn(nearby, 100+folkFollowLooseRadius, 0, 0, 0)
	r.follow(nearby, 1)
	if r.f.IsMoving() {
		t.Fatal("the follower moved toward a target 150 away, want it to hold")
	}
}

// TestFolkFollowDropsADeadOrSelfTarget follows NpcAI.thinkFollow 123-128: a
// dead target, or the follower itself, drops the follow desire.
func TestFolkFollowDropsADeadOrSelfTarget(t *testing.T) {
	r := newFolkFollowRig(t)
	dead := deadTarget{&hostileTarget{id: 4, playable: true}}
	r.f.world.Spawn(dead.hostileTarget, 900, 0, 0, 0)
	r.follow(dead, 1)
	if r.f.cast.desires.HasKind(ai.IntentionFollow) || r.f.IsMoving() {
		t.Fatal("the follow on a dead target was kept")
	}
	r.follow(r.f, 1)
	if r.f.cast.desires.HasKind(ai.IntentionFollow) || r.f.IsMoving() {
		t.Fatal("the follow on the NPC itself was kept")
	}
}

// TestFolkFollowTeleportsAStuckFollower follows NpcAI.thinkFollow 130-138:
// a follower whose path finding failed 10 times stops its swing and walk,
// jumps to within 10 of its target and starts a new streak.
func TestFolkFollowTeleportsAStuckFollower(t *testing.T) {
	r := newFolkFollowRig(t)
	r.other = 20
	for range folkFollowGeoFailLimit {
		r.f.motion.AddGeoPathFailCount()
	}
	r.attack.DoAttack(r.target)
	r.follow(r.target, 1)
	x, y, _ := r.f.Position()
	if x != 600+folkFollowTeleportOffset || y != folkFollowTeleportOffset {
		t.Fatalf("follower at (%d, %d), want (%d, %d): next to its target", x, y, 600+folkFollowTeleportOffset, folkFollowTeleportOffset)
	}
	if n := r.f.motion.GeoPathFailCount(); n != 0 {
		t.Fatalf("geo path fail streak = %d after the jump, want 0", n)
	}
	if r.attack.attacking || r.f.IsMoving() {
		t.Fatal("the jump left the swing or the walk going")
	}
}
