package npc

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

var (
	_ interface {
		Dead() bool
		HP() float64
		ReduceHPByDOT(float64, effect.Participant, bool)
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

// TestDamageOverTimeEffectRecordsZeroHateThreat pins Finding 1 of the #1088
// closed-PR review: ReduceHPByDOT must record the caster in the threat
// table at zero hate weight, matching Npc.reduceCurrentHp's unconditional
// addDamageHate(attacker, damage, 0) (Npc.java:390-395) — DOT feeds the
// AggroList's damage/timestamp bookkeeping even though it never raises
// hate above zero (so target selection is unaffected).
func TestDamageOverTimeEffectRecordsZeroHateThreat(t *testing.T) {
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
	if threat.Hate != 0 {
		t.Fatalf("threat.Hate = %v, want 0", threat.Hate)
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
