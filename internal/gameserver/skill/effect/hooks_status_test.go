package effect

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

type spoilFakeCaster struct {
	id    int32
	level int
}

func (c *spoilFakeCaster) ObjectID() int32 { return c.id }

func (c *spoilFakeCaster) Level() int { return c.level }

type spoilFakeTarget struct {
	dead  bool
	level int
	pool  *item.SpoilPool
}

func (t *spoilFakeTarget) Dead() bool { return t.dead }

func (t *spoilFakeTarget) Level() int { return t.level }

func (t *spoilFakeTarget) SpoilPool() *item.SpoilPool { return t.pool }

func TestSpoilEffectMarksLiveUnspoiledTargetOnSuccess(t *testing.T) {
	// A caster far above the target's level drives the resist rate to its
	// floor, making success near-certain; repeated trials tolerate the
	// residual chance without asserting a literal 100%.
	const trials = 100
	marked := 0
	for i := 0; i < trials; i++ {
		caster := &spoilFakeCaster{id: 77, level: 80}
		target := &spoilFakeTarget{level: 1, pool: &item.SpoilPool{}}
		e, err := New(Skill{MagicLevel: 80}, modelskill.EffectTemplate{Name: "Spoil"})
		if err != nil {
			t.Fatalf("New() error: %v", err)
		}
		e.Effector = caster
		e.Effected = target

		if !e.OnStart(e) {
			t.Fatal("spoil effect start rejected a valid attempt")
		}
		if target.pool.IsSpoiler(77) {
			marked++
		}
	}
	if marked == 0 {
		t.Fatal("target was never marked spoiled across repeated trials, want at least one success")
	}
}

func TestSpoilEffectRejectsDeadOrAlreadySpoiledOrWrongActorTypes(t *testing.T) {
	e, err := New(Skill{MagicLevel: 80}, modelskill.EffectTemplate{Name: "Spoil"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	// Effector missing the caster surface entirely.
	e.Effector = &liveEffectTarget{}
	e.Effected = &spoilFakeTarget{level: 1, pool: &item.SpoilPool{}}
	if e.OnStart(e) {
		t.Error("spoil effect started with an effector lacking caster identity")
	}

	caster := &spoilFakeCaster{id: 5, level: 80}

	// Effected missing the spoil-pool surface entirely.
	e.Effector = caster
	e.Effected = &liveEffectTarget{}
	if e.OnStart(e) {
		t.Error("spoil effect started against a target with no spoil pool")
	}

	// Dead target.
	dead := &spoilFakeTarget{dead: true, level: 1, pool: &item.SpoilPool{}}
	e.Effected = dead
	if e.OnStart(e) {
		t.Error("spoil effect started against a dead target")
	}

	// Already spoiled target.
	spoiled := &spoilFakeTarget{level: 1, pool: &item.SpoilPool{}}
	spoiled.pool.Mark(999)
	e.Effected = spoiled
	if e.OnStart(e) {
		t.Error("spoil effect started against an already-spoiled target")
	}
	if !spoiled.pool.IsSpoiler(999) {
		t.Error("an already-spoiled target's existing spoiler must not be overwritten")
	}
}

func TestRelaxEffectSitsOnStartAndDrainsMpWhileSeatedAndNotFull(t *testing.T) {
	target := &liveEffectTarget{standing: true, mp: 10}
	e, err := New(Skill{Toggle: true}, modelskill.EffectTemplate{Name: "Relax", Value: 2})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	if e.Flag != flagRelaxing {
		t.Fatalf("Flag = %v, want flagRelaxing", e.Flag)
	}
	if !e.OnStart(e) {
		t.Fatal("relax effect start rejected a valid target")
	}
	if target.standing {
		t.Fatal("relax effect start must sit its target down")
	}

	if !e.ActionTime() {
		t.Fatal("relax effect action tick ended while seated with MP and HP available")
	}
	if target.mp != 8 {
		t.Fatalf("target mp = %v, want 8", target.mp)
	}
	// EffectRelax.java:54 drains MP through the same reduceMp/setMp chain as
	// EffectManaDamOverTime (CreatureStatus.java:338-355, 274-306), whose
	// Player override unconditionally includes CUR_MP in the broadcast
	// (PlayerStatus.java:408-416) — this tick must broadcast too.
	if target.mpBroadcasts != 1 {
		t.Fatalf("mp broadcasts = %d, want 1", target.mpBroadcasts)
	}
}

func TestRelaxEffectActionEndsWhenStandingOrHpFullOrLackMp(t *testing.T) {
	tests := []struct {
		name   string
		target *liveEffectTarget
	}{
		{"standing", &liveEffectTarget{standing: true, mp: 10}},
		{"hp full", &liveEffectTarget{standing: false, hpFull: true, mp: 10}},
		{"lacks mp", &liveEffectTarget{standing: false, mp: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := New(Skill{Toggle: true}, modelskill.EffectTemplate{Name: "Relax", Value: 2})
			if err != nil {
				t.Fatalf("New() error: %v", err)
			}
			e.Effected = tt.target
			if e.ActionTime() {
				t.Fatal("relax effect action tick continued, want it to end")
			}
		})
	}
}

func TestChameleonRestEffectGatesActionOnContSkillTypeAndSitting(t *testing.T) {
	target := &liveEffectTarget{standing: true, mp: 10}
	e, err := New(Skill{Toggle: true, SkillType: "CONT"}, modelskill.EffectTemplate{Name: "ChameleonRest", Value: 6})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	if want := FlagSilentMove | flagRelaxing; e.Flag != want {
		t.Fatalf("Flag = %v, want %v", e.Flag, want)
	}
	if !e.OnStart(e) {
		t.Fatal("chameleon rest effect start rejected a valid target")
	}
	if target.standing {
		t.Fatal("chameleon rest effect start must sit its target down")
	}

	if !e.ActionTime() {
		t.Fatal("chameleon rest effect action tick ended while seated on a CONT skill")
	}
	if target.mp != 4 {
		t.Fatalf("target mp = %v, want 4", target.mp)
	}

	nonCont, err := New(Skill{Toggle: true, SkillType: "BUFF"}, modelskill.EffectTemplate{Name: "ChameleonRest", Value: 6})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	nonCont.Effected = &liveEffectTarget{standing: false, mp: 10}
	if nonCont.ActionTime() {
		t.Fatal("chameleon rest effect action tick continued on a non-CONT skill, want it to end")
	}

	standingTarget := &liveEffectTarget{standing: true, mp: 10}
	e.Effected = standingTarget
	if e.ActionTime() {
		t.Fatal("chameleon rest effect action tick continued while standing, want it to end")
	}
}

func TestFakeDeathEffectSitsOnStartAndDrainsMpEachTick(t *testing.T) {
	target := &liveEffectTarget{standing: true, mp: 100}
	e, err := New(Skill{Toggle: true}, modelskill.EffectTemplate{Name: "FakeDeath", Value: 35})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	if e.Flag != FlagFakeDeath {
		t.Fatalf("Flag = %v, want FlagFakeDeath", e.Flag)
	}
	if !e.OnStart(e) {
		t.Fatal("fake death effect start rejected a valid target")
	}
	if target.standing {
		t.Fatal("fake death effect start must sit its target down")
	}

	if !e.ActionTime() {
		t.Fatal("fake death effect action tick ended with MP available")
	}
	if target.mp != 65 {
		t.Fatalf("target mp = %v, want 65", target.mp)
	}
	// EffectFakeDeath.java:51 drains MP through the same reduceMp/setMp
	// chain as EffectManaDamOverTime and EffectRelax; the shared manaDrainTick
	// helper must broadcast it too (see the relax test for the full chain).
	if target.mpBroadcasts != 1 {
		t.Fatalf("mp broadcasts = %d, want 1", target.mpBroadcasts)
	}
}

func TestFakeDeathEffectActionEndsWhenDeadOrLackMp(t *testing.T) {
	tests := []struct {
		name   string
		target *liveEffectTarget
	}{
		{"dead", &liveEffectTarget{dead: true, mp: 100}},
		{"lacks mp", &liveEffectTarget{mp: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := New(Skill{Toggle: true}, modelskill.EffectTemplate{Name: "FakeDeath", Value: 35})
			if err != nil {
				t.Fatalf("New() error: %v", err)
			}
			e.Effected = tt.target
			if e.ActionTime() {
				t.Fatal("fake death effect action tick continued, want it to end")
			}
		})
	}
}

func TestFakeDeathEffectExitStandsUpAndStartsRecentFakeDeathGrace(t *testing.T) {
	target := &liveEffectTarget{standing: false, mp: 10}
	e, err := New(Skill{Toggle: true}, modelskill.EffectTemplate{Name: "FakeDeath", Value: 35})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	e.OnExit(e)
	if !target.standing {
		t.Fatal("fake death effect exit must stand its target back up")
	}
	if !target.recentFakeDeath {
		t.Fatal("fake death effect exit must mark the recent-fake-death grace period")
	}
}

// TestBetrayEffectAttacksSummonOwnerAndFollowsOnExit moved to
// hooks_status_player_test.go (package effect_test): it needs a real
// attackable.Combatant owner, and *player.Character already implements
// that interface, so it uses the real type instead of a hand-rolled fake.
// See docs/agents/test-strategy.md.

// fakeCombatant is a minimal attackable.Combatant used as the redirect
// candidate/attacker argument in the hostileEffectTarget-based tests below.
// hostileEffectTarget is itself an internal-package-only test double (no
// real production equivalent conveniently constructible here), so moving
// these particular tests to use a real *player.Character candidate would
// require duplicating hostileEffectTarget across the package boundary for
// no drift-risk reduction (fakeCombatant has no logic to drift: it's a
// pure ID/false-false stub). Left as-is per the "keep, document why" rule
// in docs/agents/test-strategy.md.

type fakeCombatant struct {
	id int32
}

func (f *fakeCombatant) ObjectID() int32  { return f.id }
func (f *fakeCombatant) SiegeGuard() bool { return false }
func (f *fakeCombatant) AlikeDead() bool  { return false }

// hostileEffectTarget is a minimal actor implementing only the interfaces
// a hostility-redirect effect needs, standing in for the npc package's
// live wiring.

type hostileEffectTarget struct {
	events       []string
	level        int
	monsterKind  bool
	candidate    attackable.Combatant
	hasCandidate bool
}

func (t *hostileEffectTarget) Level() int { return t.level }

func (t *hostileEffectTarget) MonsterKind() bool { return t.monsterKind }

func (t *hostileEffectTarget) AddDamageHate(attacker attackable.Combatant, damage, hate float64) {
	t.events = append(t.events, fmt.Sprintf("add-damage-hate:%d:%g:%g", attacker.ObjectID(), damage, hate))
}

func (t *hostileEffectTarget) RandomNearbyMonster(radius int) (attackable.Combatant, bool) {
	t.events = append(t.events, fmt.Sprintf("nearby-monster:%d", radius))
	return t.candidate, t.hasCandidate
}

func (t *hostileEffectTarget) RandomNearbyCombatant(radius int) (attackable.Combatant, bool) {
	t.events = append(t.events, fmt.Sprintf("nearby-combatant:%d", radius))
	return t.candidate, t.hasCandidate
}

func (t *hostileEffectTarget) StopMostHatedTarget() {
	t.events = append(t.events, "stop-most-hated")
}

func (t *hostileEffectTarget) RandomizeHate() bool {
	t.events = append(t.events, "randomize-hate")
	return t.hasCandidate
}

func (t *hostileEffectTarget) StopMove() {
	t.events = append(t.events, "stop-move")
}

func (t *hostileEffectTarget) UpdateAbnormalEffect() {
	t.events = append(t.events, "abnormal")
}

func TestDistrustEffectRaisesHateAgainstARandomNearbyMonster(t *testing.T) {
	target := &hostileEffectTarget{monsterKind: true, candidate: &fakeCombatant{id: 9}, hasCandidate: true}
	effector := &hostileEffectTarget{level: 40}
	e, err := New(Skill{}, modelskill.EffectTemplate{Name: "Distrust"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effector = effector
	e.Effected = target

	if !e.OnStart(e) {
		t.Fatal("distrust effect start rejected a valid Monster-family target")
	}
	if len(target.events) != 2 || target.events[0] != "nearby-monster:600" {
		t.Fatalf("events = %#v, want a nearby-monster search followed by an add-damage-hate call", target.events)
	}
}

func TestDistrustEffectRejectsNonMonsterTargetAndNoOpsWithNoCandidate(t *testing.T) {
	notMonster := &hostileEffectTarget{monsterKind: false}
	e, err := New(Skill{}, modelskill.EffectTemplate{Name: "Distrust"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = notMonster
	if e.OnStart(e) {
		t.Fatal("distrust effect started against a non-Monster-family target")
	}

	noCandidate := &hostileEffectTarget{monsterKind: true, hasCandidate: false}
	e2, err := New(Skill{}, modelskill.EffectTemplate{Name: "Distrust"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e2.Effected = noCandidate
	if !e2.OnStart(e2) {
		t.Fatal("distrust effect start rejected a Monster-family target with no nearby candidate, want success")
	}
	for _, evt := range noCandidate.events {
		if evt != "nearby-monster:600" {
			t.Fatalf("unexpected event %q with no candidate available", evt)
		}
	}
}

func TestConfusionEffectRedirectsNonPlayerTargetOntoARandomNearbyCombatant(t *testing.T) {
	target := &hostileEffectTarget{candidate: &fakeCombatant{id: 3}, hasCandidate: true}
	e, err := New(Skill{}, modelskill.EffectTemplate{Name: "Confusion"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	if !e.OnStart(e) {
		t.Fatal("confusion effect start rejected a valid non-player target")
	}
	want := []string{"stop-move", "abnormal", "nearby-combatant:1000", "add-damage-hate:3:0:2.147483647e+09"}
	if !reflect.DeepEqual(target.events, want) {
		t.Fatalf("events = %#v, want %#v", target.events, want)
	}

	e.OnExit(e)
	if got := target.events[len(target.events)-2:]; !reflect.DeepEqual(got, []string{"abnormal", "stop-most-hated"}) {
		t.Fatalf("exit events = %#v, want abnormal refresh followed by stop-most-hated", got)
	}
}

func TestRandomizeHateEffectDelegatesToTheThreatTableSwap(t *testing.T) {
	target := &hostileEffectTarget{hasCandidate: true}
	e, err := New(Skill{}, modelskill.EffectTemplate{Name: "RandomizeHate"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	if !e.OnStart(e) {
		t.Fatal("randomize-hate effect start rejected a valid Attackable target")
	}
	if want := []string{"randomize-hate"}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("events = %#v, want %#v", target.events, want)
	}
}

// TestRandomizeHateEffectRejectsATargetWithNoThreatTable moved to
// hooks_status_player_test.go (package effect_test): it needs a real
// non-RandomizeHate-capable target, and *player.Character already lacks
// that method, so it uses the real type instead of a hand-rolled fake.

func TestConfusionEffectLeavesAPlayerTargetEntirelyUntouched(t *testing.T) {
	target := &liveEffectTarget{isPlayer: true}
	e, err := New(Skill{}, modelskill.EffectTemplate{Name: "Confusion"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	if !e.OnStart(e) {
		t.Fatal("confusion effect start rejected a player target, want success as a no-op")
	}
	if len(target.events) != 0 {
		t.Fatalf("events = %#v, want none: a player target must never be redirected", target.events)
	}

	// The abnormal-effect refresh runs unconditionally on exit, even for a
	// player target — only the hate-table redirect and its cleanup are
	// player-exempt.
	e.OnExit(e)
	if want := []string{"abnormal"}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("exit events = %#v, want %#v", target.events, want)
	}
}

type growEffectTarget struct {
	events []string
	radius float64
}

func (t *growEffectTarget) CollisionRadius() float64 { return t.radius }

func (t *growEffectTarget) SetCollisionRadius(radius float64) {
	t.radius = radius
	t.events = append(t.events, fmt.Sprintf("set:%g", radius))
}

func (t *growEffectTarget) ResetCollisionRadius() {
	t.events = append(t.events, "reset")
}

func (t *growEffectTarget) StartAbnormalEffect(mask int) {
	t.events = append(t.events, fmt.Sprintf("start:%#x", mask))
}

func (t *growEffectTarget) StopAbnormalEffect(mask int) {
	t.events = append(t.events, fmt.Sprintf("stop:%#x", mask))
}

func (t *growEffectTarget) UpdateAbnormalEffect() {
	t.events = append(t.events, "abnormal")
}

func TestGrowEffectScalesCollisionRadiusAndRestoresOnExit(t *testing.T) {
	baseRadius := 9.0
	target := &growEffectTarget{radius: baseRadius}
	e, err := New(Skill{}, modelskill.EffectTemplate{Name: "Grow"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	if !e.OnStart(e) {
		t.Fatal("grow effect start rejected a valid Npc-shaped target")
	}
	want := baseRadius * growRadiusScale
	if target.radius != want {
		t.Fatalf("radius after start = %v, want %v", target.radius, want)
	}

	e.OnExit(e)
	if wantEvents := []string{fmt.Sprintf("set:%g", want), "start:0x10000", "abnormal", "reset", "stop:0x10000", "abnormal"}; !reflect.DeepEqual(target.events, wantEvents) {
		t.Fatalf("events = %#v, want %#v", target.events, wantEvents)
	}
}

func TestGrowEffectRejectsNonNpcTarget(t *testing.T) {
	target := &liveEffectTarget{}
	e, err := New(Skill{}, modelskill.EffectTemplate{Name: "Grow"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	if e.OnStart(e) {
		t.Fatal("grow effect started against a non-Npc-shaped target")
	}
}

// recoveryEffectTarget is a minimal player-shaped actor implementing only
// the death-penalty-level counter surface Recovery needs.

type recoveryEffectTarget struct {
	level int
}

func (t *recoveryEffectTarget) ReduceDeathPenaltyLevel() int {
	if t.level > 0 {
		t.level--
	}
	return t.level
}

func TestRecoveryEffectDecrementsAboveZero(t *testing.T) {
	target := &recoveryEffectTarget{level: 3}
	e, err := New(Skill{}, modelskill.EffectTemplate{Name: "Recovery"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	if !e.OnStart(e) {
		t.Fatal("recovery effect start rejected a valid player-shaped target")
	}
	if target.level != 2 {
		t.Fatalf("level after start = %d, want 2", target.level)
	}
}

func TestRecoveryEffectDecrementsToZeroIsNoFurtherOp(t *testing.T) {
	target := &recoveryEffectTarget{level: 0}
	e, err := New(Skill{}, modelskill.EffectTemplate{Name: "Recovery"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	if !e.OnStart(e) {
		t.Fatal("recovery effect start rejected a valid player-shaped target")
	}
	if target.level != 0 {
		t.Fatalf("level after start = %d, want 0", target.level)
	}
}

func TestRecoveryEffectRejectsNonPlayerTarget(t *testing.T) {
	target := &liveEffectTarget{}
	e, err := New(Skill{}, modelskill.EffectTemplate{Name: "Recovery"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	if e.OnStart(e) {
		t.Fatal("recovery effect started against a non-player-shaped target")
	}
}
