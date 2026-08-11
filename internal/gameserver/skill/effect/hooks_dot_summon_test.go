package effect_test

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

func TestDamageOverTimeHookDamagesSummon(t *testing.T) {
	actor := summon.NewPet(summon.PetConfig{Stats: summon.CombatStats{MaxHP: 10}})
	want := actor.HP() - 4
	e, err := effect.New(effect.Skill{ID: 4082}, modelskill.EffectTemplate{Name: "DamOverTime", Value: 4})
	if err != nil {
		t.Fatalf("effect.New() error: %v", err)
	}
	e.Effected = actor

	if !e.ActionTime() {
		t.Fatal("ActionTime() = false, want true")
	}
	if got := actor.HP(); got != want {
		t.Fatalf("HP() = %v, want %v", got, want)
	}
}
