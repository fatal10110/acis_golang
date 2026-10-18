package ai

import (
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// lostTargetDesires queues, for each kind aimed at a creature, a desire on
// target outweighing a follow desire on other.
var lostTargetDesires = []struct {
	name  string
	queue func(brain *Attackable, target *fakeActor)
	kind  Intention
}{
	{"attack", func(b *Attackable, target *fakeActor) {
		b.Desires().AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: target, Weight: 100, MoveToTarget: true})
	}, IntentionAttack},
	{"cast", func(b *Attackable, target *fakeActor) {
		b.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: skill.Ref{ID: 4, Level: 1}, Weight: 100})
	}, IntentionCast},
	{"follow", func(b *Attackable, target *fakeActor) {
		b.AddFollowDesire(target, 100)
	}, IntentionFollow},
	{"flee", func(b *Attackable, target *fakeActor) {
		b.Desires().AddOrUpdate(&Desire{Kind: IntentionFlee, FinalTarget: target, Distance: 500, Weight: 100})
	}, IntentionFlee},
}

// lose takes target out of owner's reach: owner forgets it, or it dies.
func lose(owner, target *fakeActor, dead bool) {
	if dead {
		target.alikeDead = true
		return
	}
	owner.known[target.ObjectID()] = false
}

// TestPruneDropsEveryDesireOnALostCreature pins the prune at the start of
// desire selection: a desire of any kind aimed at a creature the actor no
// longer knows, or at a dead one, leaves the queue before selection, and the
// next desire is taken up. It runs while the actor is out of control too,
// and the continue pass, which never reads the queue, leaves it queued.
func TestPruneDropsEveryDesireOnALostCreature(t *testing.T) {
	for _, tc := range lostTargetDesires {
		for _, dead := range []bool{false, true} {
			how := "unknown"
			if dead {
				how = "dead"
			}
			t.Run(tc.name+"/"+how, func(t *testing.T) {
				setup := func() (*Attackable, *fakeActor, *fakeActor, *fakeActor) {
					owner := actor(1)
					target, other := actor(2), actor(3)
					brain := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
					pastFirstCycle(t, brain)
					brain.SetCastController(&recordingCast{disabled: true})
					tc.queue(brain, target)
					brain.AddFollowDesire(other, 1)
					lose(owner, target, dead)
					return brain, owner, target, other
				}
				probe := func(target *fakeActor) *Desire {
					return &Desire{Kind: tc.kind, FinalTarget: target, Skill: skill.Ref{ID: 4, Level: 1}}
				}

				brain, _, target, _ := setup()
				if err := brain.Think(); err != nil {
					t.Fatalf("Think() error: %v", err)
				}
				if !brain.Desires().Has(probe(target)) {
					t.Fatal("continue pass dropped the desire, want it left for desire selection")
				}

				brain, _, target, other := setup()
				if err := brain.RunAI(); err != nil {
					t.Fatalf("RunAI() error: %v", err)
				}
				if brain.Desires().Has(probe(target)) {
					t.Fatal("desire on the lost creature still queued after desire selection")
				}
				if got := brain.CurrentIntention(); got != IntentionFollow || !sameCombatant(brain.TopDesireTarget(), other) {
					t.Fatalf("intention after selection = %v on %v, want the follow on the other creature", got, brain.TopDesireTarget())
				}

				brain, owner, target, _ := setup()
				owner.denyAction = true
				if err := brain.TickThink(); err != nil {
					t.Fatalf("TickThink() error: %v", err)
				}
				if brain.Desires().Has(probe(target)) {
					t.Fatal("desire on the lost creature still queued after an out-of-control cycle")
				}
			})
		}
	}
}

// TestStunnedPeriodicIdleKeepsTheAttack pins the periodic idle of a stunned
// monster whose target died or left its known list: its attack desire is
// pruned, and each stunned cycle runs the cycle's own idle once, keeping the
// attack current. The first cycle after the stun ends finds an in-control
// actor still attacking with nothing queued: desire selection idles it and
// the cycle's own idle runs again, so the no-desire point opens twice.
func TestStunnedPeriodicIdleKeepsTheAttack(t *testing.T) {
	for _, dead := range []bool{false, true} {
		name := "unknown"
		if dead {
			name = "dead"
		}
		t.Run(name, func(t *testing.T) {
			owner := actor(1)
			target := actor(2)
			brain := NewAttackable(owner, &recordingMove{}, &recordingAttack{canAttack: true})
			if err := brain.TickThink(); err != nil {
				t.Fatalf("first TickThink() error: %v", err)
			}
			attackUnlatched(t, brain, target)
			owner.denyAction = true
			lose(owner, target, dead)

			for i := range 3 {
				owner.hooks.points = nil
				if err := brain.TickThink(); err != nil {
					t.Fatalf("stunned TickThink() error: %v", err)
				}
				assertHookPoints(t, owner, "on a stunned cycle", HookSeeCreature, HookNoDesire)
				if got := brain.CurrentIntention(); got != IntentionAttack {
					t.Fatalf("CurrentIntention() after stunned cycle %d = %v, want %v kept", i, got, IntentionAttack)
				}
			}
			if got := brain.Desires().Len(); got != 0 {
				t.Fatalf("queued desires while stunned = %d, want the attack desire pruned", got)
			}

			owner.denyAction = false
			owner.hooks.points = nil
			if err := brain.TickThink(); err != nil {
				t.Fatalf("recovery TickThink() error: %v", err)
			}
			assertHookPoints(t, owner, "on the first cycle after the stun", HookSeeCreature, HookNoDesire, HookNoDesire)
			if got := brain.CurrentIntention(); got != IntentionIdle {
				t.Fatalf("CurrentIntention() after recovery = %v, want idle", got)
			}
		})
	}
}

// TestContinueStepsCastWithoutDesire pins the continue pass on a cast whose
// desire left the queue: it casts again, as the pass never reads the queue.
func TestContinueStepsCastWithoutDesire(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ref := skill.Ref{ID: 4, Level: 1}
	cast := &recordingCast{canAttempt: true, canCast: true}
	brain := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	pastFirstCycle(t, brain)
	brain.SetCastController(cast)
	brain.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: ref, Weight: 10})
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if cast.castCalls != 1 {
		t.Fatalf("Cast calls after selection = %d, want 1", cast.castCalls)
	}
	brain.Desires().Clear()
	owner.hooks.points = nil

	if err := brain.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	if cast.castCalls != 2 || cast.castedTarget != target || cast.castedRef != ref {
		t.Fatalf("Cast calls after the continue pass = %d on %v/%v, want a second cast of the same skill on the target", cast.castCalls, cast.castedTarget, cast.castedRef)
	}
	if got := brain.CurrentIntention(); got != IntentionCast {
		t.Fatalf("CurrentIntention() after the continue pass = %v, want %v", got, IntentionCast)
	}
	assertHookPoints(t, owner, "on the cast's continue pass")
}

// TestBackToPeaceKeepsScriptDesires pins the return to peace when the last
// hate is stopped: the hate tables are cleared and the stopped target's
// attack desire leaves the queue, but a cast, a flee and a walk queued by a
// behavior stay, and the current intention and movement are left alone.
func TestBackToPeaceKeepsScriptDesires(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	move := &recordingMove{}
	brain := NewAttackable(owner, move, &recordingAttack{canAttack: true})
	brain.SetCastController(&recordingCast{disabled: true})
	attackUnlatched(t, brain, target)
	brain.AddHate(target, 30)

	curse := &Desire{Kind: IntentionCast, FinalTarget: target, Skill: skill.Ref{ID: 4, Level: 1}, Weight: 1_000_000}
	flee := &Desire{Kind: IntentionFlee, FinalTarget: target, Distance: 500, Weight: 5}
	walk := &Desire{Kind: IntentionMoveTo, Location: location.Location{X: 300}, Weight: 5}
	brain.Desires().AddOrUpdate(curse)
	brain.Desires().AddOrUpdate(flee)
	brain.Desires().AddOrUpdate(walk)
	brain.Hates().StopHate(target)
	stops := move.stopCount

	brain.StopAggroHate(target)

	if !brain.Threats().IsEmpty() || !brain.Hates().IsEmpty() {
		t.Fatal("hate tables not cleared on the return to peace")
	}
	if brain.Desires().Has(&Desire{Kind: IntentionAttack, FinalTarget: target}) {
		t.Fatal("attack desire on the stopped target still queued")
	}
	for _, d := range []*Desire{curse, flee, walk} {
		if !brain.Desires().Has(d) {
			t.Fatalf("%v desire dropped by the return to peace, want it kept", d.Kind)
		}
	}
	if got := brain.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() = %v, want %v kept", got, IntentionAttack)
	}
	if move.stopCount != stops {
		t.Fatalf("move stops = %d, want %d (the return to peace stops nothing)", move.stopCount, stops)
	}
}

// tickDecay advances the AI clock one cycle and runs the hate refresh and
// weight decay that cycle's periodic pass would run after its selection,
// without the selection itself.
func tickDecay(a *Attackable) {
	a.Tick()
	a.mu.Lock()
	a.cycleDecayLocked()
	a.mu.Unlock()
}

// TestCycleDecayRunsAfterTheCycleSelection pins the periodic cycle's order:
// selection first, then every third cycle the hate refresh and weight decay.
// The cycle whose decay drops the last attack desire still keeps the attack
// it selected; the next cycle finds the queue empty and idles.
func TestCycleDecayRunsAfterTheCycleSelection(t *testing.T) {
	owner, target := actor(1), actor(2)
	brain := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	brain.AddDamageHate(target, 0, 100)
	brain.Desires().AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: target, Weight: 5, MoveToTarget: true})
	if err := brain.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() = %v, want %v", got, IntentionAttack)
	}
	cycle := func(n int) {
		t.Helper()
		brain.Tick()
		if err := brain.TickThink(); err != nil {
			t.Fatalf("cycle %d TickThink() error: %v", n, err)
		}
	}
	for n := 1; n <= 3; n++ {
		cycle(n)
		if got := brain.CurrentIntention(); got != IntentionAttack {
			t.Fatalf("cycle %d CurrentIntention() = %v, want %v", n, got, IntentionAttack)
		}
	}
	if n := brain.Desires().Len(); n != 0 {
		t.Fatalf("desires after the third cycle's decay = %d, want 0", n)
	}
	if got, want := brain.Threats().Hate(target), 93.4; math.Abs(got-want) > 1e-9 {
		t.Fatalf("hate after the third cycle's decay = %v, want %v", got, want)
	}
	cycle(4)
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("cycle 4 CurrentIntention() = %v, want %v", got, IntentionIdle)
	}
}
