package npc

import (
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

// TestDamageOverTimeEffectRecordsDamageWithoutHate: a damage-over-time
// tick on an NPC enters its threat table at zero hate and queues nothing;
// the attacked hooks of its behavior decide any reaction.
func TestDamageOverTimeEffectRecordsDamageWithoutHate(t *testing.T) {
	h := newCombatHostile(t, 1, &Template{HPMax: 100, MPMax: 50})
	caster := newCombatHostile(t, 2, &Template{HPMax: 100, MPMax: 50})
	spawnPartyWorld(t, h, caster)
	e, err := effect.New(effect.Skill{ID: 1}, skill.EffectTemplate{Name: "DamOverTime", Value: 4})
	if err != nil {
		t.Fatalf("effect.New() error: %v", err)
	}
	e.Effected = h
	e.Effector = caster
	if !e.ActionTime() {
		t.Fatal("ActionTime() = false, want true")
	}
	if d, ok := h.AI().Desires().Peek(); ok {
		t.Fatalf("Desires().Peek() = %+v, want nothing queued", d)
	}
	threat, ok := h.AI().Threats().Get(caster)
	if !ok || threat.Damage != 4 || threat.Hate != 0 {
		t.Fatalf("threat = (%+v, %v), want damage 4 at zero hate", threat, ok)
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
