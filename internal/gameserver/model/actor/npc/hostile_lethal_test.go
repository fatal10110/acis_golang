package npc

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

type deniedLethalCaster struct{ *Hostile }

func (deniedLethalCaster) CanGiveDamage() bool { return false }

func TestHostileLethalSurfaceBuildsInputAndAppliesOutcomes(t *testing.T) {
	caster := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", Level: 40, HPMax: 500})
	target := newCombatHostile(t, 2, &Template{ID: 2, Type: "Monster", Level: 45, HPMax: 500})
	skill := modelskill.Definition{LethalChance1: 30, LethalChance2: 10, MagicLevel: 40}

	in, ok := target.LethalInput(caster, skill)
	if !ok {
		t.Fatal("LethalInput() ok = false")
	}
	if in.Chance1 != 30 || in.Chance2 != 10 || in.MagicLevel != 40 || in.AttackerLevel != 40 || in.TargetLevel != 45 || in.LethalMul != 1 {
		t.Fatalf("LethalInput() = %+v, want skill fields and 40/45/1 actor values", in)
	}

	hp := target.MaxHPValue()
	target.SetHP(hp)
	target.ApplyLethalOutcome(formulas.LethalHalf, caster, skill)
	if got := target.HP(); got != hp/2 {
		t.Fatalf("half lethal HP = %v, want %v", got, hp/2)
	}

	target.SetHP(hp)
	target.ApplyLethalOutcome(formulas.LethalFull, caster, skill)
	if got := target.HP(); got != 1 {
		t.Fatalf("full lethal HP = %v, want 1", got)
	}
}

func TestHostileLethalInputRejectsGuardedDamage(t *testing.T) {
	caster := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", Level: 40, HPMax: 500})
	target := newCombatHostile(t, 2, &Template{ID: 2, Type: "Monster", Level: 45, HPMax: 500})
	skill := modelskill.Definition{LethalChance1: 30}
	target.SetInvul(true)
	if _, ok := target.LethalInput(caster, skill); ok {
		t.Fatal("LethalInput accepted an invulnerable hostile")
	}
	target.SetInvul(false)
	if _, ok := target.LethalInput(deniedLethalCaster{caster}, skill); ok {
		t.Fatal("LethalInput accepted an attacker without damage permission")
	}
}

func TestHostileLethalableExcludesReferenceExceptions(t *testing.T) {
	for _, tt := range []struct {
		id   int
		want bool
	}{
		{id: 1, want: true},
		{id: 22215}, {id: 22216}, {id: 22217}, {id: 35062},
		{id: 35410}, {id: 35368}, {id: 35375}, {id: 35629},
	} {
		t.Run("npc", func(t *testing.T) {
			h := newCombatHostile(t, 1, &Template{ID: tt.id, Type: "Monster", HPMax: 100})
			if got := h.Lethalable(); got != tt.want {
				t.Fatalf("Lethalable() = %v, want %v for NPC %d", got, tt.want, tt.id)
			}
		})
	}
}
