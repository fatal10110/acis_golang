package summon

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

func TestSummonEffectListHoldsAppliedEffects(t *testing.T) {
	servitor := NewServitor(ServitorConfig{ObjectID: 1, Level: 40, Stats: CombatStats{MaxHP: 500, MaxMP: 200}, Roll: zeroSummonRoll})

	if servitor.MaxBuffCount() != baseBuffSlots {
		t.Fatalf("MaxBuffCount() = %d, want %d", servitor.MaxBuffCount(), baseBuffSlots)
	}

	list := servitor.EffectList()
	if list == nil {
		t.Fatal("EffectList() = nil, want a live list wired at construction")
	}

	e, err := effect.New(effect.Skill{ID: 2280, Level: 1, SkillType: "BUFF"}, modelskill.EffectTemplate{Name: "BlockBuff", Count: 1, Time: 60})
	if err != nil {
		t.Fatalf("effect.New() error = %v", err)
	}
	e.Effector = servitor
	e.Effected = servitor
	list.Add(e)

	if got := len(list.All()); got != 1 {
		t.Fatalf("EffectList().All() len = %d, want 1", got)
	}
}

func TestSummonDenyAIActionHonorsCrowdControlEffects(t *testing.T) {
	for _, tt := range []struct {
		name string
		flag effect.Flag
	}{
		{"stun", effect.FlagStunned},
		{"immobile until attacked", effect.FlagMeditating},
		{"sleep", effect.FlagSleep},
		{"paralyze", effect.FlagParalyzed},
		{"fear", effect.FlagFear},
	} {
		t.Run(tt.name, func(t *testing.T) {
			summon := NewServitor(ServitorConfig{ObjectID: 1})
			summon.EffectList().Add(&effect.Effect{Flag: tt.flag})

			if !summon.DenyAIAction() {
				t.Fatal("DenyAIAction() = false while crowd controlled")
			}
		})
	}
}

func TestSummonDenyAIActionHonorsTransientControlStates(t *testing.T) {
	summon := NewServitor(ServitorConfig{ObjectID: 1})

	if !summon.SetParalyzed(true) || !summon.DenyAIAction() {
		t.Fatal("paralyzed summon must deny AI actions")
	}
	if !summon.SetParalyzed(false) || summon.DenyAIAction() {
		t.Fatal("clearing paralysis must allow AI actions")
	}
	if !summon.SetTeleporting(true) || !summon.DenyAIAction() {
		t.Fatal("teleporting summon must deny AI actions")
	}
}

func TestPetEffectListHoldsAppliedEffects(t *testing.T) {
	pet := NewPet(PetConfig{ObjectID: 1, Level: 40, Stats: CombatStats{MaxHP: 500, MaxMP: 200}, Roll: zeroSummonRoll})

	if pet.EffectList() == nil {
		t.Fatal("EffectList() = nil, want a live list wired at construction")
	}
	if pet.MaxBuffCount() != baseBuffSlots {
		t.Fatalf("MaxBuffCount() = %d, want %d", pet.MaxBuffCount(), baseBuffSlots)
	}
}
