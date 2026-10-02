package skill

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// ---- from handler_test.go ----
// TestRegistryDispatchesBySkillType registers the production
// HEAL_PERCENT/MANAHEAL_PERCENT handler on its own and proves dispatch by
// what it did to a real player: the registered type restores its MP, an
// unregistered type changes nothing.
func TestRegistryDispatchesBySkillType(t *testing.T) {
	h, ok := NewDefaultRegistry().Handler("HEAL_PERCENT")
	if !ok {
		t.Fatal("default registry has no HEAL_PERCENT handler")
	}
	registry := NewRegistry(h)
	target := &player.Character{ID: 1}
	target.SetResourceValues(player.Resources{MaxHP: 100, CurrentHP: 100, MaxMP: 100, CurrentMP: 1})
	const healed = 51.0 // 1 + 50% of 100

	if _, ok := registry.Handler("heal_percent"); !ok {
		t.Fatal("Handler() did not normalize skill type keys")
	}
	if !registry.Use(Cast{Caster: target, Skill: modelskill.Definition{SkillType: "MANAHEAL_PERCENT", Power: 50}, Targets: []Actor{target}}) {
		t.Fatal("Use() returned false for a registered skill type")
	}
	if got := target.MPValue(); got != healed {
		t.Fatalf("target MP = %v, want %v", got, healed)
	}
	if registry.Use(Cast{Caster: target, Skill: modelskill.Definition{SkillType: "NOT_REGISTERED", Power: 50}, Targets: []Actor{target}}) {
		t.Fatal("Use() returned true for an unregistered skill type")
	}
	if got := target.MPValue(); got != healed {
		t.Fatalf("target MP after an unregistered dispatch = %v, want unchanged %v", got, healed)
	}
}

func TestRegistryReportsAttackFailedForPhysicalSkillWithNoDamage(t *testing.T) {
	registry := NewDefaultRegistry()
	target := &skillTarget{
		physicalOK: true,
		physicalInput: formulas.PhysicalSkillInput{
			AttackPower: -1, Defence: 1,
			RandomMul: 1, RaceMul: 1, PvPMul: 1, ElementalMul: 1, WeaponVulnMul: 1,
		},
	}

	result, ok := registry.UseResult(Cast{
		Skill:   modelskill.Definition{SkillType: "PDAM"},
		Targets: []Actor{target},
	})
	if !ok {
		t.Fatal("UseResult() handled = false, want true for PDAM")
	}
	if result.AttackFailed != 1 {
		t.Fatalf("AttackFailed = %d, want 1", result.AttackFailed)
	}
}

func TestDefaultRegistryHasRepresentativeHandlers(t *testing.T) {
	registry := NewDefaultRegistry()

	for _, skillType := range []string{
		"PDAM", "FATAL", "MDAM", "DEATHLINK", "BLOW", "MANADAM",
		"HEAL", "HEAL_STATIC", "HEAL_PERCENT", "MANAHEAL_PERCENT", "MANAHEAL", "MANARECHARGE",
		"COMBATPOINTHEAL", "BALANCE_LIFE", "REAL_DAMAGE", "GIVE_SP",
		"CPDAMPERCENT", "DUMMY", "BEAST_FEED",
		"SUMMON_CREATURE", "SUMMON_FRIEND", "SUMMON_PARTY", "ERASE",
	} {
		if _, ok := registry.Handler(skillType); !ok {
			t.Fatalf("default registry missing %s", skillType)
		}
	}
}
