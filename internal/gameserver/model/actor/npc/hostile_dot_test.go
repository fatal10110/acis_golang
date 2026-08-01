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
		ReduceHPByDOT(float64, any)
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
	if got, want := h.HP(), 96.0; got != want {
		t.Fatalf("HP() = %v, want %v", got, want)
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
