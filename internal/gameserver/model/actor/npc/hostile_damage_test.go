package npc

import (
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// ---- from hostile_damage_effects_test.go ----
// TestHostileReduceHPStopsSleepAndImmobileUntilAttackedEffects mirrors
// NpcStatus.reduceHp's inherited CreatureStatus.reduceHp non-DOT block
// (CreatureStatus.java:228-248): a non-DOT hit stops SLEEP and
// IMMOBILE_UNTIL_ATTACKED.
func TestHostileReduceHPStopsSleepAndImmobileUntilAttackedEffects(t *testing.T) {
	hostile := newTestHostile(t, &hostileMove{}, &hostileAttack{})
	addHostileEffect(t, hostile, "Sleep")
	addHostileEffect(t, hostile, "ImmobileUntilAttacked")

	hostile.ReduceHP(10, nil, skill.Definition{})

	if hostile.Sleeping() {
		t.Fatal("Sleeping() = true after ReduceHP, want the sleep effect stopped")
	}
	if hostile.ImmobileUntilAttacked() {
		t.Fatal("ImmobileUntilAttacked() = true after ReduceHP, want the effect stopped")
	}
}

func TestHostileMovementDisabledTracksCrowdControl(t *testing.T) {
	h := newTestHostile(t, &hostileMove{}, &hostileAttack{})
	h.Instance.Template.CanMove = true
	if h.MovementDisabled() {
		t.Fatal("MovementDisabled() = true for a mobile NPC with no crowd-control")
	}

	h.Instance.Template.CanMove = false
	if !h.MovementDisabled() {
		t.Fatal("MovementDisabled() = false for canMove=false")
	}
	h.Instance.Template.CanMove = true

	root := addHostileEffect(t, h, "Root")
	if !h.MovementDisabled() {
		t.Fatal("MovementDisabled() = false while rooted")
	}
	h.EffectList().Remove(root)
	if h.MovementDisabled() {
		t.Fatal("MovementDisabled() = true after root was removed")
	}

	addHostileEffect(t, h, "Fear")
	if h.MovementDisabled() {
		t.Fatal("MovementDisabled() = true while afraid; fear does not disable movement")
	}
}

// TestHostileAttackDisabledMatchesReferenceTerms pins the NPC attack gate to
// the reference's creature union (Creature.isAttackingDisabled minus flying,
// which an NPC never is): stun, immobile-until-attacked, sleep, paralysis,
// fear and death disable attacking; root and teleport gate movement only.
func TestHostileAttackDisabledMatchesReferenceTerms(t *testing.T) {
	effects := []struct {
		name     string
		disables bool
	}{
		{"Stun", true},
		{"ImmobileUntilAttacked", true},
		{"Sleep", true},
		{"Paralyze", true},
		{"Fear", true},
		{"Root", false},
	}
	for _, tt := range effects {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHostile(t, &hostileMove{}, &hostileAttack{})
			if h.AttackDisabled() {
				t.Fatal("AttackDisabled() = true with no effect active")
			}
			e := addHostileEffect(t, h, tt.name)
			if got := h.AttackDisabled(); got != tt.disables {
				t.Fatalf("AttackDisabled() = %v with %s active, want %v", got, tt.name, tt.disables)
			}
			h.EffectList().Remove(e)
			if h.AttackDisabled() {
				t.Fatalf("AttackDisabled() = true after %s was removed", tt.name)
			}
		})
	}

	t.Run("teleporting", func(t *testing.T) {
		h := newTestHostile(t, &hostileMove{}, &hostileAttack{})
		h.SetTeleporting(true)
		if !h.MovementDisabled() {
			t.Fatal("MovementDisabled() = false while teleporting")
		}
		if h.AttackDisabled() {
			t.Fatal("AttackDisabled() = true while only teleporting")
		}
	})
}

// newSurvivingHostile returns a monster at full HP that a 10-point hit
// leaves alive: a dying NPC loses its effects, which would hide what a hit
// alone does to them.
func newSurvivingHostile(t *testing.T) *Hostile {
	t.Helper()
	h := newCombatHostile(t, 101, &Template{ID: 9001, Type: "Monster", HPMax: 500})
	h.SetHP(h.MaxHPValue())
	return h
}

// TestHostileReduceHPByDOTLeavesEffectsAloneOnRealDOTTick mirrors the
// !isDOT gate on CreatureStatus.reduceHp's whole SLEEP/IMMOBILE/STUN block:
// NpcStatus has no PlayerStatus-style override, so a real DOT tick
// (isDOT=true) skips the block entirely.
func TestHostileReduceHPByDOTLeavesEffectsAloneOnRealDOTTick(t *testing.T) {
	hostile := newSurvivingHostile(t)
	addHostileEffect(t, hostile, "Sleep")
	addHostileEffect(t, hostile, "ImmobileUntilAttacked")
	addHostileEffect(t, hostile, "Stun")
	hostile.SetRollSource(func(int) int { return 0 })

	hostile.ReduceHPByDOT(10, nil, true)

	if !hostile.Sleeping() {
		t.Fatal("Sleeping() = false after a DOT tick, want the sleep effect untouched")
	}
	if !hostile.ImmobileUntilAttacked() {
		t.Fatal("ImmobileUntilAttacked() = false after a DOT tick, want the effect untouched")
	}
	if !hostile.Stunned() {
		t.Fatal("Stunned() = false after a DOT tick, want the stun effect untouched")
	}
}

// TestHostileReduceHPByDOTAppliesEffectsWhenNotARealDOTTick mirrors
// drowning's WaterTaskManager.reduceCurrentHp(hp, player, false, false,
// null) call: isDOT=false periodic damage still runs the block.
func TestHostileReduceHPByDOTAppliesEffectsWhenNotARealDOTTick(t *testing.T) {
	hostile := newTestHostile(t, &hostileMove{}, &hostileAttack{})
	addHostileEffect(t, hostile, "Sleep")

	hostile.ReduceHPByDOT(10, nil, false)

	if hostile.Sleeping() {
		t.Fatal("Sleeping() = true after a non-DOT periodic hit, want the sleep effect stopped")
	}
}

// TestHostileTakeDamageStopsSleepAndImmobileUntilAttackedEffects mirrors the
// melee auto-attack path (CreatureAttack.java:263 -> NpcStatus.reduceHp),
// which is always non-DOT.
func TestHostileTakeDamageStopsSleepAndImmobileUntilAttackedEffects(t *testing.T) {
	hostile := newTestHostile(t, &hostileMove{}, &hostileAttack{})
	addHostileEffect(t, hostile, "Sleep")
	addHostileEffect(t, hostile, "ImmobileUntilAttacked")

	hostile.TakeDamage(10, nil)

	if hostile.Sleeping() {
		t.Fatal("Sleeping() = true after TakeDamage, want the sleep effect stopped")
	}
	if hostile.ImmobileUntilAttacked() {
		t.Fatal("ImmobileUntilAttacked() = true after TakeDamage, want the effect stopped")
	}
}

// TestHostileReduceHPBreaksStunOnOneInTenRollForNonDOTDamage mirrors
// !isDOT && isStunned() && Rnd.get(10) == 0.
func TestHostileReduceHPBreaksStunOnOneInTenRollForNonDOTDamage(t *testing.T) {
	tests := []struct {
		name      string
		roll      int
		wantAfter bool
	}{
		{"winning roll breaks stun", 0, false},
		{"losing roll leaves stun active", 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hostile := newSurvivingHostile(t)
			addHostileEffect(t, hostile, "Stun")
			hostile.SetRollSource(func(int) int { return tt.roll })

			hostile.ReduceHP(10, nil, skill.Definition{})

			if got := hostile.Stunned(); got != tt.wantAfter {
				t.Fatalf("Stunned() = %v, want %v", got, tt.wantAfter)
			}
		})
	}
}

// ---- from hostile_dot_test.go ----
var (
	_ interface {
		Dead() bool
		HP() float64
		ReduceHPByDOT(float64, effect.Actor, bool)
	} = (*Hostile)(nil)
	_ interface {
		Dead() bool
		MPValue() float64
		ReduceMP(float64) float64
	} = (*Hostile)(nil)
)

func TestDamageOverTimeEffectTargetsHostile(t *testing.T) {
	h := newCombatHostile(t, 1, &Template{HPMax: 100, MPMax: 50})
	e, err := effect.New(effect.Skill{ID: 1}, skill.EffectTemplate{Name: "DamOverTime", Value: 4})
	if err != nil {
		t.Fatalf("effect.New() error: %v", err)
	}
	e.Effected = h
	if !e.ActionTime() {
		t.Fatal("ActionTime() = false, want true")
	}
	if got, want := h.HP(), h.MaxHPValue()-4; got != want {
		t.Fatalf("HP() = %v, want %v", got, want)
	}
}

// TestDamageOverTimeEffectRecordsFlatHateFromAttackDesire updates Finding 1
// of the #1088 closed-PR review for #2340: ReduceHPByDOT's own
// addDamageHate(attacker, damage, 0) call (matching Npc.reduceCurrentHp,
// Npc.java:390-395) still contributes zero hate — damage-only bookkeeping —
// but the DOT registerHit path also queues an attack Desire via
// attackedHateWeight (see TestDamageOverTimeEffectQueuesAttackDesire), and
// that Desire now feeds the same weight into the threat table
// (addAttackDesireWithMove). The caster here is a non-Playable Hostile, so
// attackedHateWeight falls back to its flat pre-existing weight (#185, M10 —
// the individual AI scripts that would derive a real per-caster formula
// aren't ported). A DOT-only attacker therefore does reach positive hate and
// can become most-hated, contrary to the #1088 review's now-corrected
// assumption.
func TestDamageOverTimeEffectRecordsFlatHateFromAttackDesire(t *testing.T) {
	h := newCombatHostile(t, 1, &Template{HPMax: 100, MPMax: 50})
	caster := newCombatHostile(t, 2, &Template{HPMax: 100, MPMax: 50})
	e, err := effect.New(effect.Skill{ID: 1}, skill.EffectTemplate{Name: "DamOverTime", Value: 4})
	if err != nil {
		t.Fatalf("effect.New() error: %v", err)
	}
	e.Effected = h
	e.Effector = caster
	if !e.ActionTime() {
		t.Fatal("ActionTime() = false, want true")
	}
	threat, ok := h.AI().Threats().Get(caster)
	if !ok {
		t.Fatal("Threats().Get(caster) ok = false, want caster recorded")
	}
	if threat.Damage != 4 {
		t.Fatalf("threat.Damage = %v, want 4", threat.Damage)
	}
	if threat.Hate != flatAttackedHateWeight {
		t.Fatalf("threat.Hate = %v, want flat fallback weight %v", threat.Hate, flatAttackedHateWeight)
	}
	if most, ok := h.AI().Threats().MostHated(); !ok || most.Attacker != caster {
		t.Fatalf("MostHated() = (%+v, %v), want DOT-only caster ranked most-hated", most, ok)
	}
}

// TestCombatDamageHateScalesWithDamageForPlayableAttacker pins #2340's
// review finding that a flat per-hit weight breaks damage-proportional
// ranking: for a Playable attacker, attackedHateWeight must scale with
// damage (Warrior.onAttacked's core term, Warrior.java:394-396:
// damage/(level+7)*100), not apply the same flat weight regardless of hit
// size.
func TestCombatDamageHateScalesWithDamageForPlayableAttacker(t *testing.T) {
	h := newCombatHostile(t, 1, &Template{HPMax: 1000, MPMax: 50, Level: 20})
	small := &hostileTarget{id: 2, playable: true}
	big := &hostileTarget{id: 3, playable: true}

	h.AI().AddCombatDamageHate(small, 10, h.attackedHateWeight(small, 10))
	h.AI().AddCombatDamageHate(big, 300, h.attackedHateWeight(big, 300))

	smallHate := h.AI().Threats().Hate(small)
	bigHate := h.AI().Threats().Hate(big)
	if smallHate <= 0 || bigHate <= smallHate*10 {
		t.Fatalf("hate small=%v big=%v, want big roughly proportional to its 30x damage, not equal/flat", smallHate, bigHate)
	}
	wantBig := 300.0 / (20 + 7) * 100
	if math.Abs(bigHate-wantBig) > 0.001 {
		t.Fatalf("big hate = %v, want %v (damage/(level+7)*100)", bigHate, wantBig)
	}
}

// TestMeleeAttackerOutranksDOTOnlyAttacker is #2340's own oracle scenario
// (b), flagged by review as missing: a higher-damage melee (or single big
// hit) attacker must outrank a low-damage-per-tick DOT-only attacker for
// MostHated, matching Java's damage-proportional onAttacked weighting
// instead of ranking by hit count.
func TestMeleeAttackerOutranksDOTOnlyAttacker(t *testing.T) {
	h := newCombatHostile(t, 1, &Template{HPMax: 10000, MPMax: 50, Level: 20})
	melee := &hostileTarget{id: 2, playable: true}
	dotCaster := &hostileTarget{id: 3, playable: true}

	h.ReduceHP(300, melee, skill.Definition{})
	for i := 0; i < 3; i++ {
		h.ReduceHPByDOT(4, dotCaster, true)
	}

	most, ok := h.AI().Threats().MostHated()
	if !ok || most.Attacker != melee {
		t.Fatalf("MostHated() = (%+v, %v), want the higher-damage melee attacker, not the DOT-only ticker", most, ok)
	}
}

// TestAggressionNotificationScalesLikeCombatHate pins the review's scale-
// mismatch finding on PR #2346: NotifyAggression must route power through
// the same attackedHateWeight scaling a real hit uses, matching
// AttackableAI.onEvtAggression (AttackableAI.java:119-123), which feeds the
// AGGRESSION event's aggro into the identical per-script onAttacked chain as
// a hit's damage. Level-20 mob, a 300-damage DD hit (hate ≈ 300/27*100 ≈
// 1111) vs a tank's power-500 AGGDEBUFF notification (hate ≈ 500/27*100 ≈
// 1852) — the tank must outrank the DD, not merely match raw power against
// scaled damage.
func TestAggressionNotificationScalesLikeCombatHate(t *testing.T) {
	h := newCombatHostile(t, 1, &Template{HPMax: 10000, MPMax: 50, Level: 20})
	dd := &hostileTarget{id: 2, playable: true}
	tank := &hostileTarget{id: 3, playable: true}

	h.ReduceHP(300, dd, skill.Definition{})
	h.NotifyAggression(tank, 500)

	most, ok := h.AI().Threats().MostHated()
	if !ok || most.Attacker != tank {
		t.Fatalf("MostHated() = (%+v, %v), want the scaled-up AGGDEBUFF tank, not the DD", most, ok)
	}
}

// TestSubOneDOTTickContributesZeroHate pins the review's truncation finding
// on PR #2346: Npc.reduceCurrentHp truncates damage to an int before
// dispatching it to the per-script onAttacked (Npc.java:403,
// "quest.onAttacked(this, attacker, (int) damage, skill)"), so a sub-1 DOT
// tick contributes zero ATTACKED hate in Java — attackedHateWeight must
// truncate the same way, not turn a 0.6 tick into positive hate.
func TestSubOneDOTTickContributesZeroHate(t *testing.T) {
	h := newCombatHostile(t, 1, &Template{HPMax: 10000, MPMax: 50, Level: 20})
	dotCaster := &hostileTarget{id: 2, playable: true}

	h.ReduceHPByDOT(0.6, dotCaster, true)

	if _, ok := h.AI().Threats().MostHated(); ok {
		t.Fatal("MostHated() ok = true, want no ranked attacker from a truncated-to-zero DOT tick")
	}
	if got := h.AI().Threats().Hate(dotCaster); got != 0 {
		t.Fatalf("hate = %v, want 0 (truncated damage/(level+7)*100 = 0)", got)
	}
}

func TestDamageOverTimeEffectQueuesAttackDesire(t *testing.T) {
	h := newCombatHostile(t, 1, &Template{HPMax: 100, MPMax: 50})
	caster := newCombatHostile(t, 2, &Template{HPMax: 100, MPMax: 50})
	e, err := effect.New(effect.Skill{ID: 1}, skill.EffectTemplate{Name: "DamOverTime", Value: 4})
	if err != nil {
		t.Fatalf("effect.New() error: %v", err)
	}
	e.Effected = h
	e.Effector = caster
	if !e.ActionTime() {
		t.Fatal("ActionTime() = false, want true")
	}
	desire, ok := h.AI().Desires().Peek()
	if !ok {
		t.Fatal("Desires().Peek() ok = false, want DOT attack desire")
	}
	if desire.FinalTarget != caster || desire.Weight != 200 {
		t.Fatalf("DOT desire = (%v, %v), want (%v, 200)", desire.FinalTarget, desire.Weight, caster)
	}
}

func TestManaDamageOverTimeEffectTargetsHostile(t *testing.T) {
	h := newCombatHostile(t, 1, &Template{HPMax: 100, MPMax: 50})
	e, err := effect.New(effect.Skill{ID: 1}, skill.EffectTemplate{Name: "ManaDamOverTime", Value: 4})
	if err != nil {
		t.Fatalf("effect.New() error: %v", err)
	}
	e.Effected = h
	if !e.ActionTime() {
		t.Fatal("ActionTime() = false, want true")
	}
	if got, want := h.MPValue(), 46.0; got != want {
		t.Fatalf("MPValue() = %v, want %v", got, want)
	}
}

// TestHostileVitalsMutatorsReportHPChangeOncePerChange pins that every HP/MP
// restore or MP payment that changes an NPC's vitals offers its targeters'
// health bar one refresh, and one that changes nothing stays silent.
// Regeneration changes both resources but offers one refresh for the pair.
func TestHostileVitalsMutatorsReportHPChangeOncePerChange(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*Hostile)
		apply func(*Hostile) float64
		want  int
	}{
		{"add hp", func(h *Hostile) { h.SetHP(h.MaxHPValue() / 2) }, func(h *Hostile) float64 { return h.AddHP(10) }, 1},
		{"add hp at max", nil, func(h *Hostile) float64 { return h.AddHP(10) }, 0},
		{"add mp", func(h *Hostile) { h.reduceMP(h.MPValue() / 2) }, func(h *Hostile) float64 { return h.AddMP(10) }, 1},
		{"add mp at max", nil, func(h *Hostile) float64 { return h.AddMP(10) }, 0},
		{"reduce mp", nil, func(h *Hostile) float64 { return h.ReduceMP(10) }, 1},
		{"reduce empty mp", func(h *Hostile) { h.reduceMP(h.MPValue()) }, func(h *Hostile) float64 { return h.ReduceMP(10) }, 0},
		{"regen both", func(h *Hostile) { h.SetHP(h.MaxHPValue() / 2); h.reduceMP(h.MPValue() / 2) }, func(h *Hostile) float64 { h.TickRegen(); return 1 }, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hostile := newTestHostile(t, &hostileMove{}, &hostileAttack{})
			hostile.Instance.Template.HPMax = 1000
			hostile.Instance.Template.MPMax = 100
			hostile.SetHP(hostile.MaxHPValue())
			hostile.addMP(hostile.MaxMPValue())
			if tc.setup != nil {
				tc.setup(hostile)
			}
			rec := &event.Recorder{}
			hostile.Attach(Runtime{Sink: rec})

			applied := tc.apply(hostile)

			if (applied > 0) != (tc.want > 0) {
				t.Fatalf("applied = %v, want a change only when %d refreshes are expected", applied, tc.want)
			}
			if got := event.Count[event.HPChanged](rec); got != tc.want {
				t.Fatalf("HP change reports = %d, want %d", got, tc.want)
			}
		})
	}
}
