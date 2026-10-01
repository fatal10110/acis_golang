package ai

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// ---- from attackable_threat_test.go ----
func TestAttackableAITickDecaysThreatEveryThirdTick(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	addAttackHate(ai, target, 0, 20)

	ai.Tick()
	ai.Tick()
	if got := ai.Threats().Hate(target); got != 20 {
		t.Fatalf("hate after two ticks = %v, want 20", got)
	}

	ai.Tick()
	if got, want := ai.Threats().Hate(target), 13.4; math.Abs(got-want) > 0.000001 {
		t.Fatalf("hate after third tick = %v, want %v", got, want)
	}
}

func TestAttackableAITickDecaysCastAndNothingDesireWeights(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: skill.Ref{ID: 4, Level: 1}, Weight: 70000})
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionNothing, Weight: 1})

	ai.Tick()
	ai.Tick()
	got, ok := ai.Desires().Peek()
	if !ok || got.Kind != IntentionCast || got.Weight != 70000 {
		t.Fatalf("Peek after two ticks = (%v, %v), want CAST 70000", got, ok)
	}

	ai.Tick()
	got, ok = ai.Desires().Peek()
	if !ok || got.Kind != IntentionCast {
		t.Fatalf("Peek after third tick = (%v, %v), want CAST", got, ok)
	}
	if math.Abs(got.Weight-4000) > 0.000001 {
		t.Fatalf("CAST weight after third tick = %v, want 4000", got.Weight)
	}
	ai.Desires().RemoveKind(IntentionCast)
	got, ok = ai.Desires().Peek()
	if !ok || got.Kind != IntentionNothing {
		t.Fatalf("Peek NOTHING after CAST removed = (%v, %v), want NOTHING", got, ok)
	}
	if math.Abs(got.Weight-0.5) > 0.000001 {
		t.Fatalf("NOTHING weight after third tick = %v, want 0.5", got.Weight)
	}
}

func TestAttackableAITickDropsCastDesireBelowDecayAmount(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: skill.Ref{ID: 4, Level: 1}, Weight: 50000})

	ai.Tick()
	ai.Tick()
	ai.Tick()

	if got := ai.Desires().Len(); got != 0 {
		t.Fatalf("queued desires after CAST decay = %d, want 0", got)
	}
}

func TestAttackableRunAIPrunesZeroWeightCastDesire(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	cast := &recordingCast{canAttempt: true, canCast: true}
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	ai.SetCastController(cast)
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: skill.Ref{ID: 4, Level: 1}, Weight: 0})

	ai.RunAI()

	if got := ai.Desires().Len(); got != 0 {
		t.Fatalf("queued desires = %d, want 0", got)
	}
	if got := ai.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want %v", got, IntentionIdle)
	}
	if cast.castCalled {
		t.Fatal("Cast called for zero-weight CAST desire")
	}
}

func TestAttackableRunAIPrunesCastWhenHPMPDisabledFails(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	cast := &recordingCast{canAttempt: true, canCast: true, hpMpFail: true}
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	ai.SetCastController(cast)
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: skill.Ref{ID: 4, Level: 1}, Weight: 100})

	ai.RunAI()

	if got := ai.Desires().Len(); got != 0 {
		t.Fatalf("queued desires = %d, want 0", got)
	}
	if got := ai.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want %v", got, IntentionIdle)
	}
	if cast.castCalled {
		t.Fatal("Cast called for CAST desire that failed HP/MP/mute")
	}
}

func TestAttackableRunAIPrunesAttackDesireBeyond1500(t *testing.T) {
	owner := actor(1)
	near := actor(2)
	far := actor(3)
	far.z = 1501
	owner.known = map[int32]bool{near.ObjectID(): true, far.ObjectID(): true}
	strike := &recordingAttack{canAttack: true}
	ai := NewAttackable(owner, &recordingMove{}, strike)
	addAttackHate(ai, far, 0, 100)
	addAttackHate(ai, near, 0, 50)

	ai.RunAI()

	if strike.target != near {
		t.Fatalf("attacked target = %v, want nearer attacker (far desire pruned)", strike.target)
	}
	if ai.Desires().Has(&Desire{Kind: IntentionAttack, FinalTarget: far}) {
		t.Fatal("far ATTACK desire still queued")
	}
}

func TestAttackableRunAIKeepsAttackDesireAt1500(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	target.x = 1500
	owner.known = map[int32]bool{target.ObjectID(): true}
	strike := &recordingAttack{canAttack: true}
	ai := NewAttackable(owner, &recordingMove{}, strike)
	addAttackHate(ai, target, 0, 20)

	ai.RunAI()

	if strike.target != target {
		t.Fatalf("attacked target = %v, want target at exactly 1500", strike.target)
	}
}

// TestAttackableRunAIKeepsFarAttackWhenOutOfControl pins NpcAI.runAI's
// out-of-control gate: a stunned or confused actor neither prunes a far
// ATTACK desire nor selects it, so the current intention stays idle and no
// attack starts.
func TestAttackableRunAIKeepsFarAttackWhenOutOfControl(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*fakeActor)
	}{
		{"denied AI action", func(a *fakeActor) { a.denyAction = true }},
		{"confused", func(a *fakeActor) { a.confused = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			owner := actor(1)
			tc.set(owner)
			far := actor(2)
			far.x = 2000
			owner.known = map[int32]bool{far.ObjectID(): true}
			strike := &recordingAttack{canAttack: true}
			ai := NewAttackable(owner, &recordingMove{}, strike)
			addAttackHate(ai, far, 0, 20)

			ai.RunAI()

			if !ai.Desires().Has(&Desire{Kind: IntentionAttack, FinalTarget: far}) {
				t.Fatal("far ATTACK desire pruned while out of control")
			}
			if got := ai.CurrentIntention(); got != IntentionIdle {
				t.Fatalf("CurrentIntention() = %v, want %v (no selection while out of control)", got, IntentionIdle)
			}
			if strike.target != nil {
				t.Fatalf("attacked %v while out of control, want none", strike.target)
			}
		})
	}
}

// TestAttackableAttackFinishedContinuesAttackWhenOutOfControl pins a
// finished swing's think past the out-of-control gate: RunAI steps nothing,
// but AttackFinished continues the current attack for a confused actor and
// does nothing for one that is denied AI action.
func TestAttackableAttackFinishedContinuesAttackWhenOutOfControl(t *testing.T) {
	for _, tc := range []struct {
		name  string
		set   func(*fakeActor)
		swing bool
	}{
		{"denied AI action", func(a *fakeActor) { a.denyAction = true }, false},
		{"confused", func(a *fakeActor) { a.confused = true }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			owner := actor(1)
			target := actor(2)
			owner.known = map[int32]bool{target.ObjectID(): true}
			strike := &recordingAttack{canAttack: true}
			ai := NewAttackable(owner, &recordingMove{}, strike)
			addAttackHate(ai, target, 0, 20)
			if err := ai.RunAI(); err != nil {
				t.Fatalf("RunAI() error = %v, want nil", err)
			}
			if strike.doAttackCalls != 1 {
				t.Fatalf("setup DoAttack calls = %d, want 1", strike.doAttackCalls)
			}
			tc.set(owner)

			if err := ai.RunAI(); err != nil {
				t.Fatalf("out-of-control RunAI() error = %v, want nil", err)
			}
			if strike.doAttackCalls != 1 {
				t.Fatalf("DoAttack calls after out-of-control RunAI = %d, want 1 (no selection)", strike.doAttackCalls)
			}
			if err := ai.AttackFinished(); err != nil {
				t.Fatalf("AttackFinished() error = %v, want nil", err)
			}
			want := 1
			if tc.swing {
				want = 2
			}
			if strike.doAttackCalls != want {
				t.Fatalf("DoAttack calls after AttackFinished = %d, want %d", strike.doAttackCalls, want)
			}
			if got := ai.CurrentIntention(); got != IntentionAttack {
				t.Fatalf("CurrentIntention() = %v, want %v kept", got, IntentionAttack)
			}
		})
	}
}

func TestAttackableThinkDropsCurrentAttackWhenTargetMovesBeyond1500(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	strike := &recordingAttack{canAttack: true}
	ai := NewAttackable(owner, &recordingMove{}, strike)
	addAttackHate(ai, target, 0, 20)
	ai.RunAI()
	if strike.target != target {
		t.Fatalf("RunAI attacked = %v, want target", strike.target)
	}

	target.x = 2000
	strike.target = nil
	ai.Think()

	if strike.target != nil {
		t.Fatalf("second Think attacked = %v, want none after distance prune", strike.target)
	}
	if got := ai.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want %v", got, IntentionIdle)
	}
	if ai.Desires().Has(&Desire{Kind: IntentionAttack, FinalTarget: target}) {
		t.Fatal("ATTACK desire still queued after target moved beyond 1500")
	}
}

func TestAttackableAITickRefreshesStaleThreatAndHate(t *testing.T) {
	owner := actor(1)
	lost := actor(2)
	dead := actor(3)
	dead.alikeDead = true
	kept := actor(4)
	owner.known = map[int32]bool{lost.ObjectID(): false, dead.ObjectID(): true, kept.ObjectID(): true}
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	addAttackHate(ai, lost, 7, 70)
	addAttackHate(ai, dead, 8, 80)
	addAttackHate(ai, kept, 9, 90)
	ai.AddHate(lost, 700)
	ai.AddHate(dead, 800)
	ai.AddHate(kept, 900)

	ai.Tick()
	ai.Tick()
	ai.Tick()

	if _, ok := ai.Threats().Get(lost); ok {
		t.Fatal("lost threat entry still present after refresh")
	}
	gotDead, ok := ai.Threats().Get(dead)
	if !ok {
		t.Fatal("dead threat entry was dropped, want damage preserved")
	}
	if gotDead.Hate != -6.6 || gotDead.Damage != 8 {
		t.Fatalf("dead threat entry = %+v, want hate refreshed then decayed and damage preserved", gotDead)
	}
	if got := ai.Threats().Hate(kept); math.Abs(got-83.4) > 0.000001 {
		t.Fatalf("kept threat hate = %v, want decay after refresh", got)
	}
	if got := ai.Hates().Hate(lost); got != 0 {
		t.Fatalf("lost hate entry = %v, want removed", got)
	}
	if got := ai.Hates().Hate(dead); got != 0 {
		t.Fatalf("dead hate entry = %v, want removed", got)
	}
	if got := ai.Hates().Hate(kept); got != 900 {
		t.Fatalf("kept hate entry = %v, want unchanged", got)
	}
}

func armOOTSweep(ai *Attackable, owner *fakeActor) {
	owner.inTerritory = false
	ai.Arrived()
}

// TestAttackableAITickClearsStaleThreatOutOfTerritory: a threat entry
// whose last damage is at least staleThreatAge old gets its hate stopped
// and its queued attack desire dropped on the first due firing after an
// out-of-territory arrival arms the sweep.
func TestAttackableAITickClearsStaleThreatOutOfTerritory(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	addAttackHate(ai, target, 0, 20)

	start := time.Now()
	ai.now = func() time.Time { return start }
	armOOTSweep(ai, owner)
	tickOOTSweepDue(ai, start.Add(91*time.Second))

	if got := ai.Threats().Hate(target); got != 0 {
		t.Fatalf("hate after stale sweep = %v, want 0 (stopped)", got)
	}
	if _, ok := ai.Threats().Get(target); !ok {
		t.Fatal("threat entry dropped, want kept with hate stopped")
	}
	if got := ai.Desires().Len(); got != 0 {
		t.Fatalf("desires len = %d, want 0 (stale attack desire dropped)", got)
	}
}

// TestAttackableAITickKeepsFreshThreatOutOfTerritory confirms the sweep
// only touches entries whose last damage is stale; an attacker still
// dealing damage within staleThreatAge keeps its desire queued.
func TestAttackableAITickKeepsFreshThreatOutOfTerritory(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	addAttackHate(ai, target, 0, 20)

	start := time.Now()
	ai.now = func() time.Time { return start }
	armOOTSweep(ai, owner)
	tickOOTSweepDue(ai, start.Add(10*time.Second))

	if got := ai.Desires().Len(); got != 1 {
		t.Fatalf("desires len = %d, want 1 (fresh attack desire kept)", got)
	}
}

// TestAttackableAITickSkipsStaleSweepInTerritory confirms the sweep never
// runs while the owner is in its territory.
func TestAttackableAITickSkipsStaleSweepInTerritory(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	addAttackHate(ai, target, 0, 20)

	start := time.Now()
	ai.now = func() time.Time { return start }
	ai.Arrived()
	tickOOTSweepDue(ai, start.Add(91*time.Second))

	if got := ai.Desires().Len(); got != 1 {
		t.Fatalf("desires len = %d, want 1 (in-territory owner never runs the OOT sweep)", got)
	}
}

// TestAttackableAITickSkipsStaleSweepWithoutArrival confirms a stationary
// out-of-territory owner never arms the sweep, so stale hate is kept.
func TestAttackableAITickSkipsStaleSweepWithoutArrival(t *testing.T) {
	owner := actor(1)
	owner.inTerritory = false
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	addAttackHate(ai, target, 0, 1000)

	future := time.Now().Add(91 * time.Second)
	ai.now = func() time.Time { return future }
	for range 10 {
		ai.Tick()
	}

	if got := ai.Desires().Len(); got != 1 {
		t.Fatalf("desires len = %d, want 1 (no arrival means no OOT sweep)", got)
	}
}

// TestAttackableAITickOutOfTerritorySweepKeepsPhaseAcrossInTerritoryTicks
// confirms an in-territory Tick skips that firing but does not cancel the
// schedule, so a later out-of-territory Tick still sweeps.
func TestAttackableAITickOutOfTerritorySweepKeepsPhaseAcrossInTerritoryTicks(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	addAttackHate(ai, target, 0, 1000)

	start := time.Now()
	ai.now = func() time.Time { return start }
	armOOTSweep(ai, owner)

	owner.inTerritory = true
	tickOOTSweepDue(ai, start.Add(ootSweepInitialDelay))
	if got := ai.Desires().Len(); got != 1 {
		t.Fatalf("desires len after in-territory firing = %d, want 1", got)
	}

	owner.inTerritory = false
	tickOOTSweepDue(ai, start.Add(91*time.Second))

	if got := ai.Desires().Len(); got != 0 {
		t.Fatalf("desires len = %d, want 0 (phase kept across in-territory tick)", got)
	}
}

// TestAttackableAIArrivedInTerritoryCancelsOOTSweep confirms only an
// in-territory arrival cancels the schedule; later out-of-territory ticks
// without a new arrival do not sweep.
func TestAttackableAIArrivedInTerritoryCancelsOOTSweep(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	addAttackHate(ai, target, 0, 1000)

	start := time.Now()
	ai.now = func() time.Time { return start }
	armOOTSweep(ai, owner)

	owner.inTerritory = true
	ai.Arrived()
	owner.inTerritory = false
	tickOOTSweepDue(ai, start.Add(91*time.Second))

	if got := ai.Desires().Len(); got != 1 {
		t.Fatalf("desires len = %d, want 1 (in-territory arrival cancelled the sweep)", got)
	}
}

func TestAttackableAIAddDefaultHateUsesTerritoryOpeningValue(t *testing.T) {
	owner := actor(1)
	first := actor(2)
	second := actor(3)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	ai.AddDefaultHate(first)
	ai.AddDefaultHate(second)

	if got := ai.Hates().Hate(first); got != 300 {
		t.Fatalf("first default hate = %v, want 300", got)
	}
	if got := ai.Hates().Hate(second); got != 100 {
		t.Fatalf("second default hate = %v, want 100", got)
	}
}

func TestAttackableAIAddDefaultHateOutsideTerritoryUsesBaseValue(t *testing.T) {
	owner := actor(1)
	owner.inTerritory = false
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	ai.AddDefaultHate(target)

	if got := ai.Hates().Hate(target); got != 100 {
		t.Fatalf("default hate outside territory = %v, want 100", got)
	}
}
