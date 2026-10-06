package ai

import (
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

var desireRef = skill.Ref{ID: 4, Level: 1}

// castDesireAI is an AI whose owner (radius 10) stands at the origin with a
// cast controller of range 100 that passes every gate.
func castDesireAI(t *testing.T) (*Attackable, *fakeActor, *recordingCast) {
	t.Helper()
	owner := actor(1)
	owner.radius = 10
	cast := &recordingCast{canAttempt: true, canCast: true, castRange: 100}
	brain := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	brain.SetCastController(cast)
	return brain, owner, cast
}

func queuedCast(brain *Attackable, target attackable.Combatant) (Desire, bool) {
	for _, d := range brain.Desires().Snapshot() {
		if d.Kind == IntentionCast && sameCombatant(d.FinalTarget, target) {
			return d, true
		}
	}
	return Desire{}, false
}

func TestAddCastDesireChecksConditionsOnlyWhenAsked(t *testing.T) {
	brain, _, cast := castDesireAI(t)
	target := actor(2)
	cast.desireFail = true

	brain.AddCastDesire(target, desireRef, 10, true, true)
	if _, ok := queuedCast(brain, target); ok {
		t.Fatal("a checked cast desire whose skill fails its reuse or cost gates was queued")
	}

	brain.AddCastDesire(target, desireRef, 10, false, true)
	d, ok := queuedCast(brain, target)
	if !ok {
		t.Fatal("an unchecked cast desire was refused for failing gates it skips")
	}
	if d.Skill != desireRef || d.Weight != 10 || !d.MoveToTarget {
		t.Fatalf("queued cast = %+v, want skill %v, weight 10, moving", d, desireRef)
	}
}

// TestAddCastDesireHoldNeedsTargetInReach pins the hold reach: cast range
// plus both collision radii (100 + 10 + 5 = 115), measured flat and strict.
func TestAddCastDesireHoldNeedsTargetInReach(t *testing.T) {
	tests := []struct {
		name   string
		x, z   int
		queued bool
	}{
		{name: "inside reach", x: 114, queued: true},
		{name: "on the edge", x: 115, queued: false},
		{name: "height ignored", x: 114, z: 500, queued: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			brain, _, _ := castDesireAI(t)
			target := actor(2)
			target.radius, target.x, target.z = 5, tt.x, tt.z

			brain.AddCastDesire(target, desireRef, 10, true, false)
			d, ok := queuedCast(brain, target)
			if ok != tt.queued {
				t.Fatalf("hold cast desire at x=%d queued = %v, want %v", tt.x, ok, tt.queued)
			}
			if ok && d.MoveToTarget {
				t.Fatal("hold cast desire queued as moving to its target")
			}
		})
	}

	brain, _, _ := castDesireAI(t)
	far := actor(3)
	far.x = 5000
	brain.AddCastDesire(far, desireRef, 10, true, true)
	if _, ok := queuedCast(brain, far); !ok {
		t.Fatal("a moving cast desire was refused for an out-of-reach target")
	}
}

func TestAddCastDesireAimsAtTheSkillsFinalTarget(t *testing.T) {
	brain, owner, cast := castDesireAI(t)
	target := actor(2)
	cast.final = owner

	brain.AddCastDesire(target, desireRef, 10, true, true)
	if _, ok := queuedCast(brain, owner); !ok {
		t.Fatal("cast desire not aimed at the final target the skill resolves")
	}

	brain, _, cast = castDesireAI(t)
	cast.noFinal = true
	brain.AddCastDesire(target, desireRef, 10, true, true)
	if n := brain.Desires().Len(); n != 0 {
		t.Fatalf("desires after a cast with no final target = %d, want 0", n)
	}
}

func TestAddCastDesireNeedsTargetAndCaster(t *testing.T) {
	brain, _, _ := castDesireAI(t)
	brain.AddCastDesire(nil, desireRef, 10, false, true)
	if n := brain.Desires().Len(); n != 0 {
		t.Fatalf("desires after a cast at no target = %d, want 0", n)
	}

	noCaster := NewAttackable(actor(1), &recordingMove{}, &recordingAttack{})
	noCaster.AddCastDesire(actor(2), desireRef, 10, false, true)
	if n := noCaster.Desires().Len(); n != 0 {
		t.Fatalf("desires of an actor that cannot cast = %d, want 0", n)
	}
}

func TestAddAttackDesireDamageRecordsDamageAndHate(t *testing.T) {
	owner := actor(1)
	attacker := actor(2)
	owner.known[attacker.ObjectID()] = false
	brain := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	brain.AddAttackDesireDamage(attacker, 1, 200)

	threats := brain.Threats().Snapshot()
	if len(threats) != 1 || threats[0].Damage != 1 || threats[0].Hate != 200 {
		t.Fatalf("threats = %+v, want one entry of damage 1, hate 200", threats)
	}
	d, ok := brain.Desires().NonMovingAttack(attacker)
	if ok {
		t.Fatalf("attack desire = %+v queued holding, want moving", d)
	}
	if !brain.Desires().Has(&Desire{Kind: IntentionAttack, FinalTarget: attacker}) {
		t.Fatal("no attack desire queued")
	}
}

// TestDesirePromotionOrderMixedKinds queues one desire of every kind a
// script asks for, then drops the promoted one pass after pass: each pass
// takes the heaviest left, whatever its kind.
func TestDesirePromotionOrderMixedKinds(t *testing.T) {
	owner := actor(1)
	victim := actor(2)
	leader := actor(3)
	move := &recordingMove{}
	attack := &recordingAttack{canAttack: true}
	cast := &recordingCast{canAttempt: true, canCast: true, castRange: 400}
	brain := NewAttackable(owner, move, attack)
	brain.SetCastController(cast)

	brain.AddDoNothingDesire(40, 500)
	brain.AddCastDesire(victim, desireRef, 400, true, true)
	addAttackHate(brain, victim, 0, 300)
	brain.AddFollowDesire(leader, 200)
	brain.AddWanderDesire(5, 100)

	want := []Intention{IntentionNothing, IntentionCast, IntentionAttack, IntentionFollow, IntentionWander}
	for i, kind := range want {
		if err := brain.RunAI(); err != nil {
			t.Fatalf("pass %d: RunAI() error: %v", i, err)
		}
		if got := brain.CurrentIntention(); got != kind {
			t.Fatalf("pass %d: CurrentIntention() = %v, want %v", i, got, kind)
		}
		brain.Desires().RemoveKind(kind)
	}
	if cast.castCalls != 1 || cast.castedTarget != victim {
		t.Fatalf("casts = %d at %v, want 1 at the victim", cast.castCalls, cast.castedTarget)
	}
	if attack.doAttackCalls != 1 || attack.target != victim {
		t.Fatalf("attacks = %d at %v, want 1 at the victim", attack.doAttackCalls, attack.target)
	}
	if owner.wanderCalls != 1 {
		t.Fatalf("wander walks = %d, want 1", owner.wanderCalls)
	}
}

// TestDoNothingDesireKeepsTheActorStill pins the do-nothing step: promoted,
// it neither moves, attacks nor casts, and it holds off the idle while it
// stays queued; its weight then decays with the AI clock until it drops.
func TestDoNothingDesireKeepsTheActorStill(t *testing.T) {
	owner := actor(1)
	move := &recordingMove{}
	attack := &recordingAttack{canAttack: true}
	cast := &recordingCast{canAttempt: true, canCast: true}
	brain := NewAttackable(owner, move, attack)
	brain.SetCastController(cast)

	brain.AddDoNothingDesire(40, 1)
	// The first periodic cycle only opens the promotion gate.
	if err := brain.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	for i := range 3 {
		if err := brain.TickThink(); err != nil {
			t.Fatalf("tick %d: TickThink() error: %v", i, err)
		}
		if got := brain.CurrentIntention(); got != IntentionNothing {
			t.Fatalf("tick %d: CurrentIntention() = %v, want %v", i, got, IntentionNothing)
		}
	}
	if move.stopCount != 0 || attack.doAttackCalls != 0 || cast.castCalls != 0 || owner.wanderCalls != 0 {
		t.Fatalf("doing nothing: stops %d, attacks %d, casts %d, walks %d; want none",
			move.stopCount, attack.doAttackCalls, cast.castCalls, owner.wanderCalls)
	}
	for _, p := range owner.hooks.points {
		if p == HookNoDesire {
			t.Fatal("the actor idled while its do-nothing desire was queued")
		}
	}

	// 1 - 0.5 - 0.5 leaves 0; the next decay would go below zero and drops it.
	for range 9 {
		brain.Tick()
	}
	if n := brain.Desires().Len(); n != 0 {
		t.Fatalf("desires after the do-nothing weight decayed = %d, want 0", n)
	}
	if err := brain.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() after decay = %v, want %v", got, IntentionIdle)
	}
}

// TestScriptDesiresQueueConcurrently queues desires from many goroutines
// while the AI thinks: equal requests fold their weights together and the
// heaviest kind is promoted.
func TestScriptDesiresQueueConcurrently(t *testing.T) {
	owner := actor(1)
	leader := actor(3)
	brain := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	brain.SetCastController(&recordingCast{canAttempt: true, canCast: true})

	const workers = 16
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			brain.AddDoNothingDesire(40, 10)
			brain.AddFollowDesire(leader, 5)
			brain.AddWanderDesire(5, 1)
			_ = brain.RunAI()
		})
	}
	wg.Wait()

	weights := map[Intention]float64{}
	for _, d := range brain.Desires().Snapshot() {
		weights[d.Kind] += d.Weight
	}
	want := map[Intention]float64{IntentionNothing: 10 * workers, IntentionFollow: 5 * workers, IntentionWander: workers}
	for kind, w := range want {
		if weights[kind] != w {
			t.Fatalf("%v weight = %v, want %v (weights %v)", kind, weights[kind], w, weights)
		}
	}
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionNothing {
		t.Fatalf("CurrentIntention() = %v, want %v", got, IntentionNothing)
	}
}
